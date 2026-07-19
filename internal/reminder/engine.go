package reminder

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
)

// ReminderMessage is the display shape of one saved-job reminder, rendered by a
// Notifier into a channel-specific message. No internal ids leak into it.
type ReminderMessage struct {
	JobTitle string
	Company  string
	Slug     string
	URL      string
}

// Notifier delivers a reminder over a channel to a destination. The engine depends
// only on this, so a new channel is a new implementation, not a change here.
type Notifier interface {
	Send(ctx context.Context, channel, dest string, m ReminderMessage) error
}

// ErrChannelNotConfigured is returned by Router.Send for a channel with no
// registered notifier (e.g. email while SES is unconfigured). The engine treats it
// as a soft-skip, not a delivery failure.
var ErrChannelNotConfigured = errors.New("reminder: channel not configured")

// Router dispatches a reminder to the notifier registered for its channel, so the
// engine stays channel-agnostic.
type Router map[string]Notifier

// Compile-time guarantee that Router is a Notifier.
var _ Notifier = (Router)(nil)

// Send routes to the registered notifier, or ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest string, m ReminderMessage) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, m)
}

// Compile-time proof that the generated *db.Queries satisfies the engine's Store.
var _ Store = (*db.Queries)(nil)

// Store is the persistence the firing engine needs. *db.Queries satisfies it.
type Store interface {
	ClaimDueReminders(ctx context.Context, arg db.ClaimDueRemindersParams) ([]int64, error)
	GetReminderForDelivery(ctx context.Context, id int64) (db.GetReminderForDeliveryRow, error)
	MarkReminderDelivered(ctx context.Context, id int64) (int64, error)
	CancelReminderAtFire(ctx context.Context, id int64) (int64, error)
	RecordReminderDeliveryFailure(ctx context.Context, arg db.RecordReminderDeliveryFailureParams) error
	ReleaseReminderClaim(ctx context.Context, id int64) error
}

// Config tunes one firing pass. Defaults come from DefaultConfig.
type Config struct {
	// LeaseSeconds is the delivery lease: a claimed-but-unfinished reminder is
	// reclaimable after this, which doubles as the crash reaper.
	LeaseSeconds int32
	// ClaimBatch bounds how many due reminders one pass fires.
	ClaimBatch int32
	// MaxAttempts dead-letters a reminder after this many failed deliveries.
	MaxAttempts int32
}

// DefaultConfig is the production tuning, mirroring internal/notify.
func DefaultConfig() Config {
	return Config{LeaseSeconds: 600, ClaimBatch: 500, MaxAttempts: 5}
}

// Stats is the per-pass summary logged by the worker.
type Stats struct {
	Delivered int // reminders sent
	Cancelled int // reminders cancelled at fire (job closed or no longer actionable)
	SoftSkips int // reminders with no deliverable channel this pass
	Failed    int // reminders whose delivery errored
}

// Runner fires due reminders.
type Runner struct {
	store    Store
	notifier Notifier
	cfg      Config
}

// NewRunner builds a firing Runner.
func NewRunner(store Store, notifier Notifier, cfg Config) *Runner {
	return &Runner{store: store, notifier: notifier, cfg: cfg}
}

// Run executes one firing pass: lease the due reminders and deliver each. Unfinished
// reminders are retried by the next pass (their lease expires), so a delivery outage
// loses nothing.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats
	due, err := r.store.ClaimDueReminders(ctx, db.ClaimDueRemindersParams{
		LeaseSeconds: r.cfg.LeaseSeconds,
		BatchSize:    r.cfg.ClaimBatch,
	})
	if err != nil {
		return stats, fmt.Errorf("claim: %w", err)
	}
	for _, id := range due {
		r.fire(ctx, id, &stats)
	}
	log.Printf("reminder: delivered=%d cancelled=%d soft_skips=%d failed=%d",
		stats.Delivered, stats.Cancelled, stats.SoftSkips, stats.Failed)
	return stats, nil
}

