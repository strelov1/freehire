// Package reminder is the saved-job reminder use case: each save schedules a
// one-shot nudge, at a fixed delay, to come back before the vacancy goes stale —
// gated by the shared notification_settings rule (also read by internal/engage/nudge).
// It owns the schedule/cancel decisions; the firing engine (Runner, in this
// package) reads the same job_reminders ledger and delivers due reminders. This
// mirrors internal/engage/subscription (the HTTP-facing use case) + internal/engage/notify (the
// delivery worker), kept in one package because a reminder is a single small
// concept.
package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/engage/notify"
)

// DefaultDelayDays is the fixed delay every saved job schedules at. It is no
// longer configurable — see the centralize-lifecycle-notifications change: the
// per-job/per-account delay choice was removed as unused surface (a production
// usage check found no account other than the developer's had ever touched it).
const DefaultDelayDays = 3

// DefaultNotificationHour is the local time of day a reminder fires for an account
// that has never chosen one. Reminders are rounded forward to this hour so that
// everything saved on one day becomes due in a single worker pass and goes out as
// one message — see the aggregate-reminder-and-nudge-digests change.
const DefaultNotificationHour = 9 * time.Hour

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

// Settings is the account-level notification rule, shared with the internal/engage/nudge
// engine (see notification_settings). An absent stored row reads as the
// opt-out-by-default default: enabled, channel email. DigestFrequency/DigestTime
// govern only internal/engage/notify's saved-search digests; QuietHoursStart/End defer
// delivery across all three notification engines — see internal/application/deliverywindow.
type Settings struct {
	Enabled         bool
	Channels        []string
	DigestFrequency string // "instant" (default/zero value reads as instant) or "daily"
	DigestTime      *time.Duration
	QuietHoursStart *time.Duration
	QuietHoursEnd   *time.Duration
}

// ScheduleContext is when an account wants to hear from us: the daily notification
// hour and the timezone that hour is read in. Both are optional, and a nil is the
// unconfigured state rather than a zero — midnight is a choosable hour, so it
// cannot double as "never chose one".
type ScheduleContext struct {
	NotificationHour *time.Duration
	Location         *time.Location
}

// hour is the configured notification hour, or the default when unset.
func (c ScheduleContext) hour() time.Duration {
	if c.NotificationHour == nil {
		return DefaultNotificationHour
	}
	return *c.NotificationHour
}

// location is the account's timezone, or UTC when unset — the same fallback
// internal/application/deliverywindow applies to quiet hours, so the two timing
// rules agree about who has no timezone.
func (c ScheduleContext) location() *time.Location {
	if c.Location == nil {
		return time.UTC
	}
	return c.Location
}

// Repository is the persistence contract. The adapter maps the generated db rows;
// GetSettings returns the default (not an error) when no row exists.
type Repository interface {
	GetSettings(ctx context.Context, userID int64) (Settings, error)
	UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error)
	GetScheduleContext(ctx context.Context, userID int64) (ScheduleContext, error)
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
	fireAt, err := s.fireAt(ctx, userID, DefaultDelayDays)
	if err != nil {
		return err
	}
	return s.repo.UpsertReminder(ctx, userID, jobID, fireAt, channels)
}

// Cancel drops the pending reminder for a (user, job), idempotently — the eager
// cleanup the handler runs on apply and unsave. No pending row is not an error.
func (s *Service) Cancel(ctx context.Context, userID, jobID int64) error {
	return s.repo.CancelReminder(ctx, userID, jobID)
}

// fireAt is the deadline `delayDays` out from now, rounded forward to the
// account's notification hour. The rounding is what makes a day's saves arrive as
// one message: two reminders scheduled hours apart land on the same instant, so
// one worker pass claims both and the engine groups them.
func (s *Service) fireAt(ctx context.Context, userID int64, delayDays int) (time.Time, error) {
	deadline := s.now().Add(time.Duration(delayDays) * 24 * time.Hour)
	sc, err := s.repo.GetScheduleContext(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	return nextNotificationHour(deadline, sc.hour(), sc.location()), nil
}

// nextNotificationHour is the first wall-clock occurrence of `hour` in `loc` at or
// after `after` — never earlier, so rounding cannot pull a reminder in ahead of its
// delay.
//
// Built by naming the hour to time.Date rather than by adding `hour` to local
// midnight: on the two days a year the offset moves, midnight+18h is 17:00 or
// 19:00, and the account asked for 18:00 on its own clock.
func nextNotificationHour(after time.Time, hour time.Duration, loc *time.Location) time.Time {
	y, mo, d := after.In(loc).Date()
	h := int(hour / time.Hour)
	m := int(hour % time.Hour / time.Minute)
	sec := int(hour % time.Minute / time.Second)

	candidate := time.Date(y, mo, d, h, m, sec, 0, loc)
	if candidate.Before(after) {
		candidate = time.Date(y, mo, d+1, h, m, sec, 0, loc)
	}
	return candidate
}
