package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
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

type baseSessionResponse struct {
	BaseCVID  string `json:"base_cv_id"`
	SessionID string `json:"session_id"`
}

// StartBaseSession returns the assistant conversation for one of the caller's BASE CVs — the one
// already bound to it, or a new one under the `base` preset (a CV and no vacancy) — so the
// tailoring workspace can edit the base with the agent. Cookie-only.
//
// It is deliberately NOT behind the paid-AI gate and spends no tailoring session: opening the
// conversation costs nothing, and a free account still opens the workspace and edits by hand.
// What the gate guards is a turn, and every message already passes mw.paidAI(FeatureAssistant),
// which answers an unpaid caller 402 — the upgrade state the chat shows in the tailoring workspace
// too. 409 for a tailored copy (its conversation is /me/cvs/:id/tailor-session).
func (h *cvHandlers) StartBaseSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rec, err := h.cvStore.BaseByID(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	sessionID, err := h.existingTailoringSession(c.Context(), userID, rec.ID)
	if err != nil {
		return err
	}
	if sessionID == "" {
		if sessionID, err = h.startCVSession(c.Context(), userID, rec.ID, assistant.PresetBase, nil); err != nil {
			return err
		}
	}
	return c.JSON(fiber.Map{"data": baseSessionResponse{BaseCVID: rec.ID.String(), SessionID: sessionID}})
}
