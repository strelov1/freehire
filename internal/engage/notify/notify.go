// Package notify is the filter-subscription matching + delivery use case. One
// pass (Runner.Run) does two stages: MATCH re-runs each DISTINCT saved-search
// query against the search index and records the jobs that match each
// subscription in the dedup ledger; DELIVER leases a subscription's pending
// matches, sends them as one digest through a channel Notifier, and marks them
// notified. It is the engine behind the run-once cmd/notify cron worker.
//
// Cost is O(distinct queries) per pass — subscriptions sharing a query are
// grouped so the index is hit once regardless of subscriber count — and the
// ledger's PK makes matching idempotent, so re-scanning recent jobs never
// delivers twice.
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

// ValidChannel reports whether c is a delivery channel. It exists because the slice alone is
// not usable as an allowlist, so both create-time gates — subscriptions and reminders — built
// the same map[string]bool from it. Exposing the membership test is what stops a third caller
// building a third copy.
func ValidChannel(c string) bool {
	return slices.Contains(Channels, c)
}

// ListLimit is how many jobs a channel message itemizes. It is a constant rather
// than a Config field because the channel notifiers take only a base URL and a
// transport — reaching Config into them would buy nothing, and a shared constant
// keeps every channel's list and its "and N more" tail arithmetic in agreement.
//
// Ten, not the snapshot's ceiling: a digest is a notification, and a mail that
// itemizes dozens of jobs is a page nobody reads to the bottom of.
const ListLimit = 10

// Listed splits a grouped message's items into the ones it itemizes — the first
// ListLimit — and the count it only mentions as an "and N more" tail. It lives
// beside the bound, and is generic, because all three engines apply the same rule
// to three different message types and a private copy per engine is a copy that
// drifts.
//
// The shown slice has no spare capacity, so a renderer that appends to it cannot
// scribble over the items it left out.
func Listed[T any](items []T) (shown []T, more int) {
	if len(items) <= ListLimit {
		return items, 0
	}
	return items[:ListLimit:ListLimit], len(items) - ListLimit
}

// Digest is one subscription's batch of new matches, rendered by a Notifier into
// a channel-specific message. Jobs is what the digest carries — the newest
// pending matches, at most Config.SnapshotCap — and is what the in-app
// notification records; Listed is the much shorter slice a message itemizes.
// Total is the true count of matches this digest delivers, which exceeds
// len(Jobs) when more were pending than one digest carries (see Overflowed), so
// a renderer can show an "and N more" tail under either bound.
type Digest struct {
	// UserID is whose digest this is. Only the email channel reads it, to sign that
	// recipient's unsubscribe link — a chat or webhook destination the user
	// connected themselves is turned off where they connected it.
	UserID          int64
	SavedSearchName string
	// SearchQuery is the saved search's stored query string — where an
	// overflowed digest's tail sends the reader, since the recorded snapshot
	// cannot list what it does not carry. See SearchPath.
	SearchQuery string
	Total       int
	Jobs        []DigestJob
	// NotificationID is the in-app notification recording this digest, which is
	// where a channel's "and N more" tail sends the reader. Zero when the
	// recording failed — the renderer then falls back to a generic destination
	// rather than dropping the tail.
	NotificationID int64
}

// Listed returns the jobs a channel message should itemize: the first ListLimit,
// or all of them when the digest is shorter. The omitted count is not returned
// here — a digest's tail counts against Total, which can exceed len(Jobs).
func (d Digest) Listed() []DigestJob {
	shown, _ := Listed(d.Jobs)
	return shown
}

// Overflowed reports whether the digest counts more matches than it carries.
// Its "and N more" tail must then lead to the saved search's live results: the
// notification's own matched-jobs page lists only the recorded snapshot, which
// would show a reader fewer jobs than the message just promised.
func (d Digest) Overflowed() bool { return d.Total > len(d.Jobs) }

// SearchPath is the site-relative URL of the saved search's live results,
// tagged with a channel's UTM source — the same /jobs?<query> link the saved
// searches page opens. The stored query is already URL-encoded, so it is
// appended as it is.
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

