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
	mountMCPServer(api, "/mcp",
		mcpAuthenticate(assistants, catalogue.origin, mcpapp.MixedResourceMetadataPath, true),
		agentview.SurfaceMCP, assistants, catalogue)
}

// mountMCPServer mounts the one MCP server at path: an OAuth grant gets mcpAccountTools, a
// full-scope API key mcpKeyTools. authenticate decides whether an anonymous caller gets in.
func mountMCPServer(
	api fiber.Router,
	path string,
	authenticate fiber.Handler,
	surface string,
	assistants *assistantHandlers,
	catalogue *mcpappHandlers,
) {
	granted := adaptor.HTTPHandler(mcpapp.MixedHandler(mcpAccountTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	keyed := adaptor.HTTPHandler(mcpapp.MixedHandler(mcpKeyTools(assistants, catalogue.origin), catalogue.origin, catalogue))
	api.All(path, authenticate, agentViews(catalogue.agentViews, surface), func(c *fiber.Ctx) error {
		if viaKey, _ := c.Locals(localsMCPAccountViaKey).(bool); viaKey {
			return keyed(c)
		}
		return granted(c)
	})
}
