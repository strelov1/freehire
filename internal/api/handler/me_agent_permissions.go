package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

// agentPermissionHandlers serves the account's switch for what a connected MCP agent may do
// beyond reading and tracking (openspec/changes/auto-apply-over-mcp). Today that is one
// thing: acting on auto-apply attempts.
type agentPermissionHandlers struct{ queries *db.Queries }

// register mounts the read with mw.key and the write with mw.cookie ONLY. The switch gates
// what a credential other than the browser session may do, so no such credential may turn it
// on — an agent granting itself the permission it is gated by would make the gate decoration.
func (h *agentPermissionHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/agent-permissions", mw.key, h.Get)
	api.Put("/me/agent-permissions", mw.cookie, h.Put)
}

type agentPermissions struct {
	AgentsMayApply bool `json:"agents_may_apply"`
}

func (h *agentPermissionHandlers) Get(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	allowed, err := h.queries.GetAgentsMayApply(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": agentPermissions{AgentsMayApply: allowed}})
}

func (h *agentPermissionHandlers) Put(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in agentPermissions
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.queries.SetAgentsMayApply(c.Context(), db.SetAgentsMayApplyParams{ID: userID, AgentsMayApply: in.AgentsMayApply}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": in})
}
