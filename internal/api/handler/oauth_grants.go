package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ListOauthGrants returns the authenticated user's connected MCP clients (metadata
// only, never the token hash; client_name is what the consent screen showed), newest
// first. Cookie-only, same reasoning as ListApiKeys: this is account management,
// not something a programmatic credential should read about itself.
func (h *authHandlers) ListOauthGrants(ctx context.Context) (*apigen.ListOauthGrantsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.ListOAuthGrantsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	grants := make([]apigen.AccountOauthGrant, len(rows))
	for i, r := range rows {
		grants[i] = apigen.AccountOauthGrant{
			ID: r.ID, ClientName: r.ClientName,
			CreatedAt:  nilDateTime(pgconv.TimePtr(r.CreatedAt)),
			ExpiresAt:  nilDateTime(pgconv.TimePtr(r.ExpiresAt)),
			LastUsedAt: nilDateTime(pgconv.TimePtr(r.LastUsedAt)),
		}
	}
	// Unpaginated, owner-scoped set, same envelope shape as ListApiKeys.
	return &apigen.ListOauthGrantsOK{Data: grants, Meta: apigen.AccountTotalMeta{Total: int64(len(grants))}}, nil
}

// RevokeOauthGrant deletes one of the authenticated user's connected clients by
// id. Owner-scoped: an id that does not exist or belongs to another user deletes
// nothing and is a 404 — identical contract to RevokeApiKey. Cookie-only.
func (h *authHandlers) RevokeOauthGrant(ctx context.Context, params apigen.RevokeOauthGrantParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	affected, err := h.queries.DeleteOAuthGrant(ctx, db.DeleteOAuthGrantParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "grant not found")
	}
	return nil
}
