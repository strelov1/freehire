// Package nudge is the lifecycle-notification decision layer: it watches tracked
// applications and drives three one-shot nudges — follow-up (an application has
// gone silent past its stage's tolerated threshold, per silence.StateFor),
// interview-prep (a stage_set moved an application into `interview`), and
// job-closed (a listing the candidate is still actively tracking closed) — over
// the same account-level notification_settings rule internal/engage/reminder reads.
//
// One pass (Runner.Run) does MATCH then DELIVER, mirroring internal/engage/notify: MATCH
// re-scans current state and records new candidates in the application_nudges
// ledger; DELIVER leases pending nudges, re-verifies the triggering condition still
// holds (a matched nudge whose condition lapsed by delivery time is cancelled, not
// sent), and delivers over the user's configured channels.
//
// Dedup rides the ledger's unique key rather than a snooze or a notified-count: an
// "episode key" — the fact that must change before a re-notify is warranted — is
// part of the key alongside (user, job, kind). For follow-up it is the
// application's last_activity_at at match time (silence dragging on doesn't change
// it; a reply does); for interview-prep it is the triggering stage_set event's
// occurred_at (a later interview round is a new event, a new episode). See the
// centralize-lifecycle-notifications change.
package nudge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/deliverywindow"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/job/silence"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Nudge kinds. Also the CHECK constraint on application_nudges.kind (migrations
// 0083, widened by 0084).
const (
	KindFollowUp      = "follow_up"
	KindInterviewPrep = "interview_prep"
	KindJobClosed     = "job_closed"
)

// Message is the display shape of one nudge, rendered by a Notifier into a
// channel-specific message. No internal ids leak into it. DaysSilent is set for
// KindFollowUp only.
type Message struct {
	Kind       string
	JobTitle   string
	Company    string
	Slug       string
	URL        string
	DaysSilent int
}

// Notifier delivers one account's due nudges OF ONE KIND over a channel to a
// destination, as a single message. The engine depends only on this, so a new
// channel is a new implementation, not a change here.
//
// The kind travels beside the slice rather than being read off the first message,
// because it is a property of the group: every member shares it, and the renderer
// switches on it before it looks at any job.
type Notifier interface {
	Send(ctx context.Context, channel, dest, kind string, ms []Message) error
}

// ErrChannelNotConfigured is returned by Router.Send for a channel with no
// registered notifier. The engine treats it as a soft-skip, not a delivery failure.
var ErrChannelNotConfigured = errors.New("nudge: channel not configured")

// ErrRecipientGone is returned by a Notifier whose channel learned from the send
// that this recipient is permanently closed to us — the Telegram bot was blocked
// or removed. Like ErrChannelNotConfigured it is a soft-skip, and additionally the
// runner forgets the link, so no later nudge, digest or reminder tries that chat
// again. Mirrors notify.ErrRecipientGone; see the note there on why each engine
// declares its own rather than sharing the transport's.
var ErrRecipientGone = errors.New("nudge: recipient will not accept messages")

// Router dispatches a nudge to the notifier registered for its channel, so the
// engine stays channel-agnostic.
type Router map[string]Notifier

// Compile-time guarantee that Router is a Notifier.
var _ Notifier = (Router)(nil)

// Send routes to the registered notifier, or ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest, kind string, ms []Message) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, kind, ms)
}

// Compile-time proof that the generated *db.Queries satisfies the engine's Store.
var _ Store = (*db.Queries)(nil)