// fire delivers one reminder and finalizes its ledger row. It re-checks that the job
// is still open and still saved-but-unapplied immediately before sending — a reminder
// that lost its intent (job closed, or the user applied/unsaved) is cancelled rather
// than delivered, which is how job-closure cancellation is enforced without hooking
// the scattered close paths.
func (r *Runner) fire(ctx context.Context, id int64, stats *Stats) {
	info, err := r.store.GetReminderForDelivery(ctx, id)
	if err != nil {
		log.Printf("reminder: load %d for delivery: %v", id, err)
		r.release(ctx, id)
		return
	}
	if !info.JobOpen || !info.StillActionable {
		if _, err := r.store.CancelReminderAtFire(ctx, id); err != nil {
			log.Printf("reminder: cancel-at-fire %d: %v", id, err)
			r.release(ctx, id)
			return
		}
		stats.Cancelled++
		return
	}

	msg := ReminderMessage{JobTitle: info.Title, Company: info.Company, Slug: info.PublicSlug, URL: info.URL}
	delivered, failedErr := r.deliverChannels(ctx, info, msg)

	switch {
	case delivered:
		if failedErr != nil {
			// One channel delivered but another errored: the reminder is done (a
			// one-shot nudge needs only one channel), but surface the broken channel
			// so a persistently-failing one is not invisible.
			log.Printf("reminder: %d delivered with a co-channel error: %v", id, failedErr)
		}
		if _, err := r.store.MarkReminderDelivered(ctx, id); err != nil {
			// Delivered but not stamped: the lease expiry re-delivers (a rare
			// duplicate), preferable to losing the reminder.
			log.Printf("reminder: mark delivered %d: %v", id, err)
		}
		stats.Delivered++
	case failedErr != nil:
		log.Printf("reminder: deliver %d: %v", id, failedErr)
		if err := r.store.RecordReminderDeliveryFailure(ctx, db.RecordReminderDeliveryFailureParams{
			ID:          id,
			LastError:   failedErr.Error(),
			MaxAttempts: r.cfg.MaxAttempts,
		}); err != nil {
			log.Printf("reminder: record failure %d: %v", id, err)
		}
		stats.Failed++
	default:
		// No channel had a usable destination (or none is configured): soft-skip,
		// keep the reminder pending for a later pass, burn no attempt.
		r.release(ctx, id)
		stats.SoftSkips++
	}
}

// deliverChannels attempts each of the reminder's channels that has a usable
// destination. It reports whether at least one send succeeded and the last hard
// error (a channel with no destination or no notifier is a soft-skip, not an error).
// One successful channel makes the reminder delivered — a co-channel outage just
// misses that channel for this one-shot nudge.
func (r *Runner) deliverChannels(ctx context.Context, info db.GetReminderForDeliveryRow, msg ReminderMessage) (delivered bool, failedErr error) {
	for _, ch := range info.Channels {
		dest, ok := recipient(ch, info)
		if !ok {
			continue
		}
		err := r.notifier.Send(ctx, ch, dest, msg)
		if errors.Is(err, ErrChannelNotConfigured) {
			continue
		}
		if err != nil {
			failedErr = err
			continue
		}
		delivered = true
	}
	return delivered, failedErr
}

// recipient resolves the destination for a channel, and whether the reminder is
// deliverable over it right now: telegram needs a linked chat, email the account
// email; anything else is undeliverable.
func recipient(channel string, info db.GetReminderForDeliveryRow) (string, bool) {
	switch channel {
	case notify.ChannelTelegram:
		if !info.TelegramChatID.Valid {
			return "", false
		}
		return strconv.FormatInt(info.TelegramChatID.Int64, 10), true
	case notify.ChannelEmail:
		if info.AccountEmail == "" {
			return "", false
		}
		return info.AccountEmail, true
	}
	return "", false
}

// release drops the lease on a reminder so it is retried promptly on a later pass.
func (r *Runner) release(ctx context.Context, id int64) {
	if err := r.store.ReleaseReminderClaim(ctx, id); err != nil {
		log.Printf("reminder: release claim %d: %v", id, err)
	}
}
