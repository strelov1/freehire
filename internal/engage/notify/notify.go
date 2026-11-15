// Package notify is the filter-subscription matching + delivery use case. One
// pass (Runner.Run) does two stages: MATCH re-runs each DISTINCT saved-search
// query against the search index and records the jobs that match each
// subscription in the dedup ledger; DELIVER leases a subscription's pending
// matches, sends them as one digest through a channel Notifier, and marks them
// notified. It is the engine behind the run-once cmd/notify cron worker.
//
// Subscriptions sharing a query are grouped so the index is hit once per distinct
// query, and the ledger's PK makes matching idempotent, so re-scanning recent jobs
// never delivers twice.
package notify

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"slices"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// ChannelTelegram and ChannelEmail are the delivery channels implemented today;
// the Notifier interface remains the seam for future channels (e.g. webhooks).
// ChannelEmail is declared alongside the Router in router.go.
const ChannelTelegram = "telegram"

// ChannelPush delivers a digest as a mobile push notification (via Expo), fanned
// out to every device the user has registered. Unlike Telegram/email it needs no
// server-side credential, so it is always registered in a Router regardless of
// environment configuration.
const ChannelPush = "push"

// ChannelWebhook delivers a digest as a plain HTTP POST to the account's
// configured webhook destination (see internal/engage/webhooknotify). Unlike
// Telegram/email/push, the destination is a URL the user supplies, not a
// platform-owned transport.
const ChannelWebhook = "webhook"

// Channels is the delivery-channel vocabulary: the single source of truth shared
// by the router's dispatch and the subscription use case's create-time allowlist,
// so the two can never drift.
var Channels = []string{ChannelTelegram, ChannelEmail, ChannelPush, ChannelWebhook}

// ValidChannel reports whether c is a delivery channel. Both create-time gates,
// subscriptions and reminders, use it instead of building their own allowlist.
func ValidChannel(c string) bool {
	return slices.Contains(Channels, c)
}

// The saved-search digest frequencies (notification_settings.digest_frequency).
//
//   - FrequencyInstant sends on every pass that finds something new.
//   - FrequencyDaily sends once a local day, at the account's digest time.
//   - FrequencyInterval sends at most once every digest_interval_hours hours.
//
// FrequencyDaily at DefaultDigestTime is the default for an account with no settings
// row (migration 0246). An unknown stored value reads as FrequencyInterval.
const (
	FrequencyInstant  = "instant"
	FrequencyDaily    = "daily"
	FrequencyInterval = "interval"
)

// The bounds of an interval digest, in hours. A full day is FrequencyDaily, which
// also picks the hour, so the interval stops one short of it. The column's CHECK
// (migration 0233) holds the same numbers.
const (
	MinIntervalHours     = 1
	MaxIntervalHours     = 23
	DefaultIntervalHours = 1
)

// DefaultDigestTime is the local time a default daily digest goes out. Without a
// time a daily digest is never due (deliverywindow.DigestDue), so the default
// frequency must carry one. Migration 0246 and the queries' COALESCE hold the same.
const DefaultDigestTime = 9 * time.Hour

// ListLimit is how many jobs a channel message itemizes. A shared constant keeps
// every channel's list and its "and N more" tail in agreement. It is well below the
// snapshot cap because a notification that itemizes dozens of jobs goes unread.
const ListLimit = 10

// Listed splits a grouped message's items into the ones it itemizes (the first
// ListLimit) and the count it only mentions as an "and N more" tail. It is generic
// because all three engines apply the same rule to different message types.
//
// The shown slice has no spare capacity, so a renderer that appends to it cannot
// scribble over the items it left out.
func Listed[T any](items []T) (shown []T, more int) {
	if len(items) <= ListLimit {
		return items, 0
	}
	return items[:ListLimit:ListLimit], len(items) - ListLimit
}

// Digest is one subscription's batch of new matches, rendered by a Notifier into a
// channel-specific message. Jobs is what the digest carries (the newest pending
// matches, at most Config.SnapshotCap) and what the in-app notification records;
// Listed is the shorter slice a message itemizes. Total is the true count delivered,
// which exceeds len(Jobs) when more were pending (see Overflowed).
type Digest struct {
	SavedSearchName string
	// SearchQuery is the saved search's stored query string, where an overflowed
	// digest's tail sends the reader. See SearchPath.
	SearchQuery string
	Total       int
	Jobs        []DigestJob
	// NotificationID is the in-app notification recording this digest, where a
	// channel's "and N more" tail sends the reader. Zero when the recording failed;
	// the renderer then falls back to a generic destination.
	NotificationID int64
}

