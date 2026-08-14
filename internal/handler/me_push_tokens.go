package handler

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
	"github.com/strelov1/freehire/internal/ratelimit"
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

// pushTokenResponse is the public shape of one registered device. The token
// itself is included because it is the only field that answers the question the
// client actually asks — "is THIS device registered?" — and it is returned only
// to the account that registered it, over a cookie-authenticated request. It is
// a send capability for our own app's notifications, not a credential to the
// account, so its owner seeing it costs nothing.
type pushTokenResponse struct {
	Token      string             `json:"token"`
	Platform   string             `json:"platform"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	LastSeenAt pgtype.Timestamptz `json:"last_seen_at"`
}

// ListPushTokens returns the caller's own registered devices. It exists so the
// mobile app can derive its notification toggle from server state instead of
// keeping a private copy: the OS permission alone cannot express "the user
// turned this off" (an app cannot revoke its own permission), so without this
// endpoint the app would have to persist a local opt-in flag that silently
// disagrees with the backend the moment a token is reassigned or pruned.
// Owner-scoped and cookie-only, like the rest of /me/push-tokens.
func (h *authHandlers) ListPushTokens(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := h.queries.ListPushTokensForUser(c.Context(), userID)
	if err != nil {
		return err
	}

	out := make([]pushTokenResponse, len(rows))
	for i, r := range rows {
		out[i] = pushTokenResponse{
			Token:      r.Token,
			Platform:   r.Platform,
			CreatedAt:  r.CreatedAt,
			LastSeenAt: r.LastSeenAt,
		}
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
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

// testPushTokensPerHour bounds how many self-test sends one caller may trigger per
// hour. Unlike the rest of this file, TestPushToken fans out one outbound call to
// Expo's push API per registered device on every hit, so — like every other handler
// in this package that triggers a metered third-party call (photo uploads, JD
// resolve, mail recall, speech transcription) — it needs a ceiling: looped by hand or
// by replaying a valid session cookie, an unbounded route here would spam the
// caller's own devices and lean on the shared Expo quota for every other user's push
// delivery too. Twenty is far past a person checking a device works.
const testPushTokensPerHour = 20

// testPushTokenLimiter throttles the self-test send per authenticated caller, keyed
// like the sibling limiters elsewhere in this package: an IP key would be lifted by
// any rotating proxy pool. Mounted AFTER the cookie middleware so the user id is
// resolved.
func testPushTokenLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByUserOrIP("pushtokentest"), testPushTokensPerHour, time.Hour)
}

// TestPushToken sends a test push to every one of the caller's own
// registered tokens — never another user's, and never a caller-supplied
// destination, so this cannot become a spam/harassment vector disguised as
// a diagnostic endpoint. Cookie-only, and rate-limited (testPushTokenLimiter):
// each call fans out one outbound send per device.
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
		switch err := h.pushNotifier.Send(c.Context(), t.Token, "freehire", "This is a test notification.", nil); {
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
