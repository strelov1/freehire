package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

// agentPermissionHandlers serves the account's switch for what a connected MCP agent may do
// beyond reading and tracking (openspec/changes/auto-apply-over-mcp). Today that is one
// thing: acting on auto-apply attempts.
type agentPermissionHandlers struct{ queries *db.Queries }

// register mounts the read with mw.key and the write with mw.cookie ONLY. The switch gates
// what a credential other than the browser session may do, so no such credential may turn it
// on — an agent granting itself the permission it is gated by would make the gate decoration.
func (h *agentPermissionHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/agent-permissions", mw.key, contract)
	api.Put("/me/agent-permissions", mw.cookie, contract)
}

// GetAgentPermissions reports the caller's switch.
func (h *agentPermissionHandlers) GetAgentPermissions(ctx context.Context) (*apigen.GetAgentPermissionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	allowed, err := h.queries.GetAgentsMayApply(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetAgentPermissionsOK{Data: apigen.AgentPermissions{AgentsMayApply: allowed}}, nil
}

// SetAgentPermissions stores the switch. An absent field turns it off, as it always did.
func (h *agentPermissionHandlers) SetAgentPermissions(ctx context.Context, req *apigen.AgentPermissionsUpdate) (*apigen.SetAgentPermissionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	allowed := req.AgentsMayApply.Value
	if err := h.queries.SetAgentsMayApply(ctx, db.SetAgentsMayApplyParams{ID: userID, AgentsMayApply: allowed}); err != nil {
		return nil, err
	}
	return &apigen.SetAgentPermissionsOK{Data: apigen.AgentPermissions{AgentsMayApply: allowed}}, nil
}