// Store is the persistence the engine needs. *db.Queries satisfies it directly.
type Store interface {
	ListFollowUpCandidates(ctx context.Context, windowDays int32) ([]db.ListFollowUpCandidatesRow, error)
	ListInterviewPrepCandidates(ctx context.Context, windowDays int32) ([]db.ListInterviewPrepCandidatesRow, error)
	ListJobClosedCandidates(ctx context.Context, windowDays int32) ([]db.ListJobClosedCandidatesRow, error)
	RecordNudge(ctx context.Context, arg db.RecordNudgeParams) (int64, error)
	// TrackJob is jobtracking's own stage-set write (upserts applications.stage and
	// emits the paired application_events row in one statement, only when the stage
	// actually moves). MATCH reuses it directly, rather than duplicating the CTE,
	// to auto-settle an application whose listing closed — see the job-closed loop.
	TrackJob(ctx context.Context, arg db.TrackJobParams) (db.TrackJobRow, error)
	ClaimDueNudges(ctx context.Context, arg db.ClaimDueNudgesParams) ([]int64, error)
	GetNudgeForDelivery(ctx context.Context, id int64) (db.GetNudgeForDeliveryRow, error)
	MarkNudgeDelivered(ctx context.Context, id int64) (int64, error)
	CancelNudgeAtFire(ctx context.Context, id int64) (int64, error)
	RecordNudgeDeliveryFailure(ctx context.Context, arg db.RecordNudgeDeliveryFailureParams) error
	ReleaseNudgeClaim(ctx context.Context, id int64) error
	// DeleteTelegramLink forgets a user's Telegram chat, called when a send
	// reports it permanently closed (ErrRecipientGone).
	DeleteTelegramLink(ctx context.Context, userID int64) (int64, error)
	// RecordNotification writes the in-app notification-center row for a
	// delivered nudge. Called from fire right after MarkNudgeDelivered; a
	// failure must never fail the delivery it accompanies (see
	// add-notification-center's design). The returned id is only of use to
	// the subscription digest, which records before it sends so its message can
	// link to the row; a nudge records after and discards it.
	RecordNotification(ctx context.Context, arg db.RecordNotificationParams) (int64, error)
}

// Config tunes one pass. Defaults come from DefaultConfig.
type Config struct {
	// FollowUpWindowDays bounds how far back ListFollowUpCandidates looks at
	// last_activity_at, so a first deploy does not detonate the entire historical
	// backlog as nudges.
	FollowUpWindowDays int32
	// InterviewPrepWindowDays is the same bound on ListInterviewPrepCandidates'
	// occurred_at.
	InterviewPrepWindowDays int32
	// JobClosedWindowDays is the same bound on ListJobClosedCandidates' closed_at.
	JobClosedWindowDays int32
	// LeaseSeconds is the delivery lease: a claimed-but-unfinished nudge is
	// reclaimable after this, which doubles as the crash reaper.
	LeaseSeconds int32
	// ClaimBatch bounds how many pending nudges one pass delivers.
	ClaimBatch int32
	// MaxAttempts dead-letters a nudge after this many failed deliveries.
	MaxAttempts int32
	// SnapshotCap bounds how many nudges one message carries — which is how many
	// the in-app record holds. It is NOT the message listing bound (that is
	// notify.ListLimit): a message is short because a long list reads badly, while
	// the record is long because it is the page the message links to. The excess is
	// released back to the pending queue rather than stamped delivered into a
	// message it never appeared in.
	SnapshotCap int
}

// DefaultConfig is the production tuning, mirroring internal/engage/reminder and
// internal/engage/notify.
func DefaultConfig() Config {
	return Config{
		FollowUpWindowDays:      30,
		InterviewPrepWindowDays: 7,
		JobClosedWindowDays:     7,
		LeaseSeconds:            600,
		ClaimBatch:              500,
		MaxAttempts:             5,
		SnapshotCap:             200,
	}
}

// Stats is the per-pass summary logged by the worker. Every counter except
// Messages counts NUDGES, not deliveries, so the numbers stay comparable across
// the change that made one message carry many.
type Stats struct {
	Matched   int // newly recorded nudge candidates
	Delivered int // nudges sent
	Messages  int // messages that landed (one per batch PER CHANNEL, not per batch)
	Cancelled int // nudges cancelled at fire (condition no longer holds)
	SoftSkips int // nudges with no deliverable channel this pass
	Deferred  int // nudges held back by quiet hours or by a full message
	Failed    int // nudges whose delivery errored
}

