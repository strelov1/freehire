package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
)

// registerPushTokenRequest is the POST /me/push-tokens body: the mobile app's
// Expo push token, plus the platform it was minted on (stored only for our
// own visibility — it plays no role in how a message is sent, since Expo's
// relay handles both platforms from the same token format).
type registerPushTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

// RegisterPushToken registers (or refreshes) the caller's device push token.
// Upserted by token value, not user-scoped: a token identifies one device
// install, not one user, so if a different account signs in on the same
// device, this reassigns the row to the caller instead of duplicating it or
// leaving it with the previous owner. Cookie-only.
func (h *authHandlers) RegisterPushToken(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in registerPushTokenRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" {
		return fiber.NewError(fiber.StatusBadRequest, "token is required")
	}
	if in.Platform != "ios" && in.Platform != "android" {
		return fiber.NewError(fiber.StatusBadRequest, "platform must be ios or android")
	}

	if _, err := h.queries.UpsertPushToken(c.Context(), db.UpsertPushTokenParams{
		UserID: userID, Token: in.Token, Platform: in.Platform,
	}); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// unregisterPushTokenRequest is the DELETE /me/push-tokens body.
type unregisterPushTokenRequest struct {
	Token string `json:"token"`
}

// UnregisterPushToken removes one of the caller's own registered tokens
// (e.g. sign-out). Owner-scoped like RevokeAPIKey: a token that doesn't
// belong to the caller is simply not found, revealing nothing about who
// does own it. Cookie-only.
func (h *authHandlers) UnregisterPushToken(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in unregisterPushTokenRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" {
		return fiber.NewError(fiber.StatusBadRequest, "token is required")
	}

	affected, err := h.queries.DeletePushToken(c.Context(), db.DeletePushTokenParams{
		UserID: userID, Token: in.Token,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "token not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// testPushTokenResponse reports what a self-test send actually did per
// registered device, so the caller can tell "arrived" apart from "that
// token turned out to be dead and was just removed" — both look identical
// to naive error-or-not handling of pushnotify.Notifier.Send.
type testPushTokenResponse struct {
	Devices int `json:"devices"`
	Sent    int `json:"sent"`
	Pruned  int `json:"pruned"`
	Failed  int `json:"failed"`
}

// TestPushToken sends a test push to every one of the caller's own
// registered tokens — never another user's, and never a caller-supplied
// destination, so this cannot become a spam/harassment vector disguised as
// a diagnostic endpoint. Cookie-only.
func (h *authHandlers) TestPushToken(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	tokens, err := h.queries.ListPushTokensForUser(c.Context(), userID)
	if err != nil {
		return err
	}

	out := testPushTokenResponse{Devices: len(tokens)}
	for _, t := range tokens {
		switch err := h.pushNotifier.Send(c.Context(), t.Token, "freehire", "This is a test notification."); {
		case err == nil:
			out.Sent++
		case errors.Is(err, pushnotify.ErrTokenPruned):
			out.Pruned++
		default:
			out.Failed++
		}
	}
	return c.JSON(fiber.Map{"data": out})
}
