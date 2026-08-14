// Package nudge is the lifecycle-notification decision layer: it watches tracked
// applications and drives three one-shot nudges — follow-up (an application has
// gone silent past its stage's tolerated threshold, per userjob.SilenceStateFor),
// interview-prep (a stage_set moved an application into `interview`), and
// job-closed (a listing the candidate is still actively tracking closed) — over
// the same account-level notification_settings rule internal/reminder reads.
//
// One pass (Runner.Run) does MATCH then DELIVER, mirroring internal/notify: MATCH
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
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/userjob"
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

// Notifier delivers a nudge over a channel to a destination. The engine depends
// only on this, so a new channel is a new implementation, not a change here.
type Notifier interface {
	Send(ctx context.Context, channel, dest string, m Message) error
}

// ErrChannelNotConfigured is returned by Router.Send for a channel with no
// registered notifier. The engine treats it as a soft-skip, not a delivery failure.
var ErrChannelNotConfigured = errors.New("nudge: channel not configured")

// Router dispatches a nudge to the notifier registered for its channel, so the
// engine stays channel-agnostic.
type Router map[string]Notifier

// Compile-time guarantee that Router is a Notifier.
var _ Notifier = (Router)(nil)

// Send routes to the registered notifier, or ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest string, m Message) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, m)
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
	// RecordNotification writes the in-app notification-center row for a
	// delivered nudge. Called from fire right after MarkNudgeDelivered; a
	// failure must never fail the delivery it accompanies (see
	// add-notification-center's design).
	RecordNotification(ctx context.Context, arg db.RecordNotificationParams) error
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
}

// DefaultConfig is the production tuning, mirroring internal/reminder and
// internal/notify.
func DefaultConfig() Config {
	return Config{
		FollowUpWindowDays:      30,
		InterviewPrepWindowDays: 7,
		JobClosedWindowDays:     7,
		LeaseSeconds:            600,
		ClaimBatch:              500,
		MaxAttempts:             5,
	}
}

// Stats is the per-pass summary logged by the worker.
type Stats struct {
	Matched   int // newly recorded nudge candidates
	Delivered int // nudges sent
	Cancelled int // nudges cancelled at fire (condition no longer holds)
	SoftSkips int // nudges with no deliverable channel this pass
	Failed    int // nudges whose delivery errored
}

// Runner executes MATCH+DELIVER passes.
type Runner struct {
	store    Store
	notifier Notifier
	cfg      Config
	now      func() time.Time
}

// New builds a Runner.
func New(store Store, notifier Notifier, cfg Config) *Runner {
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
	log.Printf("nudge: matched=%d delivered=%d cancelled=%d soft_skips=%d failed=%d",
		stats.Matched, stats.Delivered, stats.Cancelled, stats.SoftSkips, stats.Failed)
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
		days := userjob.DaysSilent(r.now(), c.LastActivityAt.Time)
		if userjob.SilenceStateFor(stage, days, c.HasPendingSuggestion) != userjob.SilenceSilent {
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
		if _, active := userjob.SilenceThresholdDays(stage); !active {
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
	for _, id := range due {
		r.fire(ctx, id, stats)
	}
	return nil
}

// fire re-checks one claimed nudge's triggering condition and delivers or cancels
// it. The re-check is what lets MATCH re-run freely without ever sending a nudge
// whose reason has since gone away.
func (r *Runner) fire(ctx context.Context, id int64, stats *Stats) {
	info, err := r.store.GetNudgeForDelivery(ctx, id)
	if err != nil {
		log.Printf("nudge: load %d for delivery: %v", id, err)
		r.release(ctx, id)
		return
	}
	if !r.actionable(info) {
		if _, err := r.store.CancelNudgeAtFire(ctx, id); err != nil {
			log.Printf("nudge: cancel-at-fire %d: %v", id, err)
			r.release(ctx, id)
			return
		}
		stats.Cancelled++
		return
	}

	msg := Message{Kind: info.Kind, JobTitle: info.Title, Company: info.Company, Slug: info.PublicSlug, URL: info.URL}
	if info.Kind == KindFollowUp && info.LastActivityAt.Valid {
		msg.DaysSilent = userjob.DaysSilent(r.now(), info.LastActivityAt.Time)
	}
	delivered, failedErr := r.deliverChannels(ctx, info, msg)

	switch {
	case delivered:
		if failedErr != nil {
			log.Printf("nudge: %d delivered with a co-channel error: %v", id, failedErr)
		}
		if _, err := r.store.MarkNudgeDelivered(ctx, id); err != nil {
			log.Printf("nudge: mark delivered %d: %v", id, err)
		}
		r.recordNotification(ctx, id, info, msg)
		stats.Delivered++
	case failedErr != nil:
		log.Printf("nudge: deliver %d: %v", id, failedErr)
		if err := r.store.RecordNudgeDeliveryFailure(ctx, db.RecordNudgeDeliveryFailureParams{
			ID:          id,
			LastError:   failedErr.Error(),
			MaxAttempts: r.cfg.MaxAttempts,
		}); err != nil {
			log.Printf("nudge: record failure %d: %v", id, err)
		}
		stats.Failed++
	default:
		r.release(ctx, id)
		stats.SoftSkips++
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
			days = userjob.DaysSilent(r.now(), info.LastActivityAt.Time)
		}
		return userjob.SilenceStateFor(stage, days, info.HasPendingSuggestion) == userjob.SilenceSilent
	case KindInterviewPrep:
		return info.JobOpen && info.ApplicationExists && stage == "interview"
	case KindJobClosed:
		if !info.ApplicationExists {
			return false
		}
		_, active := userjob.SilenceThresholdDays(stage)
		return active
	default:
		return false
	}
}

// deliverChannels attempts each configured channel that has a usable destination.
// It reports whether at least one send succeeded and the last hard error (a
// channel with no destination or no notifier is a soft-skip, not an error). One
// successful channel makes the nudge delivered — a co-channel outage just misses
// that channel for this one-shot nudge.
func (r *Runner) deliverChannels(ctx context.Context, info db.GetNudgeForDeliveryRow, msg Message) (delivered bool, failedErr error) {
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

// recordNotification writes the in-app notification-center row for a delivered
// nudge, reusing renderNudge's title/body — the same copy the push channel
// already shows — so the in-app record never drifts from it. A nudge always
// concerns exactly one job, so PublicSlug is always populated. A failure here
// must not fail the delivery: the nudge was already sent and marked delivered;
// losing the in-app record is a degraded read-side feature, not a reason to
// treat the delivery as failed — the same posture this func's caller already
// takes toward MarkNudgeDelivered's own failure, just above.
func (r *Runner) recordNotification(ctx context.Context, id int64, info db.GetNudgeForDeliveryRow, msg Message) {
	title, body := renderNudge(msg)
	if err := r.store.RecordNotification(ctx, db.RecordNotificationParams{
		UserID:     info.UserID,
		Kind:       "nudge_" + msg.Kind,
		Title:      title,
		Body:       body,
		PublicSlug: pgtype.Text{String: msg.Slug, Valid: true},
	}); err != nil {
		log.Printf("nudge: record notification %d: %v", id, err)
	}
}

// release drops the lease on a nudge so it is retried promptly on a later pass.
func (r *Runner) release(ctx context.Context, id int64) {
	if err := r.store.ReleaseNudgeClaim(ctx, id); err != nil {
		log.Printf("nudge: release claim %d: %v", id, err)
	}
}
