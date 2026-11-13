package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
)

// mountMCP serves the catalogue and account tools at /api/v1/mcp, to signed-in callers only:
// an OAuth grant gets mcpAccountTools, a full-scope API key mcpKeyTools, and a request with
// no credential is a 401 (mcpAuthenticate).
func mountMCP(
	api fiber.Router,
	assistants *assistantHandlers,
	catalogue *mcpappHandlers,
) {
	granted := adaptor.HTTPHandler(mcpapp.Handler(mcpAccountTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	keyed := adaptor.HTTPHandler(mcpapp.Handler(mcpKeyTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	api.All("/mcp", mcpAuthenticate(assistants, catalogue.origin),
		agentViews(catalogue.agentViews, agentview.SurfaceMCP), func(c *fiber.Ctx) error {
			if viaKey, _ := c.Locals(localsMCPViaKey).(bool); viaKey {
				return keyed(c)
			}
			return granted(c)
		})
}