// Notifier delivers a digest over a channel to a destination. The matching engine
// depends only on this, so a new channel is a new implementation, not a change
// here. (Telegram resolves dest as a chat_id string; webhook/email as a URL/address.)
type Notifier interface {
	Send(ctx context.Context, channel, dest string, d Digest) error
}

// Searcher is the search backend the matcher runs filters against. *search.Client
// satisfies it; tests inject a fake.
type Searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// Store is the persistence the engine needs. *db.Queries satisfies it directly.
type Store interface {
	ListActiveSubscriptions(ctx context.Context) ([]db.ListActiveSubscriptionsRow, error)
	// ListUserProfilesExcludedSkills returns each given user's avoid-skills preference, one
	// round trip regardless of batch size. A user id with no profile row simply has no row in
	// the result; the caller treats absence as an empty exclude set.
	ListUserProfilesExcludedSkills(ctx context.Context, userIDs []int64) ([]db.ListUserProfilesExcludedSkillsRow, error)
	RecordSubscriptionMatches(ctx context.Context, arg db.RecordSubscriptionMatchesParams) (int64, error)
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
	// RecordNotification returns the new row's id. A digest is recorded BEFORE
	// it is sent so the message can link to that row's matched-jobs page;
	// DeleteNotification withdraws it when the send then fails. With a dedup key
	// set it returns pgx.ErrNoRows when another channel of the same saved search
	// already recorded this event — not a failure, see recordNotification.
	RecordNotification(ctx context.Context, arg db.RecordNotificationParams) (int64, error)
	// GetNotificationIDByDedupKey reads back the row another channel recorded
	// for this same event, so every channel's message links to one page.
	GetNotificationIDByDedupKey(ctx context.Context, arg db.GetNotificationIDByDedupKeyParams) (int64, error)
	DeleteNotification(ctx context.Context, id int64) error
	MarkDigestSent(ctx context.Context, id int64) error
	// DeleteTelegramLink forgets a user's Telegram chat. Called when a send
	// reports the chat is permanently closed (ErrRecipientGone) — the same
	// unlink the settings page performs, reached from the delivery side.
	DeleteTelegramLink(ctx context.Context, userID int64) (int64, error)
	// DisableWebhookConfig disables a user's webhook destination. Called when a
	// webhook send reports the destination is gone for good (ErrRecipientGone,
	// mapped from an HTTP 410) — the webhook channel's counterpart to
	// DeleteTelegramLink.
	DisableWebhookConfig(ctx context.Context, userID int64) (int64, error)
	// RecordWebhookDeliverySuccess stamps last_success_at after a webhook
	// digest sends successfully — the counterpart to DisableWebhookConfig on
	// the success side.
	RecordWebhookDeliverySuccess(ctx context.Context, userID int64) error
	// RecordWebhookDeliveryFailure counts a non-410 webhook send failure
	// toward the destination's auto-disable threshold, disabling it once the
	// returned count reaches Config.WebhookMaxConsecutiveFailures — the
	// counterpart to DisableWebhookConfig for a destination that fails
	// without ever answering 410 Gone (see migration 0177).
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
	// subscriptions. It is a backstop, not the working bound: what a pass normally
	// claims is SnapshotCap per subscription with anything pending. Set it above
	// SnapshotCap x the active subscription count, or the cut falls on whichever
	// subscriptions the scan reached last — the starvation this replaced.
	ClaimBatch int32
	// MaxAttempts dead-letters a match after this many failed deliveries.
	MaxAttempts int32
	// WebhookMaxConsecutiveFailures auto-disables a webhook destination once
	// it has failed this many sends in a row with no success in between (see
	// RecordWebhookDeliveryFailure) — the webhook channel's own dead-letter,
	// independent of MaxAttempts since a destination that is simply gone
	// would otherwise fail a fresh match every pass, forever.
	WebhookMaxConsecutiveFailures int64
	// SnapshotCap bounds how many jobs a digest carries — which is how many the
	// in-app notification records and its matched-jobs page can show. It is NOT
	// the message listing bound (that is ListLimit): a message is short because a
	// notification should be, while the recorded set is short only to keep one
	// notification's jsonb document from growing without limit. It is also the
	// per-subscription claim, newest first. When more are pending, the rest are
	// COUNTED in the digest's Total, its tail links to the saved search's live
	// results, and once it is delivered they are marked notified with it — never
	// re-queued, which is what kept a busy `daily` subscription mailing the same
	// backlog, oldest first, for weeks.
	SnapshotCap int
	// FreshnessWindow is how long a match may wait to be delivered. Older than
	// this — or for a job that has since closed — it goes stale and leaves the
	// pending set undelivered (expired_at, migration 0193). It is what bounds the
	// backlog of a subscription nobody can currently receive (push with no device,
	// an unlinked Telegram chat), so a person who reconnects is sent what is new,
	// not a month of postings that have mostly closed.
	FreshnessWindow time.Duration
}

