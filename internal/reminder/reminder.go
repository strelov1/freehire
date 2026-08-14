// Package reminder is the saved-job reminder use case: each save schedules a
// one-shot nudge, at a fixed delay, to come back before the vacancy goes stale —
// gated by the shared notification_settings rule (also read by internal/nudge).
// It owns the schedule/cancel decisions; the firing engine (Runner, in this
// package) reads the same job_reminders ledger and delivers due reminders. This
// mirrors internal/subscription (the HTTP-facing use case) + internal/notify (the
// delivery worker), kept in one package because a reminder is a single small
// concept.
package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/notify"
)

// DefaultDelayDays is the fixed delay every saved job schedules at. It is no
// longer configurable — see the centralize-lifecycle-notifications change: the
// per-job/per-account delay choice was removed as unused surface (a production
// usage check found no account other than the developer's had ever touched it).
const DefaultDelayDays = 3

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidChannel is an unsupported delivery channel (mapped to 400).
	ErrInvalidChannel = errors.New("reminder: unsupported channel")
	// ErrNoChannels is an enabled rule with no channels to deliver over (mapped to 400).
	ErrNoChannels = errors.New("reminder: enabled rule needs at least one channel")
	// ErrInvalidFrequency is a DigestFrequency outside {instant, daily} (mapped to 400).
	ErrInvalidFrequency = errors.New("reminder: digest frequency must be instant or daily")
	// ErrMissingDigestTime is daily frequency without a DigestTime (mapped to 400).
	ErrMissingDigestTime = errors.New("reminder: daily frequency requires a digest time")
	// ErrIncompleteQuietHours is exactly one of QuietHoursStart/QuietHoursEnd set
	// (mapped to 400) — the window is meaningless with only one edge.
	ErrIncompleteQuietHours = errors.New("reminder: quiet hours needs both a start and an end")
)

// Settings is the account-level notification rule, shared with the internal/nudge
// engine (see notification_settings). An absent stored row reads as the
// opt-out-by-default default: enabled, channel email. DigestFrequency/DigestTime
// govern only internal/notify's saved-search digests; QuietHoursStart/End defer
// delivery across all three notification engines — see internal/deliverywindow.
type Settings struct {
	Enabled         bool
	Channels        []string
	DigestFrequency string // "instant" (default/zero value reads as instant) or "daily"
	DigestTime      *time.Duration
	QuietHoursStart *time.Duration
	QuietHoursEnd   *time.Duration
}

// Repository is the persistence contract. The adapter maps the generated db rows;
// GetSettings returns the default (not an error) when no row exists.
type Repository interface {
	GetSettings(ctx context.Context, userID int64) (Settings, error)
	UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error)
	UpsertReminder(ctx context.Context, userID, jobID int64, fireAt time.Time, channels []string) error
	CancelReminder(ctx context.Context, userID, jobID int64) error
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

// UpdateSettings validates and stores the caller's rule. An enabled rule must have
// at least one valid channel. DigestFrequency (empty reads as "instant") must be
// "instant" or "daily"; "daily" requires a DigestTime. QuietHoursStart/End must be
// set together or not at all — a one-sided window has no meaning.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in Settings) (Settings, error) {
	for _, c := range in.Channels {
		if !notify.ValidChannel(c) {
			return Settings{}, ErrInvalidChannel
		}
	}
	if in.Enabled && len(in.Channels) == 0 {
		return Settings{}, ErrNoChannels
	}
	if in.DigestFrequency != "" && in.DigestFrequency != "instant" && in.DigestFrequency != "daily" {
		return Settings{}, ErrInvalidFrequency
	}
	if in.DigestFrequency == "daily" && in.DigestTime == nil {
		return Settings{}, ErrMissingDigestTime
	}
	if (in.QuietHoursStart == nil) != (in.QuietHoursEnd == nil) {
		return Settings{}, ErrIncompleteQuietHours
	}
	return s.repo.UpsertSettings(ctx, userID, in)
}

// ScheduleOnSave decides the reminder for a just-saved job: scheduled at the fixed
// DefaultDelayDays when the account's shared notification rule is enabled, skipped
// entirely otherwise. There is no per-job override any more — the shared rule is
// the only control.
func (s *Service) ScheduleOnSave(ctx context.Context, userID, jobID int64) error {
	settings, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	// An enabled rule always has channels (UpdateSettings enforces it, and the
	// never-configured default already includes email); the fallback is a
	// defensive backstop, not a reachable path.
	channels := settings.Channels
	if len(channels) == 0 {
		channels = []string{notify.ChannelEmail}
	}
	return s.repo.UpsertReminder(ctx, userID, jobID, s.fireAt(DefaultDelayDays), channels)
}

// Cancel drops the pending reminder for a (user, job), idempotently — the eager
// cleanup the handler runs on apply and unsave. No pending row is not an error.
func (s *Service) Cancel(ctx context.Context, userID, jobID int64) error {
	return s.repo.CancelReminder(ctx, userID, jobID)
}

// fireAt is the deadline `delayDays` out from now.
func (s *Service) fireAt(delayDays int) time.Time {
	return s.now().Add(time.Duration(delayDays) * 24 * time.Hour)
}
