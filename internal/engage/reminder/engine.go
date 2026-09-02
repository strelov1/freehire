package reminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/deliverywindow"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ReminderMessage is the display shape of one saved-job reminder, rendered by a
// Notifier into a channel-specific message. No internal ids leak into it.
type ReminderMessage struct {
	JobTitle string
	Company  string
	Slug     string
	URL      string
}

// Notifier delivers one account's due reminders over a channel to a destination,
// as a single message. The engine depends only on this, so a new channel is a new
// implementation, not a change here.
//
// The slice, rather than a second batch method alongside a single-reminder one, is
// deliberate: a per-reminder send path left alive is a per-reminder send path some
// channel keeps using, and the flood this replaced was exactly that.
type Notifier interface {
	Send(ctx context.Context, channel, dest string, ms []ReminderMessage) error
}

// ErrChannelNotConfigured is returned by Router.Send for a channel with no
// registered notifier (e.g. email while SES is unconfigured). The engine treats it
// as a soft-skip, not a delivery failure.
var ErrChannelNotConfigured = errors.New("reminder: channel not configured")

// ErrRecipientGone is returned by a Notifier whose channel learned from the send
// that this recipient is permanently closed to us — the Telegram bot was blocked
// or removed. Like ErrChannelNotConfigured it is a soft-skip, and additionally the
// runner forgets the link, so no later reminder, digest or nudge tries that chat
// again. Mirrors notify.ErrRecipientGone; see the note there on why each engine
// declares its own rather than sharing the transport's.
var ErrRecipientGone = errors.New("reminder: recipient will not accept messages")

// Router dispatches a reminder to the notifier registered for its channel, so the
// engine stays channel-agnostic.
type Router map[string]Notifier

// Compile-time guarantee that Router is a Notifier.
var _ Notifier = (Router)(nil)

// Send routes to the registered notifier, or ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest string, ms []ReminderMessage) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, ms)
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
	// RecordNotification's id is discarded here: a reminder records after it is
	// delivered, so nothing needs to link to the row. Only the subscription
	// digest, which records before sending, uses it.
	RecordNotification(ctx context.Context, arg db.RecordNotificationParams) (int64, error)
	// DeleteTelegramLink forgets a user's Telegram chat, called when a send
	// reports it permanently closed (ErrRecipientGone).
	DeleteTelegramLink(ctx context.Context, userID int64) (int64, error)
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
	// SnapshotCap bounds how many reminders one message carries — which is how
	// many the in-app record holds. It is NOT the message listing bound (that is
	// notify.ListLimit): a message is short because a long list reads badly, while
	// the record is long because it is the page the message links to. The excess
	// is released back to the pending queue, so a later pass delivers it as its
	// own message rather than stamping it delivered into a message it never
	// appeared in.
	SnapshotCap int
}

// DefaultConfig is the production tuning, mirroring internal/engage/notify.
func DefaultConfig() Config {
	return Config{LeaseSeconds: 600, ClaimBatch: 500, MaxAttempts: 5, SnapshotCap: 200}
}

// Stats is the per-pass summary logged by the worker. Every counter except
// Messages counts REMINDERS, not deliveries, so the numbers stay comparable across
// the change that made one message carry many.
type Stats struct {
	Delivered int // reminders sent
	Messages  int // messages that landed (one per batch PER CHANNEL, not per batch)
	Cancelled int // reminders cancelled at fire (job closed or no longer actionable)
	SoftSkips int // reminders with no deliverable channel this pass
	Deferred  int // reminders held back by quiet hours or by a full message
	Failed    int // reminders whose delivery errored
}

// batch is one account's due reminders in a pass that share a channel set.
// Destinations and quiet hours are properties of the ACCOUNT, so the first
// member's row answers for the whole batch and only the messages accumulate.
type batch struct {
	info db.GetReminderForDeliveryRow
	ids  []int64
	msgs []ReminderMessage
}

// batchKey is what makes two reminders one message: the same account AND the same
// channel set. The channels are NOT an account property here — job_reminders
// snapshots them at schedule time so a later settings edit never rewrites a pending
// reminder (migration 0034) — so an account that changed channels between two saves
// has two genuinely different deliveries due. Grouping on the account alone would
// send one of them over the other's channels and stamp it delivered.
type batchKey struct {
	userID   int64
	channels string
}

