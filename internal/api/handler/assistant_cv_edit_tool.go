package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
)

// cvOpSchema mirrors cvedit.Op field by field. The address vocabulary is generated from the
// document's own structure, so a field added to the CV becomes addressable without editing this schema.
var cvOpSchema = map[string]any{
	"type": "object",
	"description": "One edit: a kind, the address it applies to, and what that kind needs. " +
		"Indices are 0-based, counted over what cv_get returned.",
	"properties": map[string]any{
		"kind": map[string]any{
			"type": "string",
			"enum": cvedit.OpKinds,
			"description": "set replaces what is at the address. insert puts a new element at that " +
				"position (one past the end appends). remove takes the element out. move takes the " +
				"element to the position in `to`.",
		},
		"path": map[string]any{
			"type": "string",
			"description": "Where to edit, e.g. `summary`, `experience[2].bullets[1]`, " +
				"`skills[0].items[3]`, `education[1].degree`. The shapes you may address are: " +
				strings.Join(cvedit.Paths(), ", ") + ".",
		},
		"value": map[string]any{
			"description": "The new content for set and insert: a string for a bullet or a field, " +
				"an object for a whole entry.",
		},
		"to": map[string]any{
			"type":        "integer",
			"description": "The element's new position, for move.",
		},
		"evidence_id": map[string]any{
			"type": "string",
			"description": "The id of the banked achievement this rests on, from experience_search. " +
				"Required whenever the edit states something about the candidate: a summary, a bullet, " +
				"a technology in a stack line, or a skill.",
		},
	},
	"required":             []string{"kind", "path"},
	"additionalProperties": false,
}

