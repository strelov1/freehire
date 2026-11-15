package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// RegisterPushToken registers (or refreshes) the caller's device push token. Upserted by
// token value: a token identifies one device install, so when another account signs in on
// the device the row is reassigned to the caller. Cookie-only.
func (h *authHandlers) RegisterPushToken(ctx context.Context, req *apigen.PushTokenRegister) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	token := strings.TrimSpace(req.Token.Value)
	if token == "" {
		return fiber.NewError(fiber.StatusBadRequest, "token is required")
	}
	// The platform is stored only for visibility; Expo's relay sends both from the same format.
	platform := req.Platform.Value
	if platform != "ios" && platform != "android" {
		return fiber.NewError(fiber.StatusBadRequest, "platform must be ios or android")
	}

	if _, err := h.queries.UpsertPushToken(ctx, db.UpsertPushTokenParams{
		UserID: userID, Token: token, Platform: platform,
	}); err != nil {
		return err
	}
	return nil
}

// UnregisterPushToken removes one of the caller's registered tokens (e.g. sign-out).
// Owner-scoped: a token that is not the caller's is not found, revealing nothing about its
// owner. Cookie-only.
func (h *authHandlers) UnregisterPushToken(ctx context.Context, req *apigen.PushTokenRemove) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	token := strings.TrimSpace(req.Token.Value)
	if token == "" {
		return fiber.NewError(fiber.StatusBadRequest, "token is required")
	}

	affected, err := h.queries.DeletePushToken(ctx, db.DeletePushTokenParams{
		UserID: userID, Token: token,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "token not found")
	}
	return nil
}

// The list answers the token itself because it is the only field that answers "is THIS
// device registered?". It goes only to the account that registered it, over a cookie; it is a
// send capability for our own app's notifications, not an account credential.

// ListPushTokens returns the caller's registered devices so the mobile app can derive its
// notification toggle from server state: the OS permission cannot express "turned off", and a
// local opt-in flag would drift when a token is reassigned or pruned. Owner-scoped and
// cookie-only.
func (h *authHandlers) ListPushTokens(ctx context.Context) (*apigen.ListPushTokensOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := h.queries.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]apigen.PushToken, len(rows))
	for i, r := range rows {
		out[i] = apigen.PushToken{
			Token:      r.Token,
			Platform:   r.Platform,
			CreatedAt:  nilDateTime(pgconv.TimePtr(r.CreatedAt)),
			LastSeenAt: nilDateTime(pgconv.TimePtr(r.LastSeenAt)),
		}
	}
	return &apigen.ListPushTokensOK{Data: out, Meta: apigen.AccountTotalMeta{Total: int64(len(out))}}, nil
}

// TestPushToken sends a test push to every one of the caller's registered tokens, never
// another user's and never a caller-supplied destination, so it cannot become a spam vector.
// Cookie-only: each call sends one push per device.
func (h *authHandlers) TestPushToken(ctx context.Context) (*apigen.TestPushTokenOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	tokens, err := h.queries.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Report what the send did per device, so "arrived" is distinguishable from "token was
	// dead and just removed".
	out := apigen.PushTokenTestResult{Devices: int64(len(tokens))}
	for _, t := range tokens {
		switch err := h.pushNotifier.Send(ctx, t.Token, "freehire", "This is a test notification.", nil); {
		case err == nil:
			out.Sent++
		case errors.Is(err, pushnotify.ErrTokenPruned):
			out.Pruned++
		default:
			out.Failed++
		}
	}
	return &apigen.TestPushTokenOK{Data: out}, nil
}
