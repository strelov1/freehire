// Package reminder is the saved-job reminder use case: a signed-in user turns on
// reminders and picks a default delay and channels (the account rule), and each
// save schedules a one-shot nudge to come back before the vacancy goes stale. It
// owns rule validation and the schedule/cancel decisions; the firing engine
// (Runner, in this package) reads the same job_reminders ledger and delivers due
// reminders. This mirrors internal/subscription (the HTTP-facing use case) +
// internal/notify (the delivery worker), kept in one package because a reminder is
// a single small concept.
package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/notify"
)

// DefaultDelayDays is the pre-filled default for a rule that was never configured,
// so the settings UI shows a sensible value before the user opts in.
const DefaultDelayDays = 3

// Delay bounds. A reminder any sooner than a day is not a "come back later" nudge;
// a year is the far end of anything useful.
const (
	minDelayDays = 1
	maxDelayDays = 365
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidChannel is an unsupported delivery channel (mapped to 400).
	ErrInvalidChannel = errors.New("reminder: unsupported channel")
	// ErrInvalidDelay is a delay outside [minDelayDays, maxDelayDays] (mapped to 400).
	ErrInvalidDelay = errors.New("reminder: delay out of range")
	// ErrNoChannels is an enabled rule with no channels to deliver over (mapped to 400).
	ErrNoChannels = errors.New("reminder: enabled rule needs at least one channel")
	// ErrJobNotFound is a slug that matches no job (mapped to 404).
	ErrJobNotFound = errors.New("reminder: job not found")
	// ErrNoReminder is a reschedule/cancel of a job with no pending reminder (mapped to 404).
	ErrNoReminder = errors.New("reminder: no pending reminder")
)

// validChannels is the channel allowlist, derived from the notify delivery-channel
// vocabulary so reminders and subscriptions can never drift on what a channel is.
var validChannels = func() map[string]bool {
	m := make(map[string]bool, len(notify.Channels))
	for _, c := range notify.Channels {
		m[c] = true
	}
	return m
}()

// Settings is the account-level default rule. An absent stored row reads as this
// zero-ish default (disabled, DefaultDelayDays, no channels).
type Settings struct {
	Enabled          bool
	DefaultDelayDays int
	Channels         []string
}

// Override is the per-save reminder choice. A nil *Override means "follow the
// account default". A non-nil Override either opts this job out (Disabled) or sets
// a custom delay (DelayDays > 0).
type Override struct {
	Disabled  bool
	DelayDays int
}

// Repository is the persistence contract. The adapter maps the generated db rows;
// GetSettings returns the unconfigured default (not an error) when no row exists,
// and RescheduleReminder returns ErrNoReminder when the pair has no pending row.
type Repository interface {
	JobIDBySlug(ctx context.Context, slug string) (int64, error)
	GetSettings(ctx context.Context, userID int64) (Settings, error)
	UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error)
	UpsertReminder(ctx context.Context, userID, jobID int64, fireAt time.Time, channels []string) error
	CancelReminder(ctx context.Context, userID, jobID int64) error
	RescheduleReminder(ctx context.Context, userID, jobID int64, fireAt time.Time) error
}

// Service implements the reminder use cases.
type Service struct {
	repo Repository
	now  func() time.Time
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// GetSettings returns the caller's default rule (the unconfigured default when unset).
func (s *Service) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	return s.repo.GetSettings(ctx, userID)
}

// UpdateSettings validates and stores the caller's default rule. An enabled rule
// must have at least one valid channel and an in-range delay; a disabled rule is
// still validated so stored values stay sane if it is later enabled.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in Settings) (Settings, error) {
	if in.DefaultDelayDays < minDelayDays || in.DefaultDelayDays > maxDelayDays {
		return Settings{}, ErrInvalidDelay
	}
	for _, c := range in.Channels {
		if !validChannels[c] {
			return Settings{}, ErrInvalidChannel
		}
	}
	if in.Enabled && len(in.Channels) == 0 {
		return Settings{}, ErrNoChannels
	}
	return s.repo.UpsertSettings(ctx, userID, in)
}

// ScheduleOnSave decides the reminder for a just-saved job. An explicit opt-out
// cancels any pending reminder. An explicit delay override schedules a reminder for
// this one job regardless of the account rule — a per-save "remind me" is an
// opt-in that stands on its own — falling back to the email channel when the rule
// has none configured. Without an override, a reminder is scheduled only when the
// account rule is enabled (the automatic path), at the default delay and channels.
// So a disabled rule stays off-by-default for ordinary saves, while an explicit
// choice always takes effect.
func (s *Service) ScheduleOnSave(ctx context.Context, userID, jobID int64, ov *Override) error {
	if ov != nil && ov.Disabled {
		return s.repo.CancelReminder(ctx, userID, jobID)
	}
	settings, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	explicit := ov != nil && ov.DelayDays > 0
	if !settings.Enabled && !explicit {
		return nil
	}
	delay := settings.DefaultDelayDays
	if explicit {
		if ov.DelayDays < minDelayDays || ov.DelayDays > maxDelayDays {
			return ErrInvalidDelay
		}
		delay = ov.DelayDays
	}
	// An enabled rule always has channels (UpdateSettings enforces it); the fallback
	// only covers an explicit reminder set while the rule is off/unconfigured, so a
	// per-save "remind me" always has a usable destination (the account email).
	channels := settings.Channels
	if len(channels) == 0 {
		channels = []string{notify.ChannelEmail}
	}
	return s.repo.UpsertReminder(ctx, userID, jobID, s.fireAt(delay), channels)
}

// Cancel drops the pending reminder for a (user, job), idempotently — the eager
// cleanup the handler runs on apply and unsave. No pending row is not an error.
func (s *Service) Cancel(ctx context.Context, userID, jobID int64) error {
	return s.repo.CancelReminder(ctx, userID, jobID)
}

// RescheduleBySlug moves a saved job's pending reminder to a new delay. A job with
// no pending reminder yields ErrNoReminder (the handler maps it to 404).
func (s *Service) RescheduleBySlug(ctx context.Context, userID int64, slug string, delayDays int) error {
	if delayDays < minDelayDays || delayDays > maxDelayDays {
		return ErrInvalidDelay
	}
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return err
	}
	return s.repo.RescheduleReminder(ctx, userID, jobID, s.fireAt(delayDays))
}

// CancelBySlug cancels the pending reminder for a saved job without unsaving it —
// the per-job "turn off" control. Idempotent.
func (s *Service) CancelBySlug(ctx context.Context, userID int64, slug string) error {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return err
	}
	return s.repo.CancelReminder(ctx, userID, jobID)
}

// fireAt is the deadline `delayDays` out from now.
func (s *Service) fireAt(delayDays int) time.Time {
	return s.now().Add(time.Duration(delayDays) * 24 * time.Hour)
}