// channelKey canonicalizes a channel set for batchKey: sorted and joined, so
// {email,telegram} and {telegram,email} are one group. Only the KEY is sorted — the
// send still walks the first member's own slice, preserving its order.
func channelKey(channels []string) string {
	sorted := slices.Clone(channels)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

// Runner fires due reminders.
type Runner struct {
	store    Store
	notifier Notifier
	cfg      Config
	now      func() time.Time
}

// NewRunner builds a firing Runner.
//
// A non-positive SnapshotCap is corrected to the default rather than honoured: it
// would make every batch full at zero members, so every claimed reminder is released
// unsent while burning no attempt — a pass that logs no failure and delivers nothing,
// forever. The other Config fields fail visibly when unset; this one does not.
func NewRunner(store Store, notifier Notifier, cfg Config) *Runner {
	if cfg.SnapshotCap <= 0 {
		cfg.SnapshotCap = DefaultConfig().SnapshotCap
	}
	return &Runner{store: store, notifier: notifier, cfg: cfg, now: time.Now}
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
	for _, b := range r.collect(ctx, due, &stats) {
		r.deliverBatch(ctx, b, &stats)
	}
	log.Printf("reminder: delivered=%d messages=%d cancelled=%d soft_skips=%d deferred=%d failed=%d",
		stats.Delivered, stats.Messages, stats.Cancelled, stats.SoftSkips, stats.Deferred, stats.Failed)
	return stats, nil
}

// collect turns the claimed ids into one batch per (account, channel set), dropping
// the reminders that must not be sent. Each is validated on its own — the checks are
// per reminder — and only the survivors group, so a stale reminder is cancelled
// without taking its neighbours' message with it.
//
// Batches are returned in the order their first member was claimed, which
// ClaimDueReminders orders by fire time, so the account that has waited longest is
// served first and a pass is reproducible.
func (r *Runner) collect(ctx context.Context, due []int64, stats *Stats) []*batch {
	byKey := make(map[batchKey]*batch, len(due))
	var order []*batch
	for _, id := range due {
		info, ok := r.validate(ctx, id, stats)
		if !ok {
			continue
		}
		key := batchKey{userID: info.UserID, channels: channelKey(info.Channels)}
		b := byKey[key]
		if b == nil {
			b = &batch{info: info}
			byKey[key] = b
			order = append(order, b)
		}
		if len(b.ids) >= r.cfg.SnapshotCap {
			// The message is full. Release rather than stamp: a reminder marked
			// delivered while appearing in no message is gone for good, which is
			// the trap internal/engage/notify's deferOverflow exists to avoid.
			r.release(ctx, id)
			stats.Deferred++
			continue
		}
		b.ids = append(b.ids, id)
		b.msgs = append(b.msgs, ReminderMessage{
			JobTitle: info.Title, Company: info.Company, Slug: info.PublicSlug, URL: info.URL,
		})
	}
	return order
}

// validate re-checks one claimed reminder immediately before it joins a batch: the
// job must still be open and still saved-but-unapplied, and the account must not be
// inside its quiet hours. A reminder that lost its intent is cancelled here, which
// is how job-closure cancellation is enforced without hooking the scattered close
// paths. It reports whether the reminder is deliverable this pass.
func (r *Runner) validate(ctx context.Context, id int64, stats *Stats) (db.GetReminderForDeliveryRow, bool) {
	info, err := r.store.GetReminderForDelivery(ctx, id)
	if err != nil {
		log.Printf("reminder: load %d for delivery: %v", id, err)
		r.release(ctx, id)
		return info, false
	}
	if !info.JobOpen || !info.StillActionable {
		if _, err := r.store.CancelReminderAtFire(ctx, id); err != nil {
			log.Printf("reminder: cancel-at-fire %d: %v", id, err)
			r.release(ctx, id)
			return info, false
		}
		stats.Cancelled++
		return info, false
	}
	if deliverywindow.InQuietHours(r.now(), info.Timezone.String, pgconv.DurationPtr(info.QuietHoursStart), pgconv.DurationPtr(info.QuietHoursEnd)) {
		// A transient time-of-day condition, not a lost intent: release (not
		// cancel) so the reminder fires once quiet hours end.
		r.release(ctx, id)
		stats.Deferred++
		return info, false
	}
	return info, true
}

// deliverBatch sends one account's batch as a single message per channel and
// finalizes every ledger row in it. The batch is the unit: one send outcome decides
// all of its reminders, because a partial result would need a second delivery
// ledger to describe and nothing reads one.
func (r *Runner) deliverBatch(ctx context.Context, b *batch, stats *Stats) {
	sent, failedErr := r.deliverChannels(ctx, b.info, b.msgs)

	switch {
	case sent > 0:
		if failedErr != nil {
			// One channel delivered but another errored: the batch is done (a
			// one-shot nudge needs only one channel), but surface the broken channel
			// so a persistently-failing one is not invisible.
			log.Printf("reminder: user %d delivered with a co-channel error: %v", b.info.UserID, failedErr)
		}
		for _, id := range b.ids {
			if _, err := r.store.MarkReminderDelivered(ctx, id); err != nil {
				// Delivered but not stamped: the lease expiry re-delivers (a rare
				// duplicate), preferable to losing the reminder.
				log.Printf("reminder: mark delivered %d: %v", id, err)
			}
		}
		r.recordNotification(ctx, b)
		stats.Delivered += len(b.ids)
		stats.Messages += sent
	case failedErr != nil:
		log.Printf("reminder: deliver batch for user %d: %v", b.info.UserID, failedErr)
		for _, id := range b.ids {
			if err := r.store.RecordReminderDeliveryFailure(ctx, db.RecordReminderDeliveryFailureParams{
				ID:          id,
				LastError:   failedErr.Error(),
				MaxAttempts: r.cfg.MaxAttempts,
			}); err != nil {
				log.Printf("reminder: record failure %d: %v", id, err)
			}
		}
		stats.Failed += len(b.ids)
	default:
		// No channel had a usable destination (or none is configured): soft-skip,
		// keep the reminders pending for a later pass, burn no attempt.
		for _, id := range b.ids {
			r.release(ctx, id)
		}
		stats.SoftSkips += len(b.ids)
	}
}

// deliverChannels attempts each of the account's channels that has a usable
// destination, sending the whole batch as one message per channel. It reports HOW MANY
// messages landed — a batch on email and Telegram is two, and the pass summary would
// undercount if this were a bool — and the last hard error (a channel with no
// destination or no notifier is a soft-skip, not an error). One successful channel
// makes the batch delivered — a co-channel outage just misses that channel for these
// one-shot nudges.
func (r *Runner) deliverChannels(ctx context.Context, info db.GetReminderForDeliveryRow, msgs []ReminderMessage) (sent int, failedErr error) {
	for _, ch := range info.Channels {
		dest, ok := recipient(ch, info)
		if !ok {
			continue
		}
		err := r.notifier.Send(ctx, ch, dest, msgs)
		if errors.Is(err, ErrChannelNotConfigured) {
			continue
		}
		// The chat is closed to us for good. Forget it and skip this channel
		// without recording a failure: retrying cannot reach it, and a reminder
		// whose OTHER channel worked is still delivered.
		if errors.Is(err, ErrRecipientGone) {
			r.unlinkTelegram(ctx, info.UserID, err)
			continue
		}
		if err != nil {
			failedErr = err
			continue
		}
		sent++
	}
	return sent, failedErr
}

// unlinkTelegram forgets a user's Telegram chat after a send reported it
// permanently closed, logging it because this changes the user's settings without
// them asking. A failure here is logged and no more: the send is skipped either
// way, and the next reminder meets the same 403 and tries again.
func (r *Runner) unlinkTelegram(ctx context.Context, userID int64, cause error) {
	rows, err := r.store.DeleteTelegramLink(ctx, userID)
	if err != nil {
		log.Printf("reminder: unlink telegram for user %d: %v", userID, err)
		return
	}
	if rows > 0 {
		log.Printf("reminder: unlinked telegram for user %d: %v", userID, cause)
	}
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
	case notify.ChannelPush:
		if !info.HasPushDevice {
			return "", false
		}
		return strconv.FormatInt(info.UserID, 10), true
	}
	return "", false
}

