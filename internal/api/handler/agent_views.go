package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/search/search"
)

// agentViews marks a request as an agent's, so the postings its handler serves are
// recorded (internal/application/agentview). The scope lives in fasthttp's user values,
// which RequestCtx.Value exposes as a context — the same context the MCP adaptor hands
// the SDK — so Fiber handlers and MCP tools reach it alike. A nil sink records nothing.
//
// The viewer is the account when one is known — a signed-in caller, or on /mcp/account
// the header mcpAccountAuthenticate has already overwritten with a verified id, or the owner
// of a full-scope API key the public gate verified — and
// otherwise the client IP (c.IP(), already resolved from the trusted proxy's X-Real-IP)
// with the User-Agent.
func agentViews(sink agentview.Sink, surface string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if sink == nil {
			return c.Next()
		}
		userID, _ := auth.UserID(c)
		if userID == 0 {
			userID, _ = c.Locals(localsGateKeyUser).(int64)
		}
		// The header is trusted only where mcpAccountAuthenticate has just rewritten it;
		// on the public surfaces a client could send any value.
		if userID == 0 && surface == agentview.SurfaceMCPAccount {
			userID, _ = strconv.ParseInt(c.Get(mcpapp.AccountUserIDHeader), 10, 64)
		}
		agentview.Attach(c.Context().SetUserValue, sink, surface,
			agentview.ViewerKey(userID, c.IP(), c.Get(fiber.HeaderUserAgent)))
		return c.Next()
	}
}

// hitIDs is the job ids of a search answer, in order.
func hitIDs(hits []search.JobDocument) []int64 {
	ids := make([]int64, len(hits))
	for i := range hits {
		ids[i] = hits[i].ID
	}
	return ids
}