// Listed returns the jobs a channel message should itemize: the first ListLimit,
// or all of them when the digest is shorter. The omitted count is not returned: a
// digest's tail counts against Total, which can exceed len(Jobs).
func (d Digest) Listed() []DigestJob {
	shown, _ := Listed(d.Jobs)
	return shown
}

// Overflowed reports whether the digest counts more matches than it carries. Its
// "and N more" tail must then lead to the saved search's live results, since the
// notification's matched-jobs page lists only the recorded snapshot.
func (d Digest) Overflowed() bool { return d.Total > len(d.Jobs) }

// SearchPath is the site-relative URL of the saved search's live results, tagged
// with a channel's UTM source. The stored query is already URL-encoded.
func (d Digest) SearchPath(utmSource string) string {
	var b strings.Builder
	b.WriteString("/jobs?")
	if d.SearchQuery != "" {
		b.WriteString(d.SearchQuery)
		b.WriteByte('&')
	}
	b.WriteString("utm_source=" + utmSource)
	return b.String()
}

// Message is what one send delivers: every digest one person is due on one channel,
// oldest subscription first (by id, so sections keep their order). A person with
// several alerts gets one message carrying all their digests. A webhook is the
// exception: its receiver reads one saved search's payload, so it gets one digest
// per message.
type Message struct {
	// UserID is whose message this is. Only the email channel reads it, to sign that
	// recipient's unsubscribe link.
	UserID  int64
	Digests []Digest
}

// Total is how many matches the message announces across its digests.
func (m Message) Total() int {
	n := 0
	for _, d := range m.Digests {
		n += d.Total
	}
	return n
}

// DigestJob is the display shape of one matched job (no internal id). The salary
// fields are projected from the job's enrichment; a zero SalaryMin/SalaryMax (or
// empty Currency) means the compensation is unknown and the renderer omits it.
type DigestJob struct {
	Title          string
	Company        string
	Slug           string
	URL            string
	SalaryMin      int
	SalaryMax      int
	SalaryCurrency string
	SalaryPeriod   string
}

// Notifier delivers a message over a channel to a destination. The matching engine
// depends only on this, so a new channel is a new implementation, not a change
// here. (Telegram resolves dest as a chat_id string; webhook/email as a URL/address.)
type Notifier interface {
	Send(ctx context.Context, channel, dest string, m Message) error
}

// Searcher is the search backend the matcher runs filters against. *search.Client
// satisfies it; tests inject a fake.
type Searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// Store is the persistence the engine needs. *db.Queries satisfies it directly.
type Store interface {
	ListActiveSubscriptions(ctx context.Context) ([]db.ListActiveSubscriptionsRow, error)
	// ListUserProfileSkillSets returns each given user's held and avoided skills. A
	// user with no profile row has no row; the caller treats absence as empty sets.
	ListUserProfileSkillSets(ctx context.Context, userIDs []int64) ([]db.ListUserProfileSkillSetsRow, error)
	// ListWatchlistSlugsForUsers returns one row per (user, company) watched. A user
	// with an empty watchlist has no rows.
	ListWatchlistSlugsForUsers(ctx context.Context, userIDs []int64) ([]db.ListWatchlistSlugsForUsersRow, error)
	RecordSubscriptionMatches(ctx context.Context, arg db.RecordSubscriptionMatchesParams) (int64, error)
	// MarkSubscriptionsSearched stamps when the matching stage last searched these
	// subscriptions; searchDue and matchQuery's paging read it back.
	MarkSubscriptionsSearched(ctx context.Context, arg db.MarkSubscriptionsSearchedParams) error
	// DeactivateSubscriptions pauses the given subscriptions and returns how many it
	// paused; see pauseUnrunnable.
	DeactivateSubscriptions(ctx context.Context, arg db.DeactivateSubscriptionsParams) (int64, error)
	// ListDeliveryCandidates lists the subscriptions with a pending match and their delivery
	// context, so the worker leases only the ones deliverable now (see deliveryHold).
	ListDeliveryCandidates(ctx context.Context) ([]db.ListDeliveryCandidatesRow, error)
	ClaimSubscriptionMatches(ctx context.Context, arg db.ClaimSubscriptionMatchesParams) ([]db.ClaimSubscriptionMatchesRow, error)
	// ExpireStaleSubscriptionMatches takes up to BatchSize stale matches (older
	// than the freshness window, or for a closed job) out of the pending set and
	// returns how many it took.
	ExpireStaleSubscriptionMatches(ctx context.Context, arg db.ExpireStaleSubscriptionMatchesParams) (int64, error)
	// CountCoveredMatches and MarkCoveredMatchesNotified measure and then retire
	// the pending matches a full claim left behind it — the ones a digest counts
	// in its Total without carrying. Same predicate, same cutoff.
	CountCoveredMatches(ctx context.Context, arg db.CountCoveredMatchesParams) (int64, error)
	MarkCoveredMatchesNotified(ctx context.Context, arg db.MarkCoveredMatchesNotifiedParams) (int64, error)
	GetSubscriptionForDelivery(ctx context.Context, id int64) (db.GetSubscriptionForDeliveryRow, error)
	GetJobsForDigest(ctx context.Context, jobIds []int64) ([]db.GetJobsForDigestRow, error)
	MarkMatchesNotified(ctx context.Context, arg db.MarkMatchesNotifiedParams) (int64, error)
	RecordMatchDeliveryFailure(ctx context.Context, arg db.RecordMatchDeliveryFailureParams) error
	ReleaseMatchClaim(ctx context.Context, arg db.ReleaseMatchClaimParams) error
	// RecordNotification returns the new row's id. A digest is recorded BEFORE it is
	// sent so the message can link to its matched-jobs page; DeleteNotification
	// withdraws it when the send fails. With a dedup key set it returns pgx.ErrNoRows
	// when another channel already recorded this event (not a failure, see
	// recordNotification).
	RecordNotification(ctx context.Context, arg db.RecordNotificationParams) (int64, error)
	// GetNotificationIDByDedupKey reads back the row another channel recorded for
	// this event, so every channel's message links to one page.
	GetNotificationIDByDedupKey(ctx context.Context, arg db.GetNotificationIDByDedupKeyParams) (int64, error)
	DeleteNotification(ctx context.Context, id int64) error
	MarkDigestSent(ctx context.Context, id int64) error
	// DeleteTelegramLink forgets a user's Telegram chat, called when a send reports
	// it permanently closed (ErrRecipientGone).
	DeleteTelegramLink(ctx context.Context, userID int64) (int64, error)
	// DisableWebhookConfig disables a user's webhook destination, called when a send
	// reports it gone for good (ErrRecipientGone, mapped from HTTP 410).
	DisableWebhookConfig(ctx context.Context, userID int64) (int64, error)
	// RecordWebhookDeliverySuccess stamps last_success_at after a successful send.
	RecordWebhookDeliverySuccess(ctx context.Context, userID int64) error
	// RecordWebhookDeliveryFailure counts a non-410 send failure toward the
	// destination's auto-disable threshold, disabling it once the returned count
	// reaches Config.WebhookMaxConsecutiveFailures (see migration 0177).
	RecordWebhookDeliveryFailure(ctx context.Context, arg db.RecordWebhookDeliveryFailureParams) (db.RecordWebhookDeliveryFailureRow, error)
}

