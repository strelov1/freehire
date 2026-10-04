package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	// Same reasoning as the public /mcp mount: agentSearchLimiter bounds how
	// hard a machine may ask, applied here too since every tool still reaches
	// the catalogue and the tracking board. Authentication runs as a real
	// Fiber middleware, ahead of the adapted raw http.Handler, so an
	// unexpected failure authenticating the token (not merely an unknown one)
	// reaches Sentry the normal way — see mcpAccountAuthenticate.
	api.All("/mcp/account", agentSearchLimiter(mw.throttler), mcpAccountAuthenticate(assistants), adaptor.HTTPHandler(mcpapp.AccountHandler(tools)))
}

// mcpAccountAuthenticate resolves the Authorization: Bearer token to a user id
// and stamps it onto the request as mcpapp.AccountUserIDHeader, overwriting
// whatever a caller sent there — nothing downstream of this middleware ever
// sees an unvalidated value. A missing or unrecognized token is a routine 401
// (never reported, per this package's Error Convention); any other failure
// resolving it — a database error, not an unknown token — is returned bare so
// the fall-through renders a 500 AND reports it, instead of being folded into
// the same "invalid token" answer a real caller mistake gets.
func mcpAccountAuthenticate(assistants *assistantHandlers) fiber.Handler {
	return func(c *fiber.Ctx) error {
		const prefix = "Bearer "
		auth := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(auth, prefix) {
			return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
		}
		token := strings.TrimPrefix(auth, prefix)

		userID, err := assistants.queries.AuthenticateOAuthGrant(c.Context(), oauth2server.HashToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}
		if err != nil {
			return fmt.Errorf("authenticate oauth grant for mcp account: %w", err)
		}

		c.Request().Header.Set(mcpapp.AccountUserIDHeader, strconv.FormatInt(userID, 10))
		return c.Next()
	}
}