// batch is one account's due nudges OF ONE KIND in a pass. Channels, destinations
// and quiet hours are properties of the ACCOUNT and the kind is shared by
// construction, so the first member's row answers for the whole batch and only the
// messages accumulate.
type batch struct {
	info db.GetNudgeForDeliveryRow
	ids  []int64
	msgs []Message
}

// batchKey is what makes two nudges one message: the same account AND the same
// kind. Merging the kinds would put "your application went quiet" and "prepare for
// your interview" in one mail, which are different errands with different actions.
type batchKey struct {
	userID int64
	kind   string
}

// Runner executes MATCH+DELIVER passes.
type Runner struct {
	store    Store
	notifier Notifier
	cfg      Config
	now      func() time.Time
}

// New builds a Runner.
//
// A non-positive SnapshotCap is corrected to the default rather than honoured: it
// would make every batch full at zero members, so every claimed nudge is released
// unsent while burning no attempt — a pass that logs no failure and delivers nothing,
// forever. The other Config fields fail visibly when unset; this one does not.
func New(store Store, notifier Notifier, cfg Config) *Runner {
	if cfg.SnapshotCap <= 0 {
		cfg.SnapshotCap = DefaultConfig().SnapshotCap
	}
	return &Runner{store: store, notifier: notifier, cfg: cfg, now: time.Now}
}

// Run executes one MATCH-then-DELIVER pass. MATCH records new candidates; DELIVER
// drains the pending queue. Unsent nudges are retried by the next pass, so a
// delivery outage loses nothing.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := r.match(ctx, &stats); err != nil {
		return stats, fmt.Errorf("match: %w", err)
	}
	if err := r.deliver(ctx, &stats); err != nil {
		return stats, fmt.Errorf("deliver: %w", err)
	}
	log.Printf("nudge: matched=%d delivered=%d messages=%d cancelled=%d soft_skips=%d deferred=%d failed=%d",
		stats.Matched, stats.Delivered, stats.Messages, stats.Cancelled, stats.SoftSkips, stats.Deferred, stats.Failed)
	return stats, nil
}

// match records every current follow-up, interview-prep, and job-closed candidate.
// Re-running over an unchanged episode is a no-op (the ledger's unique key), so
// this can freely re-scan the same candidates every pass.
func (r *Runner) match(ctx context.Context, stats *Stats) error {
	candidates, err := r.store.ListFollowUpCandidates(ctx, r.cfg.FollowUpWindowDays)
	if err != nil {
		return fmt.Errorf("list follow-up candidates: %w", err)
	}
	for _, c := range candidates {
		if !c.JobID.Valid || !c.LastActivityAt.Valid {
			continue // defensive: the query's JOINs already guarantee both are set
		}
		stage := ""
		if c.Stage.Valid {
			stage = c.Stage.String
		}
		days := silence.Days(r.now(), c.LastActivityAt.Time)
		if silence.StateFor(stage, days, c.HasPendingSuggestion) != silence.Silent {
			continue
		}
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: c.UserID, JobID: c.JobID.Int64, Kind: KindFollowUp,
			EpisodeKey: c.LastActivityAt,
		})
		if err != nil {
			return fmt.Errorf("record follow-up nudge: %w", err)
		}
		stats.Matched += int(affected)
	}

	events, err := r.store.ListInterviewPrepCandidates(ctx, r.cfg.InterviewPrepWindowDays)
	if err != nil {
		return fmt.Errorf("list interview-prep candidates: %w", err)
	}
	for _, e := range events {
		if !e.JobID.Valid || !e.OccurredAt.Valid {
			continue // defensive: the query's WHERE clause already guarantees both are set
		}
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: e.UserID, JobID: e.JobID.Int64, Kind: KindInterviewPrep,
			EpisodeKey: e.OccurredAt,
		})
		if err != nil {
			return fmt.Errorf("record interview-prep nudge: %w", err)
		}
		stats.Matched += int(affected)
	}

	closed, err := r.store.ListJobClosedCandidates(ctx, r.cfg.JobClosedWindowDays)
	if err != nil {
		return fmt.Errorf("list job-closed candidates: %w", err)
	}
	for _, c := range closed {
		if !c.JobID.Valid || !c.ClosedAt.Valid {
			continue // defensive: the query's WHERE clause already guarantees both are set
		}
		stage := ""
		if c.Stage.Valid {
			stage = c.Stage.String
		}
		if _, active := silence.ThresholdDays(stage); !active {
			continue // a settled application does not care that the listing closed
		}
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: c.UserID, JobID: c.JobID.Int64, Kind: KindJobClosed,
			EpisodeKey: c.ClosedAt,
		})
		if err != nil {
			return fmt.Errorf("record job-closed nudge: %w", err)
		}
		stats.Matched += int(affected)

		// Auto-settle the board: a closed listing is not something to leave sitting
		// on an active stage waiting for the candidate to notice and clear by hand.
		// Ordered AFTER RecordNudge — TrackJob only writes a stage_set event when the
		// stage actually moves, so it is naturally idempotent on retry, but running it
		// first would flip the stage to `expired` before the notification is durably
		// recorded; a failure between the two would then leave the candidate never
		// notified and the next pass no longer finding an active stage to re-check.
		if _, err := r.store.TrackJob(ctx, db.TrackJobParams{
			UserID: c.UserID, JobID: c.JobID.Int64,
			Stage:       pgtype.Text{String: "expired", Valid: true},
			EventSource: appevent.SourceSystem,
		}); err != nil {
			return fmt.Errorf("auto-expire job-closed application: %w", err)
		}
	}
	return nil
}

