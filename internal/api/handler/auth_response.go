package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// userResponse is the public shape of a user. It deliberately omits
// password_hash so the hash never reaches a response. role is included so the SPA can
// decide whether to surface moderator-only UI; it is an affordance only, as RequireRole
// re-checks the DB-stored role on every privileged request.
type userResponse struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	BetaTester bool   `json:"beta_tester"`
	// EmailVerified drives the SPA's "confirm your email" prompt. It is an affordance
	// only: the server never trusts the client's copy of it.
	EmailVerified bool `json:"email_verified"`
	// HasPassword lets the SPA offer a password change to password accounts and explain
	// itself to OAuth-only ones, instead of guessing from "is signed in".
	HasPassword bool       `json:"has_password"`
	CreatedAt   *time.Time `json:"created_at"`
	// Timezone is the account's IANA name (nil until set), read by the profile
	// page to pre-fill its timezone field and by the notification-settings UI's
	// missing-timezone hint.
	Timezone *string `json:"timezone"`
	// Language is the preferred interface language (one of supportedLanguages, "en" until
	// changed). No UI translation exists yet; this only records the preference.
	Language string `json:"language"`
	// OnboardingCompletedAt is when the account finished (or declined) onboarding. The
	// root layout's whole gate: null routes to /onboarding, a timestamp never does.
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at"`
	// Tier is the caller's resolved plan (plan.TierFree/TierPro/TierUltra); the header badge reads it here to avoid a second request.
	Tier plan.Tier `json:"tier"`
	// AgentConnected is whether the account holds a live API key or MCP grant — see
	// UserHasAgentCredential. The site's "connect your agent" strip stops asking once it
	// is true; riding along here lets the server leave the strip out of the first paint.
	AgentConnected bool `json:"agent_connected"`
	// APITier is the API product the account holds (billing.APITierOf), separate from
	// Tier: the account area shows its API workspace only to a holder.
	APITier billing.APITier `json:"api_tier"`
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Timezone is the browser-detected IANA zone, captured at registration only
	// (Login ignores it — an existing account's timezone is edited on the
	// profile page, not overwritten silently on every sign-in).
	Timezone *string `json:"timezone"`
}

// toUserResponse maps an accounts.User to its public shape. Pure and DB-free: it leaves
// Tier zero, so response handlers call toUserResponseWithTier instead.
func toUserResponse(u accounts.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, BetaTester: u.BetaTester,
		EmailVerified: u.EmailVerified, HasPassword: u.HasPassword, CreatedAt: u.CreatedAt,
		Timezone: u.Timezone, Language: u.Language, OnboardingCompletedAt: u.OnboardingCompletedAt}
}

// toUserResponseWithTier is toUserResponse plus the caller's resolved tier and agent connection.
func (h *authHandlers) toUserResponseWithTier(ctx context.Context, u accounts.User) userResponse {
	resp := toUserResponse(u)
	resp.Tier, resp.APITier = h.resolveTiers(ctx, u.ID)
	resp.AgentConnected = h.resolveAgentConnected(ctx, u.ID)
	return resp
}

// resolveAgentConnected degrades to false on a read failure or unset queries, for the
// reason resolveTiers gives: the worst it costs is the strip asking once more.
func (h *authHandlers) resolveAgentConnected(ctx context.Context, userID int64) bool {
	if h.queries == nil {
		return false
	}
	connected, err := h.queries.UserHasAgentCredential(ctx, userID)
	if err != nil {
		log.Printf("auth: resolving agent connection for user %d: %v", userID, err)
		return false
	}
	return connected
}

// resolveTiers reads one row of entitlements and resolves the plan via plan.TierOf and
// the API product via billing.APITierOf. A read failure or unset queries degrades to
// free / no API product and is logged: sign-in and /me must not depend on a badge.
func (h *authHandlers) resolveTiers(ctx context.Context, userID int64) (plan.Tier, billing.APITier) {
	if h.queries == nil {
		return plan.TierFree, billing.APITierNone
	}
	row, err := h.queries.GetUserAPIEntitlement(ctx, userID)
	if err != nil {
		log.Printf("auth: resolving tier for user %d: %v", userID, err)
		return plan.TierFree, billing.APITierNone
	}
	now := time.Now().UTC()
	return plan.TierOf(row.ProUntil.Time, row.UltraUntil.Time, now), apiPlanFrom(row, now).Tier
}

// authUserOut is the contract's shape of the account: the same fields userResponse writes
// for the routes that still set a cookie.
func authUserOut(r userResponse) apigen.AuthUser {
	return apigen.AuthUser{ID: r.ID, Email: r.Email, Role: r.Role, BetaTester: r.BetaTester,
		EmailVerified: r.EmailVerified, HasPassword: r.HasPassword, CreatedAt: nilDateTime(r.CreatedAt),
		Timezone: nilString(r.Timezone), Language: r.Language,
		OnboardingCompletedAt: nilDateTime(r.OnboardingCompletedAt),
		Tier:                  apigen.AuthUserTier(r.Tier), AgentConnected: r.AgentConnected,
		APITier: apigen.AuthUserAPITier(r.APITier)}
}

// accountsError maps the accounts service sentinels to HTTP errors.
func accountsError(err error) error {
	switch {
	case errors.Is(err, accounts.ErrInvalidEmail):
		return fiber.NewError(fiber.StatusBadRequest, "invalid email")
	case errors.Is(err, accounts.ErrPasswordTooShort):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at least 8 characters")
	case errors.Is(err, accounts.ErrPasswordTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at most 72 characters")
	case errors.Is(err, accounts.ErrEmailTaken):
		return fiber.NewError(fiber.StatusConflict, "email already registered")
	case errors.Is(err, accounts.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, accounts.ErrAccountSuspended):
		return errAccountSuspended
	case errors.Is(err, accounts.ErrUserNotFound):
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	case errors.Is(err, accounts.ErrInvalidTimezone):
		return fiber.NewError(fiber.StatusBadRequest, "invalid timezone")
	case errors.Is(err, accounts.ErrInvalidLanguage):
		return fiber.NewError(fiber.StatusBadRequest, "invalid language")
	case errors.Is(err, accounts.ErrUsernameInvalid):
		return fiber.NewError(fiber.StatusBadRequest, "invalid username")
	case errors.Is(err, accounts.ErrUsernameTaken):
		return fiber.NewError(fiber.StatusConflict, "username already taken")
	case errors.Is(err, accounts.ErrUsernameReserved):
		return fiber.NewError(fiber.StatusConflict, "username is reserved")
	case errors.Is(err, accounts.ErrUsernameCooldown):
		return fiber.NewError(fiber.StatusTooManyRequests, "username changed too recently")
	default:
		// Anything else is ours: classify answers a client cancel 499 and reports the rest.
		return err
	}
}
