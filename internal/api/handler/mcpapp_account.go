package handler

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

// mountMCPAccount wires the signed-in MCP server at /api/v1/mcp/account. It
// takes the already-constructed assistantHandlers so it can reuse its tools
// verbatim — the same capabilities the in-app assistant exposes, not a second
// implementation of them. Which of them are served is mcpAccountTools' answer.
func mountMCPAccount(api fiber.Router, mw middleware, assistants *assistantHandlers, frontendOrigin string) {
	tools := mcpAccountTools(assistants)
	// Same reasoning as the public /mcp mount: agentSearchLimiter bounds how
	// hard a machine may ask, applied here too since every tool still reaches
	// the catalogue and the tracking board. Authentication runs as a real
	// Fiber middleware, ahead of the adapted raw http.Handler, so an
	// unexpected failure authenticating the token (not merely an unknown one)
	// reaches Sentry the normal way — see mcpAccountAuthenticate.
	api.All("/mcp/account", chatAgentLimiter(mw.public), mcpAccountAuthenticate(assistants, frontendOrigin), adaptor.HTTPHandler(mcpapp.AccountHandler(tools, frontendOrigin)))
}

// mcpAccountTools is the in-app assistant's discovery and tracking tools, less
// the one that only works inside our own client.
//
// present_jobs draws vacancy cards in freehire's chat UI and returns a bare
// receipt of slugs; its description tells the model that it is the ONLY way a
// vacancy reaches the screen and to never write a job link. In Claude or ChatGPT
// there is no card renderer, so a model obeying that description shows the
// person nothing at all. Without the tool, the model writes the links
// search_jobs already hands it.
func mcpAccountTools(assistants *assistantHandlers) []assistant.Tool {
	all := append(assistants.assistantDiscoveryTools(), assistants.assistantTrackingTools()...)
	all = append(all, assistants.autoApplyMCPTools()...)
	return slices.DeleteFunc(all, func(t assistant.Tool) bool { return t.Name == presentJobsToolName })
}

// mcpAccountAuthenticate resolves the Authorization: Bearer token to a user id
// and stamps it onto the request as mcpapp.AccountUserIDHeader, overwriting
// whatever a caller sent there — nothing downstream of this middleware ever
// sees an unvalidated value. A missing or unrecognized token is a routine 401
// (never reported, per this package's Error Convention); any other failure
// resolving it — a database error, not an unknown token — is returned bare so
// the fall-through renders a 500 AND reports it, instead of being folded into
// the same "invalid token" answer a real caller mistake gets.
//
// Every 401 carries a WWW-Authenticate challenge naming the protected-resource
// metadata. The MCP authorization spec requires it: it is how a client that has
// only this URL discovers which authorization server to sign in with. Without
// it a client must guess the well-known path, and one that does not guess has
// no way to connect at all.
func mcpAccountAuthenticate(assistants *assistantHandlers, frontendOrigin string) fiber.Handler {
	challenge := `Bearer resource_metadata="` + frontendOrigin + `/.well-known/oauth-protected-resource"`
	return func(c *fiber.Ctx) error {
		const prefix = "Bearer "
		auth := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(auth, prefix) {
			c.Set(fiber.HeaderWWWAuthenticate, challenge)
			return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
		}
		token := strings.TrimPrefix(auth, prefix)

		userID, err := assistants.queries.AuthenticateOAuthGrant(c.Context(), oauth2server.HashToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			// RFC 6750 §3.1: a presented token we refuse is invalid_token, which
			// tells the client to sign in again rather than retry the same token.
			c.Set(fiber.HeaderWWWAuthenticate, challenge+`, error="invalid_token"`)
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}
		if err != nil {
			return fmt.Errorf("authenticate oauth grant for mcp account: %w", err)
		}

		c.Request().Header.Set(mcpapp.AccountUserIDHeader, strconv.FormatInt(userID, 10))
		return c.Next()
	}
}
