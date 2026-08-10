package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/reminder"
)

// notificationSettingsResponse is the public shape of the account's shared
// notification rule (saved-job reminders + both lifecycle nudges). It carries no
// user_id (the caller is the user) and always emits channels as an array so the
// SPA can bind checkboxes without a null guard.
type notificationSettingsResponse struct {
	Enabled  bool     `json:"enabled"`
	Channels []string `json:"channels"`
}

func toNotificationSettingsResponse(s reminder.Settings) notificationSettingsResponse {
	ch := s.Channels
	if ch == nil {
		ch = []string{}
	}
	return notificationSettingsResponse{Enabled: s.Enabled, Channels: ch}
}

// notificationSettingsRequest is the PUT body for the account notification rule.
type notificationSettingsRequest struct {
	Enabled  bool     `json:"enabled"`
	Channels []string `json:"channels"`
}

// reminderError maps the reminder sentinels onto HTTP statuses: a bad channel, or
// an enabled rule with no channels, is a 400. Anything else falls through to a 500.
func reminderError(err error) error {
	switch {
	case errors.Is(err, reminder.ErrInvalidChannel):
		return fiber.NewError(fiber.StatusBadRequest, "unsupported notification channel")
	case errors.Is(err, reminder.ErrNoChannels):
		return fiber.NewError(fiber.StatusBadRequest, "enable at least one channel to turn on notifications")
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
	s, err := h.reminder.UpdateSettings(c.Context(), userID, reminder.Settings{
		Enabled:  in.Enabled,
		Channels: in.Channels,
	})
	if err != nil {
		return reminderError(err)
	}
	return c.JSON(fiber.Map{"data": toNotificationSettingsResponse(s)})
}
