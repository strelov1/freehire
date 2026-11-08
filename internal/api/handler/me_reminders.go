package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/reminder"
)

// notificationSettingsResponse is the public shape of the account's shared
// notification rule (saved-job reminders + both lifecycle nudges), plus the
// delivery-timing controls (digest frequency/time/interval, quiet hours). It carries no
// user_id (the caller is the user) and always emits channels as an array so the
// SPA can bind checkboxes without a null guard. Times are "HH:MM" strings (nil
// when unset), matching an HTML time input and readable without a client-side
// duration library.
type notificationSettingsResponse struct {
	Enabled             bool     `json:"enabled"`
	Channels            []string `json:"channels"`
	DigestFrequency     string   `json:"digest_frequency"`
	DigestTime          *string  `json:"digest_time"`
	DigestIntervalHours int      `json:"digest_interval_hours"`
	QuietHoursStart     *string  `json:"quiet_hours_start"`
	QuietHoursEnd       *string  `json:"quiet_hours_end"`
}

func toNotificationSettingsResponse(s reminder.Settings) notificationSettingsResponse {
	ch := s.Channels
	if ch == nil {
		ch = []string{}
	}
	freq := s.DigestFrequency
	if freq == "" {
		freq = notify.FrequencyDaily
	}
	return notificationSettingsResponse{
		Enabled:             s.Enabled,
		Channels:            ch,
		DigestFrequency:     freq,
		DigestTime:          formatHHMM(s.DigestTime),
		DigestIntervalHours: s.DigestIntervalHours,
		QuietHoursStart:     formatHHMM(s.QuietHoursStart),
		QuietHoursEnd:       formatHHMM(s.QuietHoursEnd),
	}
}

// notificationSettingsRequest is the PUT body for the account notification rule.
// An omitted digest_interval_hours reads as the default, so a client that
// predates the field keeps working.
type notificationSettingsRequest struct {
	Enabled             bool     `json:"enabled"`
	Channels            []string `json:"channels"`
	DigestFrequency     string   `json:"digest_frequency"`
	DigestTime          *string  `json:"digest_time"`
	DigestIntervalHours int      `json:"digest_interval_hours"`
	QuietHoursStart     *string  `json:"quiet_hours_start"`
	QuietHoursEnd       *string  `json:"quiet_hours_end"`
}

// formatHHMM renders a time-of-day duration as "HH:MM", or nil when unset.
func formatHHMM(d *time.Duration) *string {
	if d == nil {
		return nil
	}
	s := time.Time{}.Add(*d).Format("15:04")
	return &s
}

// parseHHMM parses an "HH:MM" string into a time-of-day duration. A nil input
// stays nil; a malformed one is reported so the handler can 400.
func parseHHMM(s *string) (*time.Duration, error) {
	if s == nil {
		return nil, nil
	}
	t, err := time.Parse("15:04", *s)
	if err != nil {
		return nil, err
	}
	d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute
	return &d, nil
}

// reminderError maps the reminder sentinels onto HTTP statuses: a bad channel, an
// enabled rule with no channels, an invalid frequency, a daily rule missing its
// digest time, an interval out of bounds, or a one-sided quiet-hours window are all 400s; push added with no
// device to reach is a 409. Anything else falls through to a 500.
func reminderError(err error) error {
	switch {
	case errors.Is(err, reminder.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
	case errors.Is(err, reminder.ErrNoPushDevice):
		return fiber.NewError(fiber.StatusConflict,
			"push notifications need the freehire mobile app — sign in there and allow notifications first")
	case errors.Is(err, reminder.ErrNoChannels):
		return fiber.NewError(fiber.StatusBadRequest, "enable at least one channel to turn on notifications")
	case errors.Is(err, reminder.ErrInvalidFrequency):
		return fiber.NewError(fiber.StatusBadRequest, "digest frequency must be instant, daily or interval")
	case errors.Is(err, reminder.ErrInvalidInterval):
		return fiber.NewError(fiber.StatusBadRequest, "digest_interval_hours must be between 1 and 23")
	case errors.Is(err, reminder.ErrMissingDigestTime):
		return fiber.NewError(fiber.StatusBadRequest, "daily frequency requires a digest time")
	case errors.Is(err, reminder.ErrIncompleteQuietHours):
		return fiber.NewError(fiber.StatusBadRequest, "quiet hours needs both a start and an end")
	default:
		return err
	}
}

// GetNotificationSettings returns the caller's notification rule (the
// opt-out-by-default default when never set). Cookie-only (RequireAuth), owner-scoped.
func (h *trackingHandlers) GetNotificationSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	s, err := h.reminder.GetSettings(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toNotificationSettingsResponse(s)})
}

// UpdateNotificationSettings validates and stores the caller's rule. An enabled
// rule needs at least one valid channel. Cookie-only.
func (h *trackingHandlers) UpdateNotificationSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in notificationSettingsRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	digestTime, err := parseHHMM(in.DigestTime)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "digest_time must be HH:MM")
	}
	quietStart, err := parseHHMM(in.QuietHoursStart)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "quiet_hours_start must be HH:MM")
	}
	quietEnd, err := parseHHMM(in.QuietHoursEnd)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "quiet_hours_end must be HH:MM")
	}
	s, err := h.reminder.UpdateSettings(c.Context(), userID, reminder.Settings{
		Enabled:             in.Enabled,
		Channels:            in.Channels,
		DigestFrequency:     in.DigestFrequency,
		DigestTime:          digestTime,
		DigestIntervalHours: in.DigestIntervalHours,
		QuietHoursStart:     quietStart,
		QuietHoursEnd:       quietEnd,
	})
	if err != nil {
		return reminderError(err)
	}
	return c.JSON(fiber.Map{"data": toNotificationSettingsResponse(s)})
}
