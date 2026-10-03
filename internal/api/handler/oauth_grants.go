package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// oauthGrantResponse is one "connected device": metadata only, never the token
// hash. client_name is what the consent screen showed, so revocation can be
// decided from the same name.
type oauthGrantResponse struct {
	ID         int64              `json:"id"`
	ClientName string             `json:"client_name"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	ExpiresAt  pgtype.Timestamptz `json:"expires_at"`
	LastUsedAt pgtype.Timestamptz `json:"last_used_at"`
}

// ListOAuthGrants returns the authenticated user's connected MCP clients, newest
// first. Cookie-only, same reasoning as ListAPIKeys: this is account management,
// not something a programmatic credential should read about itself.
func (h *authHandlers) ListOAuthGrants(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.queries.ListOAuthGrantsByUser(c.Context(), userID)
	if err != nil {
		return err
	}
	grants := make([]oauthGrantResponse, len(rows))
	for i, r := range rows {
		grants[i] = oauthGrantResponse{
			ID: r.ID, ClientName: r.ClientName,
			CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, LastUsedAt: r.LastUsedAt,
		}
	}
	// Unpaginated, owner-scoped set, same envelope shape as ListAPIKeys.
	return c.JSON(fiber.Map{"data": grants, "meta": fiber.Map{"total": len(grants)}})
}

// RevokeOAuthGrant deletes one of the authenticated user's connected clients by
// id. Owner-scoped: an id that does not exist or belongs to another user deletes
// nothing and is a 404 — identical contract to RevokeAPIKey. Cookie-only.
func (h *authHandlers) RevokeOAuthGrant(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	affected, err := h.queries.DeleteOAuthGrant(c.Context(), db.DeleteOAuthGrantParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "grant not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