// deliver leases due nudges and fires each.
func (r *Runner) deliver(ctx context.Context, stats *Stats) error {
	due, err := r.store.ClaimDueNudges(ctx, db.ClaimDueNudgesParams{
		LeaseSeconds: r.cfg.LeaseSeconds,
		BatchSize:    r.cfg.ClaimBatch,
	})
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	for _, b := range r.collect(ctx, due, stats) {
		r.deliverBatch(ctx, b, stats)
	}
	return nil
}

// collect turns the claimed ids into one batch per (account, kind), dropping the
// nudges that must not be sent. Each is re-checked on its own — the trigger is per
// nudge — and only the survivors group, so a lapsed nudge is cancelled without
// taking its neighbours' message with it.
//
// Batches are returned in the order their first member was claimed, which
// ClaimDueNudges orders by creation time, so the nudge that has waited longest is
// served first and a pass is reproducible.
func (r *Runner) collect(ctx context.Context, due []int64, stats *Stats) []*batch {
	byKey := make(map[batchKey]*batch, len(due))
	var order []*batch
	for _, id := range due {
		info, ok := r.validate(ctx, id, stats)
		if !ok {
			continue
		}
		key := batchKey{userID: info.UserID, kind: info.Kind}
		b := byKey[key]
		if b == nil {
			b = &batch{info: info}
			byKey[key] = b
			order = append(order, b)
		}
		if len(b.ids) >= r.cfg.SnapshotCap {
			// The message is full. Release rather than stamp: a nudge marked
			// delivered while appearing in no message is gone for good.
			r.release(ctx, id)
			stats.Deferred++
			continue
		}
		b.ids = append(b.ids, id)
		b.msgs = append(b.msgs, r.message(info))
	}
	return order
}