// recordNotification writes the one in-app notification-center record for a
// delivered batch. A batch of one keeps the shape reminders have always recorded —
// a public slug the notification links straight to. A batch of several has no
// single job to point at, so it carries the job list instead and the notification
// center renders its own page, exactly as a multi-job subscription digest does.
// A failure here must not fail the delivery it accompanies — the batch was already
// sent, so this is logged and dropped, not propagated.
func (r *Runner) recordNotification(ctx context.Context, b *batch) {
	title, body := renderReminderBatch(b.msgs)
	arg := db.RecordNotificationParams{
		UserID: b.info.UserID,
		Kind:   "reminder",
		Title:  title,
		Body:   body,
	}
	if len(b.msgs) == 1 {
		arg.PublicSlug = pgtype.Text{String: b.msgs[0].Slug, Valid: true}
	} else {
		arg.Jobs = jobsSnapshot(b.msgs)
	}
	if _, err := r.store.RecordNotification(ctx, arg); err != nil {
		log.Printf("reminder: record notification for user %d: %v", b.info.UserID, err)
	}
}

// jobsSnapshot is the job list a multi-job batch records for its notification's own
// page, in the shape notify owns because one page renders every engine's rows.
func jobsSnapshot(ms []ReminderMessage) json.RawMessage {
	jobs := make([]notify.SnapshotJob, len(ms))
	for i, m := range ms {
		jobs[i] = notify.SnapshotJob{Title: m.JobTitle, Company: m.Company, Slug: m.Slug}
	}
	return notify.JobsSnapshot(jobs)
}

// release drops the lease on a reminder so it is retried promptly on a later pass.
func (r *Runner) release(ctx context.Context, id int64) {
	if err := r.store.ReleaseReminderClaim(ctx, id); err != nil {
		log.Printf("reminder: release claim %d: %v", id, err)
	}
}
