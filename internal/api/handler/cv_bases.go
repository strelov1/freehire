package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/apigen"
)

// ListBaseCVs returns the caller's base (non-tailored) CVs, oldest created first, flagging the
// default. A separate read from GET /me/cvs, whose items describe tailored copies and are
// consumed by the CLI and the legacy redirect. Cookie or API key, like the other CV reads.
//
// IsDefault marks the base used wherever a single base is needed without asking: the one the
// newest tailored copy came from, else the oldest base. The picker preselects it. HandEdited
// records that the candidate changed this base in the editor; the automatic refresh after a
// new CV upload leaves such a base alone.
func (h *cvHandlers) ListBaseCVs(ctx context.Context) (*apigen.ListBaseCVsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.cvStore.Bases(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CvBase, len(items))
	for i, it := range items {
		meta := cvMetaOf(it.Meta)
		out[i] = apigen.CvBase{
			ID: meta.ID, Title: meta.Title, TemplateID: meta.TemplateID, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
			IsDefault: it.IsDefault, HandEdited: it.HandEdited,
		}
	}
	return &apigen.ListBaseCVsOK{Data: out}, nil
}

// DuplicateCV copies one of the caller's base CVs — the stored document, template and
// appearance — as "<title> (copy)" and returns the new record so the client can open it.
// Only a base may be duplicated (409 for a tailored copy); a foreign or missing id is a 404.
// Cookie-only, like every CV mutation.
func (h *cvHandlers) DuplicateCV(ctx context.Context, params apigen.DuplicateCVParams) (*apigen.DuplicateCVCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	src, err := h.cvStore.BaseByID(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	meta, err := h.cvStore.Duplicate(ctx, id, userID, cvTitle(src.Title+" (copy)"))
	if err != nil {
		return nil, mapCVError(err)
	}
	rec, err := h.cvStore.Get(ctx, meta.ID, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	out, err := cvRecordOf(rec)
	if err != nil {
		return nil, err
	}
	return &apigen.DuplicateCVCreated{Data: out}, nil
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
func (h *cvHandlers) StartBaseSession(ctx context.Context, params apigen.StartBaseSessionParams) (*apigen.StartBaseSessionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	rec, err := h.cvStore.BaseByID(ctx, id, userID)
	if err != nil {
		return nil, mapCVError(err)
	}
	sessionID, err := h.existingTailoringSession(ctx, userID, rec.ID)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		if sessionID, err = h.startCVSession(ctx, userID, rec.ID, assistant.PresetBase, nil); err != nil {
			return nil, err
		}
	}
	return &apigen.StartBaseSessionOK{Data: apigen.CvBaseSession{BaseCvID: rec.ID.String(), SessionID: sessionID}}, nil
}
