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

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// mintAPIKey generates a new API key for the user and persists it (only the hash
// is stored), returning the plaintext token — revealed to the caller exactly once —
// and the stored row. Used by the key-management endpoint (the
// browser-extension connect flow issues a session JWT instead).
func (h *authHandlers) mintAPIKey(ctx context.Context, userID int64, name string, expiresAt pgtype.Timestamptz, scope string) (string, db.CreateAPIKeyRow, error) {
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		// Returned rather than phrased as fiber.NewError(500, …): the reader we need
		// here is the exception log, and a *fiber.Error carries no cause for it to show. The
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

// CreateApiKey mints a new API key for the authenticated user and returns the
// plaintext token exactly once. Behind RequireAuth (cookie-only): a leaked key
// must not be able to mint more keys. An absent/empty scope mints auth.ScopeFull; the
// only other scope a client may request for itself is auth.ScopeEmbed (for the
// volunteer-embed-mvp service, a separate repo that only ever reads/validates api_keys
// directly — this scope grants nothing on freehire's own API). ScopeCV stays
// server-only: no client-requestable value mints it.
func (h *authHandlers) CreateApiKey(ctx context.Context, req *apigen.ApiKeyCreate) (*apigen.CreateApiKeyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name.Value)
	if name == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "name is required")
	}

	scope := auth.ScopeFull
	if req.Scope.Value != "" {
		if req.Scope.Value != auth.ScopeEmbed {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid scope")
		}
		scope = req.Scope.Value
	}

	token, row, err := h.mintAPIKey(ctx, userID, name, pgconv.Timestamptz(ptrOf(req.ExpiresAt.Get)), scope)
	if err != nil {
		return nil, err
	}

	return &apigen.CreateApiKeyCreated{Data: apigen.ApiKeyCreated{
		ID:          row.ID,
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
		Scope:       row.Scope,
		CreatedAt:   nilDateTime(pgconv.TimePtr(row.CreatedAt)),
		LastUsedAt:  nilDateTime(pgconv.TimePtr(row.LastUsedAt)),
		ExpiresAt:   nilDateTime(pgconv.TimePtr(row.ExpiresAt)),
		Token:       token,
	}}, nil
}

// ListApiKeys returns the authenticated user's keys, newest first, as metadata only
// (never the token or its hash). Cookie-only.
func (h *authHandlers) ListApiKeys(ctx context.Context) (*apigen.ListApiKeysOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := h.queries.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	keys := make([]apigen.ApiKey, len(rows))
	for i, r := range rows {
		keys[i] = apigen.ApiKey{
			ID:          r.ID,
			Name:        r.Name,
			TokenPrefix: r.TokenPrefix,
			Scope:       r.Scope,
			CreatedAt:   nilDateTime(pgconv.TimePtr(r.CreatedAt)),
			LastUsedAt:  nilDateTime(pgconv.TimePtr(r.LastUsedAt)),
			ExpiresAt:   nilDateTime(pgconv.TimePtr(r.ExpiresAt)),
		}
	}
	// A user's keys are an unpaginated, owner-scoped set; meta carries the count
	// to keep the list envelope ({"data", "meta"}) consistent with other lists.
	return &apigen.ListApiKeysOK{Data: keys, Meta: apigen.AccountTotalMeta{Total: int64(len(keys))}}, nil
}

// RevokeApiKey deletes one of the authenticated user's keys by id. Owner-scoped: an
// id that does not exist or belongs to another user deletes nothing and is a 404,
// revealing nothing about it. Cookie-only.
func (h *authHandlers) RevokeApiKey(ctx context.Context, params apigen.RevokeApiKeyParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}

	affected, err := h.queries.DeleteAPIKey(ctx, db.DeleteAPIKeyParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "key not found")
	}
	return nil
}

// apiPlanResponse is the account's API product entitlement. The assistant's account tool
// writes it too, so it stays a Go type beside the contract's ApiKeyPlan.
type apiPlanResponse struct {
	Tier  billing.APITier `json:"tier"`
	Until *time.Time      `json:"until,omitempty"`
}

// GetApiPlan reports the caller's API standing. Cookie-only, like the rest of key management.
func (h *authHandlers) GetApiPlan(ctx context.Context) (*apigen.GetApiPlanOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	row, err := h.queries.GetUserAPIEntitlement(ctx, userID)
	if err != nil {
		return nil, err
	}
	plan := apiPlanFrom(row, time.Now())
	out := apigen.ApiKeyPlan{Tier: apigen.ApiKeyPlanTier(plan.Tier)}
	optNil(plan.Until, out.Until.SetTo)
	return &apigen.GetApiPlanOK{Data: out}, nil
}

func apiPlanFrom(row db.GetUserAPIEntitlementRow, now time.Time) apiPlanResponse {
	apiUntil, dataUntil := row.ApiUntilStripe.Time, row.ApiDataUntilStripe.Time
	out := apiPlanResponse{Tier: billing.APITierOf(apiUntil, dataUntil, now)}
	switch out.Tier {
	case billing.APITierData:
		out.Until = &dataUntil
	case billing.APITierAPI:
		out.Until = &apiUntil
	}
	return out
}
