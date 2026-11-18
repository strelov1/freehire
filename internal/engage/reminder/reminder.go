// Package reminder is the saved-job reminder use case: each save schedules a
// one-shot nudge at a fixed delay, gated by the shared notification_settings rule
// (also read by internal/engage/nudge). It owns the schedule/cancel decisions; the
// Runner in this package delivers due reminders from the job_reminders ledger.
package reminder

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/notify"
)

// DefaultDelayDays is the fixed delay every saved job schedules at. It is not
// configurable.
const DefaultDelayDays = 3

// DefaultNotificationHour is the local time of day a reminder fires for an account
// that has never chosen one. Reminders are rounded forward to this hour so a day's
// saves become due in one worker pass and go out as one message.
const DefaultNotificationHour = 9 * time.Hour

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrPlanUnavailable means paid access could not be checked (mapped to 503).
	ErrPlanUnavailable = errors.New("reminder: cannot verify inbox digest plan")
	// ErrFasterAlertsNeedPaidPlan is a Free account newly asking for saved-search alerts
	// faster than daily (mapped to 402). It wraps plan.ErrPaymentRequired.
	ErrFasterAlertsNeedPaidPlan = fmt.Errorf("%w: saved-search alerts faster than daily", plan.ErrPaymentRequired)
	// ErrInvalidChannel is an unsupported delivery channel (mapped to 400).
	ErrInvalidChannel = errors.New("reminder: unsupported channel")
	// ErrNoChannels is an enabled rule with no channels to deliver over (mapped to 400).
	ErrNoChannels = errors.New("reminder: enabled rule needs at least one channel")
	// ErrNoPushDevice is push newly added to the channels of an account with no
	// registered device (mapped to 409). Push reaches nothing else, so the channel
	// would only ever soft-skip. Only ADDING it is refused: a rule that already
	// carries push keeps it, so a pruned device never locks the rest of the
	// settings behind un-ticking it first.
	ErrNoPushDevice = errors.New("reminder: no registered push device")
	// ErrInvalidFrequency is a DigestFrequency outside {instant, daily, interval} (mapped to 400).
	ErrInvalidFrequency = errors.New("reminder: digest frequency must be instant, daily or interval")
	// ErrInvalidInterval is interval frequency with DigestIntervalHours outside
	// [notify.MinIntervalHours, notify.MaxIntervalHours] (mapped to 400).
	ErrInvalidInterval = errors.New("reminder: digest interval must be between 1 and 23 hours")
	// ErrMissingDigestTime is daily frequency without a DigestTime (mapped to 400).
	ErrMissingDigestTime = errors.New("reminder: daily frequency requires a digest time")
	// ErrIncompleteQuietHours is exactly one of QuietHoursStart/QuietHoursEnd set
	// (mapped to 400) — the window is meaningless with only one edge.
	ErrIncompleteQuietHours = errors.New("reminder: quiet hours needs both a start and an end")
)

// Settings is the account-level notification rule, shared with the nudge engine
// (see notification_settings). An absent stored row reads as the default: enabled,
// channel email, a daily digest. DigestFrequency/DigestTime/DigestIntervalHours
// govern only notify's saved-search digests; QuietHoursStart/End defer delivery
// across all three engines (internal/application/deliverywindow).
type Settings struct {
	Enabled                  bool
	InboxDigestEnabled       bool
	ApplicationDigestEnabled bool
	Channels                 []string
	// DigestFrequency is one of notify.Frequency*; empty reads as notify.FrequencyInterval.
	DigestFrequency string
	DigestTime      *time.Duration
	// DigestIntervalHours is read only in interval mode; zero reads as
	// notify.DefaultIntervalHours.
	DigestIntervalHours int
	QuietHoursStart     *time.Duration
	QuietHoursEnd       *time.Duration
}

// ScheduleContext is the account's daily notification hour and the timezone it is
// read in. Nil means unconfigured, not zero: midnight is a choosable hour.
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

// location is the account's timezone, or UTC when unset, matching
// internal/application/deliverywindow.
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
	// HasPushDevice reports whether the account has a registered push device.
	HasPushDevice(ctx context.Context, userID int64) (bool, error)
	HasPaidPlan(ctx context.Context, userID int64) (bool, error)
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

