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

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// ChannelTelegram is the only delivery channel implemented today; the Notifier
// interface is the seam for webhook/email.
const ChannelTelegram = "telegram"

// Digest is one subscription's batch of new matches, rendered by a Notifier into
// a channel-specific message. Jobs is capped to the configured digest size; Total
// is the true count so the renderer can show an "and N more" tail.
type Digest struct {
	SavedSearchName string
	Total           int
	Jobs            []DigestJob
}

// DigestJob is the display shape of one matched job (no internal id).
type DigestJob struct {
	Title   string
	Company string
	Slug    string
	URL     string
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
	RecordSubscriptionMatch(ctx context.Context, arg db.RecordSubscriptionMatchParams) (int64, error)
	ClaimSubscriptionMatches(ctx context.Context, arg db.ClaimSubscriptionMatchesParams) ([]db.ClaimSubscriptionMatchesRow, error)
	GetSubscriptionForDelivery(ctx context.Context, id int64) (db.GetSubscriptionForDeliveryRow, error)
	GetJobsForDigest(ctx context.Context, jobIds []int64) ([]db.GetJobsForDigestRow, error)
	MarkMatchesNotified(ctx context.Context, arg db.MarkMatchesNotifiedParams) (int64, error)
	RecordMatchDeliveryFailure(ctx context.Context, arg db.RecordMatchDeliveryFailureParams) error
	ReleaseMatchClaim(ctx context.Context, arg db.ReleaseMatchClaimParams) error
}

// Config tunes one pass. Defaults come from DefaultConfig.
type Config struct {
	// MatchLimit bounds how many recent jobs (by created_at desc) each distinct
	// query scans per pass. A burst beyond this for one filter is the known seam.
	MatchLimit int
	// LeaseSeconds is the delivery lease: a claimed-but-undelivered match is
	// reclaimable after this, which doubles as the crash reaper.
	LeaseSeconds int32
	// ClaimBatch bounds how many pending matches one pass delivers.
	ClaimBatch int32
	// MaxAttempts dead-letters a match after this many failed deliveries.
	MaxAttempts int32
	// DigestCap bounds how many jobs are listed in one digest message (the rest
	// are still marked notified and summarized as a count).
	DigestCap int
}

// DefaultConfig is the production tuning. MatchLimit/cadence are revisited from
// observed ingest rates (see the design's open questions).
func DefaultConfig() Config {
	return Config{
		MatchLimit:   200,
		LeaseSeconds: 600,
		ClaimBatch:   500,
		MaxAttempts:  5,
		DigestCap:    20,
	}
}

// Stats is the per-pass summary logged by the worker.
type Stats struct {
	Queries   int // distinct queries matched this pass
	Matched   int // newly recorded (subscription, job) matches
	Delivered int // digests sent
	SoftSkips int // digests skipped (e.g. Telegram not linked)
	Failed    int // digest deliveries that errored
}

// Runner executes matching + delivery passes.
type Runner struct {
	store    Store
	searcher Searcher
	notifier Notifier
	cfg      Config
}

// New builds a Runner.
func New(store Store, searcher Searcher, notifier Notifier, cfg Config) *Runner {
	return &Runner{store: store, searcher: searcher, notifier: notifier, cfg: cfg}
}

// Run executes one MATCH-then-DELIVER pass. MATCH records new matches; DELIVER
// drains the pending queue. Unsent matches are retried by the next pass, so a
// delivery outage loses nothing.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := r.match(ctx, &stats); err != nil {
		return stats, fmt.Errorf("match: %w", err)
	}
	if err := r.deliver(ctx, &stats); err != nil {
		return stats, fmt.Errorf("deliver: %w", err)
	}
	log.Printf("notify: queries=%d matched=%d delivered=%d soft_skips=%d failed=%d",
		stats.Queries, stats.Matched, stats.Delivered, stats.SoftSkips, stats.Failed)
	return stats, nil
}

// telegramRecipient resolves the destination string for delivery, and whether the
// subscription is deliverable right now. Telegram resolves the linked chat_id
// (absent → not deliverable, soft-skipped); other channels use destination.
func recipient(info db.GetSubscriptionForDeliveryRow) (string, bool) {
	if info.Channel == ChannelTelegram {
		if !info.TelegramChatID.Valid {
			return "", false
		}
		return strconv.FormatInt(info.TelegramChatID.Int64, 10), true
	}
	if !info.Destination.Valid || info.Destination.String == "" {
		return "", false
	}
	return info.Destination.String, true
}
