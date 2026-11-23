package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplySettingsHandlers serves the account's auto-apply preferences. Today that is one
// switch: whether every attempt waits for the person's approval, or only those whose form
// asks something only they can answer.
type autoApplySettingsHandlers struct{ queries *db.Queries }

// register mounts the read with mw.key and the write with mw.cookie only: whether attempts go
// out unreviewed is the person's call, never a connected agent's.
func (h *autoApplySettingsHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/auto-apply-settings", mw.key, contract)
	api.Put("/me/auto-apply-settings", mw.cookie, contract)
}

// GetAutoApplySettings reports the caller's switch.
func (h *autoApplySettingsHandlers) GetAutoApplySettings(ctx context.Context) (*apigen.GetAutoApplySettingsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	confirmEach, err := h.queries.GetAutoApplyConfirmEach(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetAutoApplySettingsOK{Data: apigen.AutoApplySettings{ConfirmEach: confirmEach}}, nil
}

// SetAutoApplySettings stores the switch. An absent field turns it off.
func (h *autoApplySettingsHandlers) SetAutoApplySettings(ctx context.Context, req *apigen.AutoApplySettingsUpdate) (*apigen.SetAutoApplySettingsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	confirmEach := req.ConfirmEach.Value
	if err := h.queries.SetAutoApplyConfirmEach(ctx, db.SetAutoApplyConfirmEachParams{ID: userID, AutoApplyConfirmEach: confirmEach}); err != nil {
		return nil, err
	}
	return &apigen.SetAutoApplySettingsOK{Data: apigen.AutoApplySettings{ConfirmEach: confirmEach}}, nil
}