// UpdateSettings validates and stores the caller's rule. An enabled rule needs at
// least one valid channel. DigestFrequency (empty reads as daily at the default
// time) must be one of notify.Frequency*; daily requires a DigestTime, interval an
// interval within notify's bounds (zero reads as the default). QuietHoursStart/End
// are set together or not at all.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in Settings) (Settings, error) {
	for _, c := range in.Channels {
		if !notify.ValidChannel(c) {
			return Settings{}, ErrInvalidChannel
		}
	}
	if in.Enabled && len(in.Channels) == 0 {
		return Settings{}, ErrNoChannels
	}
	switch in.DigestFrequency {
	case "":
		in.DigestFrequency = notify.FrequencyDaily
		if in.DigestTime == nil {
			t := notify.DefaultDigestTime
			in.DigestTime = &t
		}
	case notify.FrequencyInstant, notify.FrequencyDaily, notify.FrequencyInterval:
	default:
		return Settings{}, ErrInvalidFrequency
	}
	if (in.DigestFrequency == notify.FrequencyDaily || in.InboxDigestEnabled || in.ApplicationDigestEnabled) && in.DigestTime == nil {
		return Settings{}, ErrMissingDigestTime
	}
	hours, err := s.resolveIntervalHours(ctx, userID, in)
	if err != nil {
		return Settings{}, err
	}
	in.DigestIntervalHours = hours
	if in.DigestFrequency != notify.FrequencyDaily {
		if err := s.checkFasterAlerts(ctx, userID, in); err != nil {
			return Settings{}, err
		}
	}
	if (in.QuietHoursStart == nil) != (in.QuietHoursEnd == nil) {
		return Settings{}, ErrIncompleteQuietHours
	}
	if err := s.checkPushDevice(ctx, userID, in.Channels); err != nil {
		return Settings{}, err
	}
	if in.InboxDigestEnabled || in.ApplicationDigestEnabled {
		current, err := s.repo.GetSettings(ctx, userID)
		if err != nil {
			return Settings{}, err
		}
		// Keep an existing preference editable after expiry; delivery checks the live plan.
		if (in.InboxDigestEnabled && !current.InboxDigestEnabled) || (in.ApplicationDigestEnabled && !current.ApplicationDigestEnabled) {
			paid, err := s.HasFasterAlerts(ctx, userID)
			if err != nil {
				return Settings{}, err
			}
			if !paid {
				return Settings{}, plan.ErrPaymentRequired
			}
		}
	}
	return s.repo.UpsertSettings(ctx, userID, in)
}

// checkFasterAlerts refuses a faster-than-daily saved-search frequency to an account
// without a live Pro or Ultra plan. Only a CHANGE is a request: the settings page sends
// the whole rule back, so a Free account that kept a historical instant or interval
// preference may save unrelated edits, and the preference stays stored for a renewal.
// Delivery reads the live plan too (ClaimSubscriptionMatches and friends), so the stored
// value never buys speed. A plan that cannot be read fails closed.
func (s *Service) checkFasterAlerts(ctx context.Context, userID int64, in Settings) error {
	current, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	if current.DigestFrequency == in.DigestFrequency &&
		(in.DigestFrequency != notify.FrequencyInterval || storedIntervalHours(current) == in.DigestIntervalHours) {
		return nil
	}
	paid, err := s.HasFasterAlerts(ctx, userID)
	if err != nil {
		return err
	}
	if !paid {
		return ErrFasterAlertsNeedPaidPlan
	}
	return nil
}

// HasFasterAlerts reports whether the account may choose a saved-search frequency
// faster than daily: a live Pro or Ultra entitlement from any source.
func (s *Service) HasFasterAlerts(ctx context.Context, userID int64) (bool, error) {
	paid, err := s.repo.HasPaidPlan(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrPlanUnavailable, err)
	}
	return paid, nil
}

// resolveIntervalHours decides the interval to store. The sent value is read only
// in interval mode (stored, or refused when out of range). Otherwise, including an
// omitted field on a full-replacement write, the stored interval is kept so an
// unrelated save cannot reset it.
func (s *Service) resolveIntervalHours(ctx context.Context, userID int64, in Settings) (int, error) {
	h := in.DigestIntervalHours
	if in.DigestFrequency == notify.FrequencyInterval && h != 0 {
		if h < notify.MinIntervalHours || h > notify.MaxIntervalHours {
			return 0, ErrInvalidInterval
		}
		return h, nil
	}
	current, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return 0, err
	}
	return storedIntervalHours(current), nil
}

// storedIntervalHours is the account's interval as the service reads it: one outside the
// bounds (never stored, since the column CHECKs it) reads as the default.
func storedIntervalHours(s Settings) int {
	if s.DigestIntervalHours < notify.MinIntervalHours || s.DigestIntervalHours > notify.MaxIntervalHours {
		return notify.DefaultIntervalHours
	}
	return s.DigestIntervalHours
}

// checkPushDevice refuses push when it is being added and the account has no
// device for it to reach (see ErrNoPushDevice).
func (s *Service) checkPushDevice(ctx context.Context, userID int64, channels []string) error {
	if !slices.Contains(channels, notify.ChannelPush) {
		return nil
	}
	current, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	if slices.Contains(current.Channels, notify.ChannelPush) {
		return nil
	}
	ok, err := s.repo.HasPushDevice(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoPushDevice
	}
	return nil
}

// ScheduleOnSave decides the reminder for a just-saved job: scheduled at the fixed
// DefaultDelayDays when the account's shared notification rule is enabled, skipped
// otherwise. There is no per-job override.
func (s *Service) ScheduleOnSave(ctx context.Context, userID, jobID int64) error {
	settings, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	// Defensive backstop: an enabled rule always has channels.
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
// account's notification hour so a day's saves land on the same instant and are
// grouped into one message.
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
// Built via time.Date, not local midnight + `hour`, which is off by an hour on DST
// change days.
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