// validate re-checks one claimed nudge's triggering condition immediately before it
// joins a batch, and holds it back inside the account's quiet hours. The re-check is
// what lets MATCH re-run freely without ever sending a nudge whose reason has since
// gone away. It reports whether the nudge is deliverable this pass.
func (r *Runner) validate(ctx context.Context, id int64, stats *Stats) (db.GetNudgeForDeliveryRow, bool) {
	info, err := r.store.GetNudgeForDelivery(ctx, id)
	if err != nil {
		log.Printf("nudge: load %d for delivery: %v", id, err)
		r.release(ctx, id)
		return info, false
	}
	if !r.actionable(info) {
		if _, err := r.store.CancelNudgeAtFire(ctx, id); err != nil {
			log.Printf("nudge: cancel-at-fire %d: %v", id, err)
			r.release(ctx, id)
			return info, false
		}
		stats.Cancelled++
		return info, false
	}
	if deliverywindow.InQuietHours(r.now(), info.Timezone.String, pgconv.DurationPtr(info.QuietHoursStart), pgconv.DurationPtr(info.QuietHoursEnd)) {
		// A transient time-of-day condition, not a lapsed trigger: release (not
		// cancel) so the nudge fires once quiet hours end.
		r.release(ctx, id)
		stats.Deferred++
		return info, false
	}
	return info, true
}

// message projects one delivery row into the display shape a channel renders.
func (r *Runner) message(info db.GetNudgeForDeliveryRow) Message {
	msg := Message{Kind: info.Kind, JobTitle: info.Title, Company: info.Company, Slug: info.PublicSlug, URL: info.URL}
	if info.Kind == KindFollowUp && info.LastActivityAt.Valid {
		msg.DaysSilent = silence.Days(r.now(), info.LastActivityAt.Time)
	}
	return msg
}

// deliverBatch sends one (account, kind) batch as a single message per channel and
// finalizes every ledger row in it. The batch is the unit: one send outcome decides
// all of its nudges, because a partial result would need a second delivery ledger
// to describe and nothing reads one.
func (r *Runner) deliverBatch(ctx context.Context, b *batch, stats *Stats) {
	sent, failedErr := r.deliverChannels(ctx, b.info, b.msgs)

	switch {
	case sent > 0:
		if failedErr != nil {
			log.Printf("nudge: user %d %s delivered with a co-channel error: %v", b.info.UserID, b.info.Kind, failedErr)
		}
		for _, id := range b.ids {
			if _, err := r.store.MarkNudgeDelivered(ctx, id); err != nil {
				log.Printf("nudge: mark delivered %d: %v", id, err)
			}
		}
		r.recordNotification(ctx, b)
		stats.Delivered += len(b.ids)
		stats.Messages += sent
	case failedErr != nil:
		log.Printf("nudge: deliver %s batch for user %d: %v", b.info.Kind, b.info.UserID, failedErr)
		for _, id := range b.ids {
			if err := r.store.RecordNudgeDeliveryFailure(ctx, db.RecordNudgeDeliveryFailureParams{
				ID:          id,
				LastError:   failedErr.Error(),
				MaxAttempts: r.cfg.MaxAttempts,
			}); err != nil {
				log.Printf("nudge: record failure %d: %v", id, err)
			}
		}
		stats.Failed += len(b.ids)
	default:
		for _, id := range b.ids {
			r.release(ctx, id)
		}
		stats.SoftSkips += len(b.ids)
	}
}

// actionable re-derives the triggering condition from the live delivery context: a
// disabled notification rule cancels every kind. follow-up and interview-prep
// additionally need the job still open (closing settles the application one way
// or another) and their own live condition; job-closed needs the opposite — the
// job stays closed once closed, so it only needs the application to still be in a
// stage that accrues silence (a settled one no longer cares that the listing shut).
func (r *Runner) actionable(info db.GetNudgeForDeliveryRow) bool {
	if !info.NotificationsEnabled {
		return false
	}
	stage := ""
	if info.Stage.Valid {
		stage = info.Stage.String
	}
	switch info.Kind {
	case KindFollowUp:
		if !info.JobOpen || !info.ApplicationExists {
			return false
		}
		days := 0
		if info.LastActivityAt.Valid {
			days = silence.Days(r.now(), info.LastActivityAt.Time)
		}
		return silence.StateFor(stage, days, info.HasPendingSuggestion) == silence.Silent
	case KindInterviewPrep:
		return info.JobOpen && info.ApplicationExists && stage == "interview"
	case KindJobClosed:
		if !info.ApplicationExists {
			return false
		}
		_, active := silence.ThresholdDays(stage)
		return active
	default:
		return false
	}
}

