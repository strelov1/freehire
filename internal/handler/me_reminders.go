package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/reminder"
)

// reminderSettingsResponse is the public shape of the account default rule. It
// carries no user_id (the caller is the user) and always emits channels as an
// array so the SPA can bind checkboxes without a null guard.
type reminderSettingsResponse struct {
	Enabled          bool     `json:"enabled"`
	DefaultDelayDays int      `json:"default_delay_days"`
	Channels         []string `json:"channels"`
}

func toReminderSettingsResponse(s reminder.Settings) reminderSettingsResponse {
	ch := s.Channels
	if ch == nil {
		ch = []string{}
	}
	return reminderSettingsResponse{
		Enabled:          s.Enabled,
		DefaultDelayDays: s.DefaultDelayDays,
		Channels:         ch,
	}
}

// reminderSettingsRequest is the PUT body for the account default rule.
type reminderSettingsRequest struct {
	Enabled          bool     `json:"enabled"`
	DefaultDelayDays int      `json:"default_delay_days"`
	Channels         []string `json:"channels"`
}

// rescheduleReminderRequest is the PATCH body: the new delay for a saved job's
// pending reminder.
type rescheduleReminderRequest struct {
	DelayDays int `json:"delay_days"`
}

// reminderError maps the reminder sentinels onto HTTP statuses: an unknown job or
// absent pending reminder is a 404; a bad channel, out-of-range delay, or an
// enabled rule with no channels is a 400. Anything else falls through to a 500.
func reminderError(err error) error {
	switch {
	case errors.Is(err, reminder.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, reminder.ErrNoReminder):
		return fiber.NewError(fiber.StatusNotFound, "no pending reminder for this job")
	case errors.Is(err, reminder.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
	case errors.Is(err, reminder.ErrInvalidDelay):
		return fiber.NewError(fiber.StatusBadRequest, "reminder delay must be between 1 and 365 days")
	case errors.Is(err, reminder.ErrNoChannels):
		return fiber.NewError(fiber.StatusBadRequest, "enable at least one channel to turn on reminders")
	default:
		return err
	}
}

// GetReminderSettings returns the caller's reminder default rule (the unconfigured
// default when never set). Cookie-only (RequireAuth), owner-scoped.
func (a *API) GetReminderSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	s, err := a.reminder.GetSettings(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toReminderSettingsResponse(s)})
}

// UpdateReminderSettings validates and stores the caller's default rule. An enabled
// rule needs at least one valid channel and an in-range delay. Cookie-only.
func (a *API) UpdateReminderSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in reminderSettingsRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	s, err := a.reminder.UpdateSettings(c.Context(), userID, reminder.Settings{
		Enabled:          in.Enabled,
		DefaultDelayDays: in.DefaultDelayDays,
		Channels:         in.Channels,
	})
	if err != nil {
		return reminderError(err)
	}
	return c.JSON(fiber.Map{"data": toReminderSettingsResponse(s)})
}

// RescheduleReminder moves a saved job's pending reminder to a new delay without
// unsaving. A job with no pending reminder is a 404.
func (a *API) RescheduleReminder(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in rescheduleReminderRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := a.reminder.RescheduleBySlug(c.Context(), userID, c.Params("slug"), in.DelayDays); err != nil {
		return reminderError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// CancelJobReminder turns off the pending reminder for a saved job without
// unsaving it. Idempotent — a job with no pending reminder is still a 204.
func (a *API) CancelJobReminder(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := a.reminder.CancelBySlug(c.Context(), userID, c.Params("slug")); err != nil {
		return reminderError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
