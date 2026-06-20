package handler

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// apiKeyResponse is the public, secret-free shape of an API key: the display prefix
// identifies it in a list, but the plaintext token and its stored hash never appear.
type apiKeyResponse struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	TokenPrefix string             `json:"token_prefix"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	LastUsedAt  pgtype.Timestamptz `json:"last_used_at"`
	ExpiresAt   pgtype.Timestamptz `json:"expires_at"`
}

// createdAPIKeyResponse adds the plaintext token to the key metadata. It is the
// response of CreateAPIKey only — the one and only time the token is revealed; it
// is never persisted (only its hash is) and never returned again.
type createdAPIKeyResponse struct {
	apiKeyResponse
	Token string `json:"token"`
}

// createAPIKeyRequest is the create body: a required display name and an optional
// expiry. A null/absent expires_at means the key never expires.
type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// CreateAPIKey mints a new API key for the authenticated user and returns the
// plaintext token exactly once. Behind RequireAuth (cookie-only): a leaked key
// must not be able to mint more keys.
func (a *API) CreateAPIKey(c *fiber.Ctx) error {
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

	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate key")
	}

	var expiresAt pgtype.Timestamptz
	if in.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}

	row, err := a.queries.CreateAPIKey(c.Context(), db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        name,
		TokenHash:   hash,
		TokenPrefix: prefix,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": createdAPIKeyResponse{
			apiKeyResponse: apiKeyResponse{
				ID:          row.ID,
				Name:        row.Name,
				TokenPrefix: row.TokenPrefix,
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
func (a *API) ListAPIKeys(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := a.queries.ListAPIKeysByUser(c.Context(), userID)
	if err != nil {
		return err
	}

	keys := make([]apiKeyResponse, len(rows))
	for i, r := range rows {
		keys[i] = apiKeyResponse{
			ID:          r.ID,
			Name:        r.Name,
			TokenPrefix: r.TokenPrefix,
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
func (a *API) RevokeAPIKey(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	affected, err := a.queries.DeleteAPIKey(c.Context(), db.DeleteAPIKeyParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "key not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