// Config tunes one pass. Defaults come from DefaultConfig.
type Config struct {
	// MatchLimit bounds how many recent jobs (by created_at desc) each distinct
	// query scans per pass. A burst beyond this for one filter is the known seam.
	MatchLimit int
	// LeaseSeconds is the delivery lease: a claimed-but-undelivered match is
	// reclaimable after this, which doubles as the crash reaper.
	LeaseSeconds int32
	// ClaimBatch bounds how many pending matches one pass delivers, across all
	// subscriptions. It is a backstop: a pass normally claims SnapshotCap per
	// subscription. Keep it above SnapshotCap x the active subscription count, or the
	// cut falls on whichever subscriptions the scan reached last (starvation).
	ClaimBatch int32
	// MaxAttempts dead-letters a match after this many failed deliveries.
	MaxAttempts int32
	// WebhookMaxConsecutiveFailures auto-disables a webhook destination after this
	// many failed sends in a row (see RecordWebhookDeliveryFailure). It is
	// independent of MaxAttempts: a gone destination would otherwise fail a fresh
	// match every pass, forever.
	WebhookMaxConsecutiveFailures int64
	// SnapshotCap bounds how many jobs a digest carries, i.e. how many the in-app
	// notification records. It is NOT the message listing bound (ListLimit); it only
	// keeps one notification's jsonb document bounded. It is also the
	// per-subscription claim, newest first. When more are pending, the rest are
	// COUNTED in the digest's Total, its tail links to the live search, and they are
	// marked notified with it, never re-queued.
	SnapshotCap int
	// FreshnessWindow is how long a match may wait to be delivered. Older, or for a
	// job that has since closed, it goes stale and leaves the pending set undelivered
	// (expired_at, migration 0193). It bounds the backlog of a subscription nobody can
	// currently receive, so a reconnecting person is sent what is new.
	FreshnessWindow time.Duration
}

