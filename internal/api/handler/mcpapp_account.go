package handler

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// mcpAccountTools is the in-app assistant's discovery and tracking tools, less the one that
// only works inside our own client, plus job alerts, dictionary suggestions, auto-apply, and
// the interview-buddy and mock-interview tools (cookie-only over REST, so grant-only here).
//
// present_jobs is left out: it draws cards in freehire's chat UI and its description says it
// is the only way a vacancy reaches the screen, so in Claude or ChatGPT (no card renderer) a
// model obeying it would show nothing. Without it the model writes the links search_jobs gives.
func mcpAccountTools(assistants *assistantHandlers, origin string) []assistant.Tool {
	all := append(mcpKeyTools(assistants, origin), assistants.autoApplyMCPTools()...)
	// Dictionary suggestions are cookie-only over REST; keep the corresponding
	// account tool with OAuth grants rather than widening full-scope API keys.
	all = append(all, assistants.dictionaryMCPTools()...)
	// Profile writes are cookie-only over REST (PUT /me/profile), so grant-only here too.
	all = append(all, assistants.profileMCPTools()...)
	// Storing a CV and reading its status are cookie-only over REST (PUT/GET /me/resume).
	all = append(all, assistants.cvSetupMCPTools(origin)...)
	all = append(all, assistants.buddies.mcpTools()...)
	return append(all, assistants.problemMCPTools()...)
}

// mcpKeyTools is what the same server offers a full-scope API key: everything an OAuth grant
// gets except cookie-only actions (auto-apply, dictionary suggestions, problem reports and
// interview buddies).
//
// A key reaches this server so the local freehire-mcp package, which holds a key and no
// grant, can manage job alerts without a second implementation. Auto-apply stays with the
// grant: it sends real applications in the person's name and has never been open to a key.
func mcpKeyTools(assistants *assistantHandlers, origin string) []assistant.Tool {
	all := append(assistants.assistantDiscoveryTools(), assistants.assistantTrackingTools()...)
	all = append(all, assistants.reportMCPTools()...)
	all = append(all, assistants.subscriptionMCPTools(origin)...)
	// REST /me/inbox admits a full-scope key, so the headers-only mail read does too.
	all = append(all, assistants.unreadMailTool(origin))
	// Read-only, and GET /me/plan is open to a key over REST.
	all = append(all, assistants.planMCPTool(origin))
	// An MCP client keeps every result in its context, so it gets the bounded facets.
	// search_jobs, get_job and get_company are listed only so the instructions and prompts see
	// them as served; the catalogue's own tools answer those names.
	for i, t := range all {
		if t.Name == "facets" {
			all[i] = assistants.facets(false)
		}
	}
	return slices.DeleteFunc(all, func(t assistant.Tool) bool { return t.Name == presentJobsToolName })
}

// localsMCPViaKey marks a request mcpAuthenticate resolved through an
// API key rather than an OAuth grant, so the mount serves it mcpKeyTools.
const localsMCPViaKey = "mcpAccount.viaKey"

// localsMCPUserID carries the validated identity to Fiber consumers such as agentViews.
const localsMCPUserID = "mcp.userID"

// mcpAuthenticate resolves the Authorization: Bearer token to a user id and stamps it onto
// the request as mcpapp.AccountUserIDHeader, overwriting whatever a caller sent, so nothing
// downstream sees an unvalidated value. The token is an OAuth grant's or, failing that, a
// full-scope API key's (the mirror of apiKeys.AuthenticateAPIKey). A narrow (cv) key is a
// 403. A missing or unrecognized token is a routine 401 (never reported); any other failure
// resolving it, such as a database error, is returned bare so it renders a 500 and is
// reported rather than folded into "invalid token".
//
// There is no anonymous session. Every 401 carries a WWW-Authenticate challenge naming the
// protected-resource metadata, and the 401 on the first request is the MCP authorization
// spec's only signal that sign-in is needed: Claude.ai, Grok, Cursor and the CLIs open
// sign-in from it while a connector is being added, and never later.
func mcpAuthenticate(assistants *assistantHandlers, frontendOrigin string) fiber.Handler {
	challenge := `Bearer resource_metadata="` + strings.TrimSuffix(frontendOrigin, "/") + mcpapp.ResourceMetadataPath + `"`
	return func(c *fiber.Ctx) error {
		observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "unknown"})
		c.Request().Header.Del(mcpapp.AccountUserIDHeader)
		const prefix = "Bearer "
		authorization := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(authorization, prefix) {
			observability.LogAuthFailure(c, fiber.StatusUnauthorized, "missing_bearer_token")
			c.Set(fiber.HeaderWWWAuthenticate, challenge)
			return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
		}
		token := strings.TrimPrefix(authorization, prefix)

		// The grant lookup refuses a grant bound to another resource (it matches on the endpoint
		// path), so a wrong-resource token reads as unknown here: no last_used_at touch, and the
		// same invalid_token answer as any other refused token.
		grant, err := assistants.queries.AuthenticateOAuthGrant(c.Context(), db.AuthenticateOAuthGrantParams{
			AccessTokenHash: oauth2server.HashToken(token),
			Endpoint:        pgconv.Text(oauth2server.MCPResourcePath),
		})
		userID := grant.UserID
		if errors.Is(err, pgx.ErrNoRows) {
			key, keyErr := assistants.queries.AuthenticateAPIKey(c.Context(), auth.HashAPIKey(token))
			switch {
			case errors.Is(keyErr, pgx.ErrNoRows):
				// RFC 6750 §3.1: a presented token we refuse is invalid_token, which
				// tells the client to sign in again rather than retry the same token.
				c.Set(fiber.HeaderWWWAuthenticate, challenge+`, error="invalid_token"`)
				observability.LogAuthFailure(c, fiber.StatusUnauthorized, "invalid_or_expired_credential")
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
			case keyErr != nil:
				observability.LogAuthFailure(c, fiber.StatusInternalServerError, "credential_lookup_failed")
				return fmt.Errorf("authenticate api key for mcp: %w", keyErr)
			}
			observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "api_key", UserID: key.UserID, KeyID: key.ID, Scope: key.Scope})
			if key.Scope != auth.ScopeFull {
				observability.LogAuthFailure(c, fiber.StatusForbidden, "insufficient_scope")
				return fiber.NewError(fiber.StatusForbidden, "api key scope is insufficient for this endpoint")
			}
			userID, err = key.UserID, nil
			c.Locals(localsMCPViaKey, true)
		}
		if err != nil {
			observability.LogAuthFailure(c, fiber.StatusInternalServerError, "credential_lookup_failed")
			return fmt.Errorf("authenticate oauth grant for mcp: %w", err)
		}
		if c.Locals(localsMCPViaKey) == nil {
			observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "oauth_grant", UserID: userID})
		}

		c.Request().Header.Set(mcpapp.AccountUserIDHeader, strconv.FormatInt(userID, 10))
		c.Locals(localsMCPUserID, userID)
		return c.Next()
	}
}
