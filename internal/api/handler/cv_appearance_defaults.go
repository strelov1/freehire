package handler

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
)

// appearanceDefaultsResult is the wire shape for a user's saved (or system-fallback) CV
// appearance defaults — the same template/typography/margins shape a CV document itself
// carries, without the rest of the document. Style and margins belong to the CV package, so
// they travel as the JSON their types marshal to.
func appearanceDefaultsResult(d cv.AppearanceDefaults) (apigen.CvAppearanceDefaultsResult, error) {
	style, err := json.Marshal(d.Style)
	if err != nil {
		return apigen.CvAppearanceDefaultsResult{}, err
	}
	margins, err := json.Marshal(d.Margins)
	if err != nil {
		return apigen.CvAppearanceDefaultsResult{}, err
	}
	return apigen.CvAppearanceDefaultsResult{TemplateID: d.TemplateID, Style: style, Margins: margins}, nil
}

// GetCVAppearanceDefaults returns the caller's saved CV appearance defaults, or the system's
// standard CV defaults when they have never saved any — never an empty/absent shape.
// Cookie-only: a personal setting, not something the in-app assistant needs to touch.
func (h *cvHandlers) GetCVAppearanceDefaults(ctx context.Context) (*apigen.GetCVAppearanceDefaultsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	defaults, _, err := h.cvStore.GetAppearanceDefaults(ctx, userID)
	if err != nil {
		return nil, err
	}
	out, err := appearanceDefaultsResult(defaults)
	if err != nil {
		return nil, err
	}
	return &apigen.GetCVAppearanceDefaultsOK{Data: out}, nil
}

// SetCVAppearanceDefaults validates and persists the caller's CV appearance defaults,
// replacing any previously saved values: a save always replaces the complete set, never a
// partial patch. Cookie-only, same as the read above.
func (h *cvHandlers) SetCVAppearanceDefaults(ctx context.Context, req *apigen.CvAppearanceDefaults) (*apigen.SetCVAppearanceDefaultsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	defaults := cv.AppearanceDefaults{TemplateID: req.TemplateID.Value}
	if len(req.Style) > 0 {
		if err := json.Unmarshal(req.Style, &defaults.Style); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
	}
	if len(req.Margins) > 0 {
		if err := json.Unmarshal(req.Margins, &defaults.Margins); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
	}
	saved, err := h.cvStore.SetAppearanceDefaults(ctx, userID, defaults)
	if err != nil {
		return nil, mapCVError(err)
	}
	out, err := appearanceDefaultsResult(saved)
	if err != nil {
		return nil, err
	}
	return &apigen.SetCVAppearanceDefaultsOK{Data: out}, nil
}
