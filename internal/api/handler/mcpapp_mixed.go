package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
)

// mountMCP serves the public catalogue and account tools at one URL. Invalid
// credentials fail closed; only a request with no credential is anonymous.
func mountMCP(
	api fiber.Router,
	mw middleware,
	assistants *assistantHandlers,
	catalogue *mcpappHandlers,
) {
	granted := adaptor.HTTPHandler(mcpapp.MixedHandler(mcpAccountTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	keyed := adaptor.HTTPHandler(mcpapp.MixedHandler(mcpKeyTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	api.All("/mcp",
		mcpAuthenticate(assistants, catalogue.origin, mcpapp.MixedResourceMetadataPath, true),
		agentViews(catalogue.agentViews, agentview.SurfaceMCP), func(c *fiber.Ctx) error {
			if viaKey, _ := c.Locals(localsMCPAccountViaKey).(bool); viaKey {
				return keyed(c)
			}
			return granted(c)
		})
}
