package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
)

// endSession revokes the recent-auth grant and clears both cookies.
func (h *authHandlers) endSession(c *fiber.Ctx) error {
	var revokeErr error
	if h.recentAuth != nil {
		revokeErr = h.recentAuth.Revoke(c.Context(), c.Cookies(recentauth.CookieName))
	}
	domain := auth.CookieDomainForHost(c.Hostname(), h.cookieDomains)
	auth.ClearTokenCookie(c, h.cookieSecure, domain)
	recentauth.ClearCookie(c, h.cookieSecure, domain)
	if revokeErr != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
	}
	return nil
}

// LogoutAll revokes every session for the caller's account — including this one — by
// advancing the account's token generation, then clears the caller's cookie. Cookie-only:
// a leaked API key must not be able to sign the owner out of their browser, and an agent
// has no business ending human sessions.
func (h *authHandlers) LogoutAll(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.queries.BumpUserTokenVersion(c.Context(), userID); err != nil {
		return err
	}
	domain := auth.CookieDomainForHost(c.Hostname(), h.cookieDomains)
	auth.ClearTokenCookie(c, h.cookieSecure, domain)
	recentauth.ClearCookie(c, h.cookieSecure, domain)
	return c.SendStatus(fiber.StatusNoContent)
}

// setSession issues a token for userID and writes it as the auth cookie. The token is
// stamped with the account's current session generation, so it is born valid and any
// later revocation strands it.
func (h *authHandlers) setSession(c *fiber.Ctx, userID int64) error {
	version, err := h.sessionVersion(c.Context(), userID)
	if err != nil {
		return err
	}
	return h.setSessionAt(c, userID, version)
}

// sessionVersion is the account's current session generation, the number a new session is
// stamped with. GetUserTokenVersion returns no row for a banned account, so this is the
// choke point that stops every path that ends in a session (password, OAuth, Apple, the
// mobile exchange, the extension connect) from signing one in; paths that hand out a
// code first call it to refuse early.
func (h *authHandlers) sessionVersion(ctx context.Context, userID int64) (int32, error) {
	version, err := h.queries.GetUserTokenVersion(ctx, userID)
	switch {
	case err == nil:
		return version, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, errAccountSuspended
	default:
		return 0, fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
	}
}

// setSessionAt writes a session cookie for a known generation. Callers that just bumped
// the counter (password change, sign-out-everywhere) pass the value they got back, so the
// caller's replacement cookie cannot race the bump they themselves performed.
func (h *authHandlers) setSessionAt(c *fiber.Ctx, userID int64, version int32) error {
	_, err := h.issueSessionAt(c, userID, version)
	return err
}

func (h *authHandlers) issueSessionAt(c *fiber.Ctx, userID int64, version int32) (string, error) {
	token, err := h.issuer.Issue(userID, version)
	if err != nil {
		// Returned rather than phrased as fiber.NewError(500, …): a signing failure is
		// a fault the error inbox must see with its cause, and a *fiber.Error carries
		// none. The caller sees the generic 500 body instead — see classify.
		return "", fmt.Errorf("issue session token for user %d: %w", userID, err)
	}
	auth.SetTokenCookie(c, token, h.issuer.TTL(), h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return token, nil
}
