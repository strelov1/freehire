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
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// mountMCPAccount serves /api/v1/mcp/account, the original URL of the signed-in server, on
// the same server as /mcp. It stays signed-in only: no bearer token is a 401, never anonymous.
// HTTP admission is enforced by the operations gateway; authentication stays in Fiber so
// credential-store failures use the normal error pipeline.
func mountMCPAccount(
	api fiber.Router,
	mw middleware,
	assistants *assistantHandlers,
	catalogue *mcpappHandlers,
) {
	mountMCPServer(api, "/mcp/account", mcpAccountAuthenticate(assistants, catalogue.origin),
		agentview.SurfaceMCPAccount, assistants, catalogue)
}

// mcpAccountTools is the in-app assistant's discovery and tracking tools, less
// the one that only works inside our own client, plus job alerts, dictionary suggestions,
// auto-apply, and the interview-buddy and mock-interview tools (cookie-only over REST, so
// grant-only here).
//
// present_jobs draws vacancy cards in freehire's chat UI and returns a bare
// receipt of slugs; its description tells the model that it is the ONLY way a
// vacancy reaches the screen and to never write a job link. In Claude or ChatGPT
// there is no card renderer, so a model obeying that description shows the
// person nothing at all. Without the tool, the model writes the links
// search_jobs already hands it.
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

// mcpKeyTools is what the same server offers a full-scope API key: everything an
// OAuth grant gets except cookie-only actions (auto-apply, dictionary suggestions, problem
// reports and interview buddies).
//
// A key reaches this server so the local MCP package (freehire-mcp), which holds a
// key and no grant, can manage job alerts through these tools rather than through
// a second implementation of them. Auto-apply stays with the grant: it sends real
// applications in the person's name, and over REST it has never been open to a key
// (its routes are cookie-only), so arriving here is not a reason to widen it.
func mcpKeyTools(assistants *assistantHandlers, origin string) []assistant.Tool {
	all := append(assistants.assistantDiscoveryTools(), assistants.assistantTrackingTools()...)
	all = append(all, assistants.reportMCPTools()...)
	all = append(all, assistants.subscriptionMCPTools(origin)...)
	// REST /me/inbox admits a full-scope key, so the headers-only mail read does too.
	all = append(all, assistants.unreadMailTool(origin))
	// Read-only, and GET /me/plan is open to a key over REST.
	all = append(all, assistants.planMCPTool(origin))
	// An MCP client keeps every result in its context, so it gets the bounded facets.
	// search_jobs, get_job and get_company stay in the list only so the instructions and
	// prompts see them as served; the catalogue's own tools answer those names.
	for i, t := range all {
		if t.Name == "facets" {
			all[i] = assistants.facets(false)
		}
	}
	return slices.DeleteFunc(all, func(t assistant.Tool) bool { return t.Name == presentJobsToolName })
}

// localsMCPAccountViaKey marks a request mcpAccountAuthenticate resolved through an
// API key rather than an OAuth grant, so the mount serves it mcpKeyTools.
const localsMCPAccountViaKey = "mcpAccount.viaKey"

// localsMCPUserID carries the validated identity to Fiber consumers such as agentViews.
const localsMCPUserID = "mcp.userID"

// mcpAccountAuthenticate resolves the Authorization: Bearer token to a user id
// and stamps it onto the request as mcpapp.AccountUserIDHeader, overwriting
// whatever a caller sent there — nothing downstream of this middleware ever
// sees an unvalidated value. The token is an OAuth grant's or, failing that, a
// full-scope API key's — the mirror of apiKeys.AuthenticateAPIKey, which lets a
// grant into every REST route a key may use. A narrow (cv) key is a 403: the
// credential is real, it is simply not allowed here. A missing or unrecognized
// token is a routine 401 (never reported, per this package's Error Convention);
// any other failure resolving it — a database error, not an unknown token — is
// returned bare so the fall-through renders a 500 AND reports it, instead of
// being folded into the same "invalid token" answer a real caller mistake gets.
//
// Every 401 carries a WWW-Authenticate challenge naming the protected-resource
// metadata. The MCP authorization spec requires it: it is how a client that has
// only this URL discovers which authorization server to sign in with. Without
// it a client must guess the well-known path, and one that does not guess has
// no way to connect at all.
func mcpAccountAuthenticate(assistants *assistantHandlers, frontendOrigin string) fiber.Handler {
	return mcpAuthenticate(assistants, frontendOrigin, "/.well-known/oauth-protected-resource", false)
}

func mcpAuthenticate(
	assistants *assistantHandlers,
	frontendOrigin string,
	metadataPath string,
	allowAnonymous bool,
) fiber.Handler {
	challenge := `Bearer resource_metadata="` + strings.TrimSuffix(frontendOrigin, "/") + metadataPath + `"`
	return func(c *fiber.Ctx) error {
		observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "unknown"})
		c.Request().Header.Del(mcpapp.AccountUserIDHeader)
		const prefix = "Bearer "
		authorization := c.Get(fiber.HeaderAuthorization)
		if authorization == "" && allowAnonymous {
			return c.Next()
		}
		if !strings.HasPrefix(authorization, prefix) {
			observability.LogAuthFailure(c, fiber.StatusUnauthorized, "missing_bearer_token")
			c.Set(fiber.HeaderWWWAuthenticate, challenge)
			return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
		}
		token := strings.TrimPrefix(authorization, prefix)

		userID, err := assistants.queries.AuthenticateOAuthGrant(c.Context(), oauth2server.HashToken(token))
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
				return fmt.Errorf("authenticate api key for mcp account: %w", keyErr)
			}
			observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "api_key", UserID: key.UserID, KeyID: key.ID, Scope: key.Scope})
			if key.Scope != auth.ScopeFull {
				observability.LogAuthFailure(c, fiber.StatusForbidden, "insufficient_scope")
				return fiber.NewError(fiber.StatusForbidden, "api key scope is insufficient for this endpoint")
			}
			userID, err = key.UserID, nil
			c.Locals(localsMCPAccountViaKey, true)
		}
		if err != nil {
			observability.LogAuthFailure(c, fiber.StatusInternalServerError, "credential_lookup_failed")
			return fmt.Errorf("authenticate oauth grant for mcp account: %w", err)
		}
		if c.Locals(localsMCPAccountViaKey) == nil {
			observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "oauth_grant", UserID: userID})
		}

		c.Request().Header.Set(mcpapp.AccountUserIDHeader, strconv.FormatInt(userID, 10))
		c.Locals(localsMCPUserID, userID)
		return c.Next()
	}
}
