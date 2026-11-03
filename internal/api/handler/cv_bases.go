package handler

import (
	"github.com/gofiber/fiber/v2"
)

// cvBaseResponse is a base CV in the bases list and the tailoring picker.
type cvBaseResponse struct {
	cvMetaResponse
	// IsDefault marks the base used wherever a single base is needed without asking: the one the
	// newest tailored copy came from, else the oldest base. The picker preselects it.
	IsDefault bool `json:"is_default"`
	// HandEdited records that the candidate changed this base in the editor; the automatic
	// refresh after a new CV upload leaves such a base alone.
	HandEdited bool `json:"hand_edited"`
}

// ListBaseCVs returns the caller's base (non-tailored) CVs, oldest created first, flagging the
// default. A separate read from GET /me/cvs, whose items describe tailored copies and are
// consumed by the CLI and the legacy redirect. Cookie or API key, like the other CV reads.
func (h *cvHandlers) ListBaseCVs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.cvStore.Bases(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]cvBaseResponse, len(items))
	for i, it := range items {
		out[i] = cvBaseResponse{cvMetaResponse: metaResponse(it.Meta), IsDefault: it.IsDefault, HandEdited: it.HandEdited}
	}
	return c.JSON(fiber.Map{"data": out})
}

// DuplicateCV copies one of the caller's base CVs — the stored document, template and
// appearance — as "<title> (copy)" and returns the new record so the client can open it.
// Only a base may be duplicated (409 for a tailored copy); a foreign or missing id is a 404.
// Cookie-only, like every CV mutation.
func (h *cvHandlers) DuplicateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	src, err := h.cvStore.BaseByID(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	meta, err := h.cvStore.Duplicate(c.Context(), id, userID, cvTitle(src.Title+" (copy)"))
	if err != nil {
		return mapCVError(err)
	}
	rec, err := h.cvStore.Get(c.Context(), meta.ID, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": recordResponse(rec)})
}
