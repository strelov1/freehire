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

// RegisterPushToken registers (or refreshes) the caller's device push token.
// Upserted by token value, not user-scoped: a token identifies one device
// install, not one user, so if a different account signs in on the same
// device, this reassigns the row to the caller instead of duplicating it or
// leaving it with the previous owner. Cookie-only.
func (h *authHandlers) RegisterPushToken(ctx context.Context, req *apigen.PushTokenRegister) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	token := strings.TrimSpace(req.Token.Value)
	if token == "" {
		return fiber.NewError(fiber.StatusBadRequest, "token is required")
	}
	// The platform is stored only for our own visibility — it plays no role in how a
	// message is sent, since Expo's relay handles both platforms from the same token format.
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

// UnregisterPushToken removes one of the caller's own registered tokens
// (e.g. sign-out). Owner-scoped like RevokeAPIKey: a token that doesn't
// belong to the caller is simply not found, revealing nothing about who
// does own it. Cookie-only.
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

// The list answers the token itself because it is the only field that answers the
// question the client actually asks — "is THIS device registered?" — and it is returned
// only to the account that registered it, over a cookie-authenticated request. It is a
// send capability for our own app's notifications, not a credential to the account, so
// its owner seeing it costs nothing.

// ListPushTokens returns the caller's own registered devices. It exists so the
// mobile app can derive its notification toggle from server state instead of
// keeping a private copy: the OS permission alone cannot express "the user
// turned this off" (an app cannot revoke its own permission), so without this
// endpoint the app would have to persist a local opt-in flag that silently
// disagrees with the backend the moment a token is reassigned or pruned.
// Owner-scoped and cookie-only, like the rest of /me/push-tokens.
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

// TestPushToken sends a test push to every one of the caller's own
// registered tokens — never another user's, and never a caller-supplied
// destination, so this cannot become a spam/harassment vector disguised as
// a diagnostic endpoint. Cookie-only:
// each call fans out one outbound send per device.
func (h *authHandlers) TestPushToken(ctx context.Context) (*apigen.TestPushTokenOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	tokens, err := h.queries.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// What the send did per registered device, so the caller can tell "arrived" apart
	// from "that token turned out to be dead and was just removed" — both look identical
	// to naive error-or-not handling of pushnotify.Notifier.Send.
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
