package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

// mountMCPAccount wires the signed-in MCP server at /api/v1/mcp/account. It
// takes the already-constructed assistantHandlers so it can reuse its
// assistantDiscoveryTools()/assistantTrackingTools() verbatim — the same
// capabilities the in-app assistant exposes, not a second implementation of them.
func mountMCPAccount(api fiber.Router, mw middleware, assistants *assistantHandlers) {
	tools := append(assistants.assistantDiscoveryTools(), assistants.assistantTrackingTools()...)
	authenticate := func(ctx context.Context, token string) (int64, error) {
		userID, err := assistants.queries.AuthenticateOAuthGrant(ctx, oauth2server.HashToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, mcpapp.ErrUnauthorized
		}
		return userID, err
	}
	// Same reasoning as the public /mcp mount: agentSearchLimiter bounds how
	// hard a machine may ask, applied here too since every tool still reaches
	// the catalogue and the tracking board.
	api.All("/mcp/account", agentSearchLimiter(mw.throttler), adaptor.HTTPHandler(mcpapp.AccountHandler(tools, authenticate)))
}
