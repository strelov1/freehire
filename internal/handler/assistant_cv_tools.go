package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/cv"
)

// assistantCVTools are the tools a CV-tailoring session gets on top of the shared
// ones. They are bound to the session's own CV and vacancy: the ids are closed
// over here rather than taken as arguments, so the model has no way to address a
// different CV — not even by guessing an id.
func (h *assistantHandlers) assistantCVTools(cvID uuid.UUID, jobID int64) []assistant.Tool {
	return []assistant.Tool{
		h.cvContextTool(jobID),
		h.cvGetTool(cvID),
		h.cvEditTool(cvID),
	}
}

// cvContextTool serves the reasoning context for the tailoring: the cached fit
// analysis split into requirements the candidate can evidence (reframe them) and
// genuine gaps (ask before writing anything).
func (h *assistantHandlers) cvContextTool(jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "cv_context",
		Description: "Read the fit analysis for the vacancy this CV is being tailored to: the vacancy's " +
			"requirements split into missing_have (the candidate has the evidence, the CV omits it — reframe an " +
			"existing bullet) and missing_gap (a real gap — ASK the candidate before writing anything).",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			analysis, err := h.cv.cachedAnalysisCtx(ctx, userID, jobID)
			if err != nil {
				return nil, err
			}
			job, err := h.queries.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}
			return tailorContext(analysis, job), nil
		},
	}
}

// cvGetTool reads the tailored CV document the session is editing.
func (h *assistantHandlers) cvGetTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name:        "cv_get",
		Description: "Read the current CV document being tailored, so edits are grounded in what it actually says.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
			if err != nil {
				return nil, cvToolError(err)
			}
			return map[string]any{"title": rec.Title, "template_id": rec.TemplateID, "document": rec.Document}, nil
		},
	}
}

// cvEditTool applies one field-level patch to the tailored CV.
func (h *assistantHandlers) cvEditTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_edit",
		Description: "Apply ONE field-level patch to the CV. Ops: set_summary, set_header_field, add_bullet, " +
			"replace_bullet, remove_bullet, reorder_bullets, set_skill_group. Never write a claim the candidate " +
			"has not confirmed; contact details cannot be edited here.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type": "object",
					"description": "One cv.Patch object: an `op` plus its address and payload, e.g. " +
						`{"op":"add_bullet","experience":0,"value":"Cut p99 latency 40%"}.`,
				},
			},
			"required": []string{"patch"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Patch json.RawMessage `json:"patch"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// Decode strictly, exactly as the HTTP endpoint does: a stray field or a
			// numeric where a string belongs must fail with a reason rather than
			// silently editing the wrong part of the document.
			p, err := cv.DecodePatch(in.Patch)
			if err != nil {
				return nil, fmt.Errorf("invalid patch: %w", err)
			}
			// The tailoring agent never sees the contact block and must not be able to
			// write it either, so the stored identifiers stay the candidate's own.
			if p.Op == cv.PatchSetHeaderField && isContactHeaderField(p.Field) {
				return nil, errors.New("contact fields are not editable in a tailoring session")
			}
			meta, err := h.cv.cvStore.Patch(ctx, cvID, userID, p)
			if err != nil {
				return nil, cvToolError(err)
			}
			return map[string]any{"updated_at": meta.UpdatedAt, "title": meta.Title}, nil
		},
	}
}

// cvToolError renders a CV failure for the model, keeping owner isolation intact:
// a foreign CV is reported as missing, never as forbidden.
func cvToolError(err error) error {
	if errors.Is(err, cv.ErrNotFound) {
		return errors.New("this tailoring session's CV is no longer available")
	}
	return err
}