// DefaultConfig is the production tuning.
func DefaultConfig() Config {
	return Config{
		MatchLimit:   200,
		LeaseSeconds: 600,
		// 200 per subscription x room for 250 active subscriptions; the headroom is
		// deliberate, since hitting this bound is what starvation looks like.
		ClaimBatch:  50000,
		MaxAttempts: 5,
		// Matches MaxAttempts: five chances, whether spread across matches or one
		// destination refusing everything.
		WebhookMaxConsecutiveFailures: 5,
		// An `instant` digest is carried whole (a query matches at most MatchLimit per
		// pass). An `interval`, `daily` or quiet-hours-held one accumulates and can
		// exceed it: it carries the newest 200 and counts the rest.
		SnapshotCap: 200,
		// Three days: outlasts quiet hours, a daily digest's wait and a weekend away,
		// yet nothing announced as new is stale.
		FreshnessWindow: 72 * time.Hour,
	}
}

// Stats is the per-pass summary logged by the worker.
type Stats struct {
	Queries        int // distinct queries searched this pass
	SkippedQueries int // distinct queries not searched: none of their subscriptions was due (searchDue)
	Searches       int // index requests made, counting every page of a paged query
	Paused         int // subscriptions paused because the search cannot run their query (pauseUnrunnable)
	FailedQueries  int // distinct queries whose search or record step errored and was skipped
	Matched        int // newly recorded (subscription, job) matches
	Delivered      int // messages sent (one per person and channel, carrying a digest per subscription)
	SoftSkips      int // digests skipped (e.g. Telegram not linked)
	Deferred       int // digests held back by quiet hours or a not-yet-due daily digest time
	Expired        int // pending matches that went stale (too old, or the job closed) undelivered
	Failed         int // digest deliveries that errored
}

// MatchingCollapsed reports whether the MATCH stage produced nothing because every
// query it ran failed, the outcome the run has to go red on.
//
// It is deliberately narrower than "any query failed": one saved search the index
// dislikes must not fail a pass that served the rest. All of them failing means an
// unreachable index or a broken credential.
func (s Stats) MatchingCollapsed() bool {
	return s.Queries > 0 && s.FailedQueries == s.Queries
}

// Runner executes matching + delivery passes.
type Runner struct {
	store    Store
	searcher Searcher
	notifier Notifier
	cfg      Config
	now      func() time.Time
}

// New builds a Runner.
func New(store Store, searcher Searcher, notifier Notifier, cfg Config) *Runner {
	return &Runner{store: store, searcher: searcher, notifier: notifier, cfg: cfg, now: time.Now}
}

// Run executes one MATCH-then-DELIVER pass. MATCH records new matches; DELIVER
// drains the pending queue. Unsent matches are retried by the next pass, so a
// delivery outage shorter than Config.FreshnessWindow loses nothing; past it,
// what was never sent goes stale rather than arriving late.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats
	// Deferred so the counters land on every exit; a pass that stopped part-way is
	// the one whose figures matter.
	defer func() {
		log.Printf("notify: queries=%d skipped_queries=%d searches=%d paused=%d failed_queries=%d matched=%d expired=%d delivered=%d soft_skips=%d deferred=%d failed=%d",
			stats.Queries, stats.SkippedQueries, stats.Searches, stats.Paused, stats.FailedQueries, stats.Matched, stats.Expired, stats.Delivered, stats.SoftSkips, stats.Deferred, stats.Failed)
	}()
	if err := r.match(ctx, &stats); err != nil {
		return stats, fmt.Errorf("match: %w", err)
	}
	if err := r.deliver(ctx, &stats); err != nil {
		return stats, fmt.Errorf("deliver: %w", err)
	}
	return stats, nil
}

// recipient resolves the destination string for delivery, and whether the
// subscription is deliverable right now. Telegram resolves the linked chat_id, email
// the account email live, push the user's id (only with a registered device), and
// webhook the configured destination (only when enabled); any other channel uses
// the stored destination. Not deliverable means soft-skipped.
//
// The deliverability half of this rule has a SQL twin: the `deliverable` CASE in
// NotifyBacklogMetrics (internal/platform/db/queries/metrics.sql), which splits the
// backlog metric the same way. Change one and change the other.
func recipient(info db.GetSubscriptionForDeliveryRow) (string, bool) {
	switch info.Channel {
	case ChannelTelegram:
		if !info.TelegramChatID.Valid {
			return "", false
		}
		return strconv.FormatInt(info.TelegramChatID.Int64, 10), true
	case ChannelEmail:
		if info.AccountEmail == "" {
			return "", false
		}
		return info.AccountEmail, true
	case ChannelPush:
		if !info.HasPushDevice {
			return "", false
		}
		return strconv.FormatInt(info.UserID, 10), true
	case ChannelWebhook:
		if !info.WebhookEnabled || !info.WebhookUrl.Valid || info.WebhookUrl.String == "" {
			return "", false
		}
		return info.WebhookUrl.String, true
	}
	if !info.Destination.Valid || info.Destination.String == "" {
		return "", false
	}
	return info.Destination.String, true
}
