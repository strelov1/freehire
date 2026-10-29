package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// apiKeyResponse is the public, secret-free shape of an API key: the display prefix
// identifies it in a list, but the plaintext token and its stored hash never appear.
type apiKeyResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	TokenPrefix string `json:"token_prefix"`
	// Scope is what this credential may reach: "full" for a user-created key, "cv" for
	// one minted for the tailoring agent. Shown so a user can see what a key can do.
	Scope      string             `json:"scope"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	LastUsedAt pgtype.Timestamptz `json:"last_used_at"`
	ExpiresAt  pgtype.Timestamptz `json:"expires_at"`
}

// createdAPIKeyResponse adds the plaintext token to the key metadata. It is the
// response of CreateAPIKey only — the one and only time the token is revealed; it
// is never persisted (only its hash is) and never returned again.
type createdAPIKeyResponse struct {
	apiKeyResponse
	Token string `json:"token"`
}

// createAPIKeyRequest is the create body: a required display name, an optional
// expiry, and an optional scope. A null/absent expires_at means the key never
// expires. An absent/empty scope mints auth.ScopeFull — unchanged from before this
// field existed, so every existing caller that never sends it keeps getting a
// full-scope key. The only other scope a client may request for itself is
// auth.ScopeEmbed (for the volunteer-embed-mvp service, a separate repo that only
// ever reads/validates api_keys directly — this scope grants nothing on freehire's
// own API). ScopeCV stays server-only: no client-requestable value mints it.
type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
	Scope     string     `json:"scope"`
}

// mintAPIKey generates a new API key for the user and persists it (only the hash
// is stored), returning the plaintext token — revealed to the caller exactly once —
// and the stored row. Used by the key-management endpoint (the
// browser-extension connect flow issues a session JWT instead).
func (h *authHandlers) mintAPIKey(ctx context.Context, userID int64, name string, expiresAt pgtype.Timestamptz, scope string) (string, db.CreateAPIKeyRow, error) {
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		// Returned rather than phrased as fiber.NewError(500, …): the reader we need
		// here is Sentry, and a *fiber.Error carries no cause for it to show. The
		// caller sees the generic 500 body instead — see classify.
		return "", db.CreateAPIKeyRow{}, fmt.Errorf("generate api key for user %d: %w", userID, err)
	}
	row, err := h.queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        name,
		TokenHash:   hash,
		TokenPrefix: prefix,
		Scope:       scope,
		ExpiresAt:   expiresAt,
	})
	// The statement inserts nothing — and so returns no row — when the account's address
	// was never proven. The caller is authenticated (a session cookie got them here), so
	// this is an authorization refusal, not a failed lookup: 403, not 404. The SPA reads
	// email_verified from /auth/me and hides the button, so this is the guard for a
	// direct API call rather than the message a user normally meets.
	if errors.Is(err, pgx.ErrNoRows) {
		return "", db.CreateAPIKeyRow{}, fiber.NewError(fiber.StatusForbidden,
			"confirm your email address before creating an API key")
	}
	if err != nil {
		return "", db.CreateAPIKeyRow{}, err
	}
	return token, row, nil
}

// CreateAPIKey mints a new API key for the authenticated user and returns the
// plaintext token exactly once. Behind RequireAuth (cookie-only): a leaked key
// must not be able to mint more keys.
func (h *authHandlers) CreateAPIKey(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createAPIKeyRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}

	scope := auth.ScopeFull
	if in.Scope != "" {
		if in.Scope != auth.ScopeEmbed {
			return fiber.NewError(fiber.StatusBadRequest, "invalid scope")
		}
		scope = in.Scope
	}

	token, row, err := h.mintAPIKey(c.Context(), userID, name, pgconv.Timestamptz(in.ExpiresAt), scope)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": createdAPIKeyResponse{
			apiKeyResponse: apiKeyResponse{
				ID:          row.ID,
				Name:        row.Name,
				TokenPrefix: row.TokenPrefix,
				Scope:       row.Scope,
				CreatedAt:   row.CreatedAt,
				LastUsedAt:  row.LastUsedAt,
				ExpiresAt:   row.ExpiresAt,
			},
			Token: token,
		},
	})
}

// ListAPIKeys returns the authenticated user's keys, newest first, as metadata only
// (never the token or its hash). Cookie-only.
func (h *authHandlers) ListAPIKeys(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := h.queries.ListAPIKeysByUser(c.Context(), userID)
	if err != nil {
		return err
	}

	keys := make([]apiKeyResponse, len(rows))
	for i, r := range rows {
		keys[i] = apiKeyResponse{
			ID:          r.ID,
			Name:        r.Name,
			TokenPrefix: r.TokenPrefix,
			Scope:       r.Scope,
			CreatedAt:   r.CreatedAt,
			LastUsedAt:  r.LastUsedAt,
			ExpiresAt:   r.ExpiresAt,
		}
	}
	// A user's keys are an unpaginated, owner-scoped set; meta carries the count
	// to keep the list envelope ({"data", "meta"}) consistent with other lists.
	return c.JSON(fiber.Map{"data": keys, "meta": fiber.Map{"total": len(keys)}})
}

// RevokeAPIKey deletes one of the authenticated user's keys by id. Owner-scoped: an
// id that does not exist or belongs to another user deletes nothing and is a 404,
// revealing nothing about it. Cookie-only.
func (h *authHandlers) RevokeAPIKey(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	affected, err := h.queries.DeleteAPIKey(c.Context(), db.DeleteAPIKeyParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "key not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// apiPlanResponse is the account's API standing as its key-management page shows it: which API
// product tier it holds and until when, and the rate tier its full-scope keys and access
// tokens get on public reads. An account without the API product has an empty tier and still a
// rate — free, Pro or Ultra.
type apiPlanResponse struct {
	Tier  billing.APITier `json:"tier"`
	Until *time.Time      `json:"until,omitempty"`
	// RateTier is the rung of the public-read ladder the account's keys stand on.
	RateTier rateTier `json:"rate_tier"`
	// PerMinute and PerDay are that rung's budget; PerDay 0 is no daily cap.
	PerMinute int `json:"per_minute"`
	PerDay    int `json:"per_day"`
}

// GetAPIPlan reports the caller's API standing. Cookie-only, like the rest of key management.
func (h *authHandlers) GetAPIPlan(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	row, err := h.queries.GetUserAPIEntitlement(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": apiPlanFrom(row, h.rateTiers, time.Now())})
}

func apiPlanFrom(row db.GetUserAPIEntitlementRow, tiers RateTiers, now time.Time) apiPlanResponse {
	apiUntil, dataUntil := row.ApiUntilStripe.Time, row.ApiDataUntilStripe.Time
	rt := accountRateTier(apiUntil, dataUntil, row.ProUntil.Time, row.UltraUntil.Time, now)
	out := apiPlanResponse{Tier: billing.APITierOf(apiUntil, dataUntil, now), RateTier: rt,
		PerMinute: tiers[rt].PerMinute, PerDay: tiers[rt].PerDay}
	switch out.Tier {
	case billing.APITierData:
		out.Until = &dataUntil
	case billing.APITierAPI:
		out.Until = &apiUntil
	}
	return out
}
