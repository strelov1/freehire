package handler

import (
	"context"
	"encoding/json"
	"errors"

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

// cvPatchSchema mirrors cv.Patch field by field: every field's name, type, and which op
// reads it. cv_edit used to advertise the patch as a bare object carrying one example in
// prose, and the model filled the rest in by analogy — replace_bullet arrived with the
// bullet's new TEXT in `bullet`, the index field, and no `value` at all. DecodePatch
// caught it, but a rejected call still costs a whole turn, so the shape belongs in the
// schema the model reads before it writes. additionalProperties mirrors the decoder's
// DisallowUnknownFields, and the op enum comes from the cv package so the two cannot drift.
var cvPatchSchema = map[string]any{
	"type": "object",
	"description": "One patch: an `op` plus the address and payload that op reads. Indices are 0-based, " +
		"counted over what cv_get returned. Send only the fields the op needs.",
	"properties": map[string]any{
		"op": map[string]any{
			"type":        "string",
			"enum":        cv.PatchOps,
			"description": "Which edit to make.",
		},
		"experience": map[string]any{
			"type": "integer",
			"description": "Index of the experience entry to edit, into the `experience` list cv_get returned. " +
				"Read by add_bullet, replace_bullet, remove_bullet, reorder_bullets and set_stack.",
		},
		"bullet": map[string]any{
			"type": "integer",
			"description": "Index of the bullet WITHIN that experience entry — a number, never the bullet's text. " +
				"Read by replace_bullet and remove_bullet; the new text goes in `value`.",
		},
		"field": map[string]any{
			"type": "string",
			"enum": []string{"location"},
			"description": "Which header field set_header_field writes. Only location is editable here; the " +
				"candidate's name, email and phone stay their own.",
		},
		"value": map[string]any{
			"type": "string",
			"description": "The new text: the summary (set_summary), the bullet's text (add_bullet, " +
				"replace_bullet) or the header field's value (set_header_field).",
		},
		"order": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "integer"},
			"description": "A permutation of the entry's bullet indices for reorder_bullets — the bullets end " +
				"up in the order listed here.",
		},
		"group": map[string]any{
			"type": "string",
			"description": `The skill group's NAME for set_skill_group (e.g. "Languages"), not an index. ` +
				"A name no group has appends a new group.",
		},
		"items": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "The skill group's full item list for set_skill_group; it replaces the group's current items.",
		},
		"stack": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "The experience entry's full technology line for set_stack; it replaces the current one.",
		},
	},
	"required":             []string{"op"},
	"additionalProperties": false,
}

// cvEditTool applies one field-level patch to the tailored CV.
func (h *assistantHandlers) cvEditTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_edit",
		Description: "Apply ONE field-level patch to the CV — the patch schema lists the ops and the fields " +
			"each one reads. Address experience entries and bullets by their index from cv_get. Never write a " +
			"claim the candidate has not confirmed; contact details cannot be edited here.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"patch": cvPatchSchema},
			"required":   []string{"patch"},
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
				return nil, err // already reads "cv: invalid patch: <reason>"
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
