package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
)

// cvAppearanceDefaultsResponse is the wire shape for a user's saved (or system-fallback) CV
// appearance defaults — the same template/typography/margins shape a CV document itself
// carries, without the rest of the document.
type cvAppearanceDefaultsResponse struct {
	TemplateID string     `json:"template_id"`
	Style      cv.Style   `json:"style"`
	Margins    cv.Margins `json:"margins"`
}

func appearanceDefaultsResponse(d cv.AppearanceDefaults) cvAppearanceDefaultsResponse {
	return cvAppearanceDefaultsResponse{TemplateID: d.TemplateID, Style: d.Style, Margins: d.Margins}
}

// GetCVAppearanceDefaults returns the caller's saved CV appearance defaults, or the system's
// standard CV defaults when they have never saved any — never an empty/absent shape.
// Cookie-only: a personal setting, not something the in-app assistant needs to touch.
func (h *cvHandlers) GetCVAppearanceDefaults(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	defaults, _, err := h.cvStore.GetAppearanceDefaults(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": appearanceDefaultsResponse(defaults)})
}

// setCVAppearanceDefaultsRequest is the same shape as the response: a save always replaces
// the complete set, never a partial patch.
type setCVAppearanceDefaultsRequest struct {
	TemplateID string     `json:"template_id"`
	Style      cv.Style   `json:"style"`
	Margins    cv.Margins `json:"margins"`
}

// SetCVAppearanceDefaults validates and persists the caller's CV appearance defaults,
// replacing any previously saved values. Cookie-only, same as the read above.
func (h *cvHandlers) SetCVAppearanceDefaults(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in setCVAppearanceDefaultsRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	defaults := cv.AppearanceDefaults{TemplateID: in.TemplateID, Style: in.Style, Margins: in.Margins}
	saved, err := h.cvStore.SetAppearanceDefaults(c.Context(), userID, defaults)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": appearanceDefaultsResponse(saved)})
}