// deliverChannels attempts each configured channel that has a usable destination,
// sending the whole batch as one message per channel. It reports HOW MANY messages
// landed — a batch on email and Telegram is two, and the pass summary would
// undercount if this were a bool — and the last hard error (a channel with no
// destination or no notifier is a soft-skip, not an error). One successful channel
// makes the batch delivered — a co-channel outage just misses that channel for these
// one-shot nudges.
func (r *Runner) deliverChannels(ctx context.Context, info db.GetNudgeForDeliveryRow, msgs []Message) (sent int, failedErr error) {
	for _, ch := range info.Channels {
		dest, ok := recipient(ch, info)
		if !ok {
			continue
		}
		err := r.notifier.Send(ctx, ch, dest, info.Kind, msgs)
		if errors.Is(err, ErrChannelNotConfigured) {
			continue
		}
		// The chat is closed to us for good. Forget it and skip this channel
		// without recording a failure: retrying cannot reach it, and a nudge whose
		// OTHER channel worked is still delivered.
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
// way, and the next nudge meets the same 403 and tries again.
func (r *Runner) unlinkTelegram(ctx context.Context, userID int64, cause error) {
	rows, err := r.store.DeleteTelegramLink(ctx, userID)
	if err != nil {
		log.Printf("nudge: unlink telegram for user %d: %v", userID, err)
		return
	}
	if rows > 0 {
		log.Printf("nudge: unlinked telegram for user %d: %v", userID, cause)
	}
}

// recipient resolves the destination for a channel, and whether the nudge is
// deliverable over it right now: telegram needs a linked chat, email the account
// email; anything else is undeliverable.
func recipient(channel string, info db.GetNudgeForDeliveryRow) (string, bool) {
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

// recordNotification writes the one in-app notification-center row for a delivered
// batch, reusing renderNudgeBatch's title/body — the same copy the push channel
// shows — so the in-app record never drifts from it. A batch of one keeps the shape
// nudges have always recorded, a public slug the notification links straight to; a
// batch of several has no single job to point at, so it carries the job list and the
// notification center renders its own page.
//
// A failure here must not fail the delivery: the batch was already sent and marked
// delivered; losing the in-app record is a degraded read-side feature, not a reason
// to treat the delivery as failed — the same posture this func's caller already
// takes toward MarkNudgeDelivered's own failure, just above.
func (r *Runner) recordNotification(ctx context.Context, b *batch) {
	title, body := renderNudgeBatch(b.info.Kind, b.msgs)
	arg := db.RecordNotificationParams{
		UserID: b.info.UserID,
		Kind:   "nudge_" + b.info.Kind,
		Title:  title,
		Body:   body,
	}
	if len(b.msgs) == 1 {
		arg.PublicSlug = pgtype.Text{String: b.msgs[0].Slug, Valid: true}
	} else {
		arg.Jobs = jobsSnapshot(b.msgs)
	}
	if _, err := r.store.RecordNotification(ctx, arg); err != nil {
		log.Printf("nudge: record notification for user %d (%s): %v", b.info.UserID, b.info.Kind, err)
	}
}

// jobsSnapshot is the job list a multi-job batch records for its notification's own
// page, in the shape notify owns because one page renders every engine's rows.
func jobsSnapshot(ms []Message) json.RawMessage {
	jobs := make([]notify.SnapshotJob, len(ms))
	for i, m := range ms {
		jobs[i] = notify.SnapshotJob{Title: m.JobTitle, Company: m.Company, Slug: m.Slug}
	}
	return notify.JobsSnapshot(jobs)
}

// release drops the lease on a nudge so it is retried promptly on a later pass.
func (r *Runner) release(ctx context.Context, id int64) {
	if err := r.store.ReleaseNudgeClaim(ctx, id); err != nil {
		log.Printf("nudge: release claim %d: %v", id, err)
	}
}