// cvEditTool applies a batch of edits to the tailored CV. A batch, not one edit per call: closing a
// requirement usually takes several edits and a turn has a step ceiling; together they are also one
// history entry. Two rules live in the editor, not here: what the agent may address is a path policy
// (the contact block is closed to it), and writing a claim requires citing banked evidence with
// publishable provenance, in the service path rather than a prompt. One uncited edit refuses the batch.
func (h *assistantHandlers) cvEditTool(cvID uuid.UUID, batchID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_edit",
		Description: "Edit a CV document (the base CV or a tailored copy), not the profile: the headline and " +
			"summary on the user's profile page change with update_profile. Send every edit that belongs together in one call — they land as one " +
			"entry in the candidate's history and cost one round instead of several. Address things by " +
			"their path from cv_get. Job roles use `experience[i].…`; portfolio and side-project entries " +
			"use `projects[i].…` (name, link, bullets) — the template prints the Projects heading when " +
			"that array is non-empty, so do not invent a heading field. Each experience (and project) " +
			"holds at most " + fmt.Sprintf("%d", cv.MaxBullets) +
			" bullets; when full, " +
			"`set` an existing index or `remove` one before inserting — an insert past the cap is refused " +
			"and no existing bullet is deleted. Anything that states what the candidate did (a bullet, a " +
			"summary, a technology, a skill) needs `evidence_id` from experience_search — ON THE OP, " +
			"beside its kind, path and value, NOT on the call beside `note` and `requirement`; if the " +
			"bank holds nothing on the point, ask the candidate and record their answer with " +
			"experience_add first. " +
			"Contact details are not editable here, and neither is `title` — it is the CV's name in the " +
			"candidate's list, never printed; aim the CV at the role through the summary and bullets. " +
			"If this batch closes a requirement from cv_context, " +
			"pass `requirement` and `requirement_status` — the report updates in this same call, so you " +
			"do not need a separate tailor_report call for it.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ops": map[string]any{
					"type":        "array",
					"minItems":    1,
					"description": "The edits, applied in order. If any one of them is refused, none is applied.",
					"items":       cvOpSchema,
				},
				"note": map[string]any{
					"type": "string",
					"description": "One short line on why you made these edits — shown to the candidate " +
						"beside them, in your own words.",
				},
				"requirement": map[string]any{
					"type": "string",
					"description": "A requirement this batch closes, copied verbatim from cv_context. Omit " +
						"when this batch does not close a requirement (rewording, reordering, a technology " +
						"tag). Requires `requirement_status`.",
				},
				"requirement_status": map[string]any{
					"type":        "string",
					"enum":        []string{string(cv.AutopilotClosedBank), string(cv.AutopilotClosedCandidate), string(cv.AutopilotPartial)},
					"description": "closed_bank or closed_candidate only when evidence supports the FULL requirement. Use partial and unverified for related evidence that leaves a gap. Required when requirement is set.",
				},
				"unverified": map[string]any{
					"type":        "string",
					"description": "Any part of the requirement this edit does not establish, including a specific tool, duration or responsibility. A nonempty value prevents marking it closed.",
				},
			},
			"required": []string{"ops"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Ops               opBatch `json:"ops"`
				Note              string  `json:"note"`
				Requirement       string  `json:"requirement"`
				RequirementStatus string  `json:"requirement_status"`
				Unverified        string  `json:"unverified"`
				// EvidenceID is not part of this call: it belongs on the op whose claim it backs. It is read here so
				// the mistake can be answered rather than met with DecodeArgs's `unknown field "evidence_id"`, which
				// names the field and not the place (the model re-sent the identical call). The mistake is reasonable:
				// `requirement` and `requirement_status` ARE call-level, so "batch metadata goes at the top" generalises.
				EvidenceID string `json:"evidence_id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if len(in.Ops) == 0 {
				return nil, errors.New("ops must contain at least one edit; use tailor_report to record open or unsupported requirements without changing the CV")
			}
			if err := adoptCallLevelEvidence(in.Ops, in.EvidenceID); err != nil {
				return nil, err
			}
			requirement := strings.TrimSpace(in.Requirement)
			status := cv.AutopilotStatus(in.RequirementStatus)
			entry := cv.AutopilotEntry{
				Requirement: requirement, Status: status, Note: in.Note, Unverified: in.Unverified,
			}
			if requirement != "" {
				if status != cv.AutopilotClosedBank && status != cv.AutopilotClosedCandidate && status != cv.AutopilotPartial {
					return nil, errors.New("requirement_status must be closed_bank, closed_candidate or partial; use tailor_report for open requirements")
				}
				clean, err := cv.SanitizeAutopilotReport([]cv.AutopilotEntry{entry})
				if err != nil {
					return nil, err
				}
				entry = clean[0]
			}
			// A model names the positions it SAW: it read the document once and wrote every index against that
			// reading, so applied in sequence the addresses shift (a batch removing two lines of one list would
			// refuse itself). Converted here, not in the editor, whose other callers (whole-document save, undo)
			// state indices sequentially and would be corrupted.
			meta, rev, err := h.cv.editor.Commit(ctx, cvID, userID, cvedit.Change{
				Actor:   cvedit.ActorAgent,
				Origin:  cvedit.OriginTailorAgent,
				BatchID: batchID,
				Note:    in.Note,
				Ops:     cvedit.OrderAgainstOriginal(in.Ops),
			})
			if err != nil {
				return nil, cvToolError(err)
			}
			// Merged only after Commit succeeds: a refused batch must not leave the report claiming a requirement
			// was closed. After Commit it is best-effort: failing the call would tell the model the edit never
			// happened, inviting a retry that reapplies ops Commit cannot dedup.
			if requirement != "" {
				if err := h.cv.cvStore.MergeAutopilotEntry(ctx, cvID, userID, entry); err != nil {
					log.Printf("assistant: recording autopilot entry for requirement %q: %v", requirement, err)
				}
			}
			// A receipt, not the document: tool results are replayed into the context on every later turn.
			outcome := "applied"
			if len(rev.Ops) == 0 {
				outcome = "unchanged"
			}
			return map[string]any{
				"status": outcome, "updated_at": meta.UpdatedAt,
				"applied": len(rev.Ops), "recorded_as": rev.Title,
			}, nil
		},
	}
}

// cvToolError renders a CV failure for the model, keeping owner isolation: a foreign CV is reported
// as missing, never as forbidden.
func cvToolError(err error) error {
	if errors.Is(err, cv.ErrNotFound) {
		return errors.New("this tailoring session's CV is no longer available")
	}
	return err
}