// DefaultConfig is the production tuning. MatchLimit/cadence are revisited from
// observed ingest rates (see the design's open questions).
func DefaultConfig() Config {
	return Config{
		MatchLimit:   200,
		LeaseSeconds: 600,
		// 200 per subscription x room for 250 active subscriptions. Prod carries 53;
		// the headroom is deliberate, since hitting this bound is what starvation
		// looks like.
		ClaimBatch:  50000,
		MaxAttempts: 5,
		// Matches MaxAttempts: five chances before giving up, whether the
		// failures are spread across different matches (MaxAttempts) or the
		// same destination refusing everything in a row (this).
		WebhookMaxConsecutiveFailures: 5,
		// One query cannot match more than MatchLimit jobs in a pass, so an
		// `instant` digest is carried whole. A `daily` one, or one held through
		// quiet hours, accumulates across passes and can exceed it; that digest
		// carries the newest 200 and counts the rest.
		SnapshotCap: 200,
		// Three days: long enough to outlast quiet hours, a daily digest's wait
		// and a weekend away from the phone, short enough that nothing announced
		// as new is a posting most people would already have seen elsewhere.
		FreshnessWindow: 72 * time.Hour,
	}
}

// Stats is the per-pass summary logged by the worker.
type Stats struct {
	Queries       int // distinct queries matched this pass
	FailedQueries int // distinct queries whose search or record step errored and was skipped
	Matched       int // newly recorded (subscription, job) matches
	Delivered     int // digests sent
	SoftSkips     int // digests skipped (e.g. Telegram not linked)
	Deferred      int // digests held back by quiet hours or a not-yet-due daily digest time
	Expired       int // pending matches that went stale (too old, or the job closed) undelivered
	Failed        int // digest deliveries that errored
}

// MatchingCollapsed reports whether the MATCH stage produced nothing because every
// query it ran failed — the outcome the run has to go red on.
//
// It is deliberately narrower than "any query failed". Matching is isolated per query
// exactly like the per-board ingest crawl, and one saved search whose filter Meilisearch
// dislikes must not fail a pass that served the other fifty-two; the count in the log is
// what makes that one visible. All of them failing is a different statement — an
// unreachable index or a broken credential — and it is the one this worker used to print
// as `queries=53 matched=0 ... failed=0` before exiting 0.
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
	// Deferred so the counters land on EVERY exit, not only the clean one. A pass that
	// stopped part-way is the pass whose figures somebody actually wants, and printing
	// them last meant the run that ended early was also the run that said nothing.
	defer func() {
		log.Printf("notify: queries=%d failed_queries=%d matched=%d expired=%d delivered=%d soft_skips=%d deferred=%d failed=%d",
			stats.Queries, stats.FailedQueries, stats.Matched, stats.Expired, stats.Delivered, stats.SoftSkips, stats.Deferred, stats.Failed)
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
// subscription is deliverable right now. Telegram resolves the linked chat_id
// (absent → not deliverable, soft-skipped); email resolves the user's account
// email live (absent → soft-skipped); push resolves the subscribing user's id,
// deliverable only when they have a currently registered device (absent →
// soft-skipped, same as an unlinked Telegram chat); webhook resolves the
// account's configured destination, deliverable only when one exists and is
// enabled; any other channel uses the stored destination.
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
