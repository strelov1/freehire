package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/matchanalysis"
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
		h.tailorReportTool(cvID),
	}
}

// tailorReportTool records what a run made of each requirement, so the workspace can show
// the outcome beside the fit analysis and the candidate can see what is left.
//
// The whole report is written on every call. There is no append: a requirement closed later
// from the candidate's own words arrives as the same list with one entry changed, which
// keeps one write path instead of two and makes the stored value always the current truth.
//
// The result is a receipt, not the report. A tool result is persisted in the transcript and
// replayed into the model's context on EVERY later turn of the session, so echoing a
// forty-line report back would be paid for again and again for nothing.
func (h *assistantHandlers) tailorReportTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "tailor_report",
		Description: "Record what you made of EVERY requirement you considered, as one list — this replaces " +
			"the previous report rather than adding to it. Call it once at the end of an unattended run, " +
			"before you write your summary, and again later whenever a requirement's outcome changes " +
			"(the candidate confirms experience you then write into the CV).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":        "array",
					"description": "One entry per requirement, in the order they appear in the fit analysis.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"requirement": map[string]any{
								"type":        "string",
								"description": "The requirement, copied verbatim from cv_context.",
							},
							"status": map[string]any{
								"type": "string",
								"enum": cv.AutopilotStatuses,
								"description": "closed_bank: the experience bank had evidence and you rewrote the CV around it. " +
									"closed_candidate: the candidate confirmed it in conversation, you banked their words and cited them. " +
									"open: the bank holds nothing — this is what to ask about. " +
									"not_reached: the run ended before you got to it.",
							},
							"note": map[string]any{
								"type":        "string",
								"description": "One short line: what you changed, or why it is still open.",
							},
						},
						"required":             []string{"requirement", "status"},
						"additionalProperties": false,
					},
				},
			},
			"required": []string{"items"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Items []cv.AutopilotEntry `json:"items"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := h.cv.cvStore.SetAutopilotReport(ctx, cvID, userID, in.Items); err != nil {
				return nil, cvToolError(err)
			}
			return map[string]any{"saved": len(in.Items)}, nil
		},
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
			base, err := h.cv.tailoringContext(ctx, userID, jobID)
			if err != nil {
				return nil, err
			}
			return h.withBankEvidence(ctx, userID, base), nil
		},
	}
}

// evidencedRequirement is a requirement plus what the candidate's own bank already holds for
// it. The evidence is named, not inlined whole: the agent needs enough to decide "I can write
// this" and the id to cite, and a tool result is replayed into its context on every later turn.
type evidencedRequirement struct {
	matchanalysis.Requirement
	Evidence []requirementEvidence `json:"evidence"`
}

type requirementEvidence struct {
	ID         string `json:"id"`
	Claim      string `json:"claim"`
	CanWriteCV bool   `json:"can_write_cv"`
}

// agentTailorContext is the tailoring context as the AGENT reads it — deliberately narrower
// than what the endpoint serves the page.
//
// Measured on a recorded session, the served context was 11.4 KB: 4.2 KB of posting, 2.5 KB of
// requirements, and 3 KB of dimension comments, strengths, gaps and recommendation. That last
// 3 KB is the part the agent must NOT act on — it cannot edit a CV from a dimension comment,
// and restating the verdict is exactly what it is told not to do. It is on the candidate's
// screen already, in the panel beside the chat.
//
// So the agent gets what it works from: the vacancy, the score in one line for tone, and the
// requirements with the bank's answer attached.
type agentTailorContext struct {
	Job          tailorJob              `json:"job"`
	Verdict      string                 `json:"verdict"`
	OverallScore int                    `json:"overall_score"`
	MissingHave  []evidencedRequirement `json:"missing_have"`
	MissingGap   []evidencedRequirement `json:"missing_gap"`
}

// evidencePerRequirement bounds what one requirement may carry. Three is enough to write a
// bullet from and to see that the bank is not empty; the rest is one experience_search away.
const evidencePerRequirement = 3

// withBankEvidence answers, for every requirement, the question the agent would otherwise
// spend a tool round on: does the bank hold anything for this?
//
// Retrieval is a linear scan over the caller's own atoms with no model call in it, so asking it
// once per requirement here costs less than the single round it replaces. A recorded session
// made TEN searches and never reached an edit; this is what those searches were asking for.
//
// A bank that cannot be read degrades to no evidence rather than to an error: the requirements
// are worth reading either way, and an empty list already means "nothing found".
func (h *assistantHandlers) withBankEvidence(ctx context.Context, userID int64, base tailorContextResponse) agentTailorContext {
	return agentTailorContext{
		Job:          base.Job,
		Verdict:      base.Verdict,
		OverallScore: base.OverallScore,
		MissingHave:  h.evidenceFor(ctx, userID, base.MissingHave),
		MissingGap:   h.evidenceFor(ctx, userID, base.MissingGap),
	}
}

func (h *assistantHandlers) evidenceFor(ctx context.Context, userID int64, reqs []matchanalysis.Requirement) []evidencedRequirement {
	out := make([]evidencedRequirement, 0, len(reqs))
	for _, r := range reqs {
		// Always a list, never nil: "looked and found nothing" is the answer that decides
		// whether to ask the candidate, and an absent field reads as "did not look".
		found := []requirementEvidence{}
		if h.experience != nil {
			matches, err := h.experience.Retrieve(ctx, userID,
				experience.Query{Text: r.Text}, evidencePerRequirement)
			if err != nil {
				log.Printf("assistant: bank retrieval for %q: %v", r.Text, err)
			}
			for _, m := range matches {
				found = append(found, requirementEvidence{
					ID:         m.Atom.ID.String(),
					Claim:      m.Atom.Claim,
					CanWriteCV: m.Atom.Provenance.Publishable(),
				})
			}
		}
		out = append(out, evidencedRequirement{Requirement: r, Evidence: found})
	}
	return out
}

// cvGetTool reads the tailored CV document the session is editing, minus the contact
// block: this result goes straight into a model's context, and that model reads
// attacker-controlled text elsewhere in the same turn.
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
			rec, err := h.cv.cvStore.GetForModel(ctx, cvID, userID)
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

// bulletWritingOps are the patch ops that put a CLAIM about the candidate on the page.
// They are the ones that require evidence; reordering, removing, and editing the
// technology line rearrange or delete what is already there and assert nothing new.
var bulletWritingOps = map[cv.PatchOp]bool{
	cv.PatchAddBullet:     true,
	cv.PatchReplaceBullet: true,
}

// cvEditTool applies one field-level patch to the tailored CV.
//
// Writing a bullet requires naming the banked evidence behind it. That is the honest wall
// made structural rather than instructional: a sentence about what the candidate did
// cannot reach the page unless it traces to something they asserted — on their CV, in
// their own words, or by typing it themselves. An agent's own inference is banked and
// searchable, and stops here.
func (h *assistantHandlers) cvEditTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_edit",
		Description: "Apply ONE field-level patch to the CV — the patch schema lists the ops and the fields " +
			"each one reads. Address experience entries and bullets by their index from cv_get. Writing a " +
			"bullet (add_bullet, replace_bullet) requires `evidence_id`: the id of the banked achievement it " +
			"is based on, from experience_search. If nothing in the bank backs it, ask the candidate and " +
			"record their answer with experience_add first. Contact details cannot be edited here.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": cvPatchSchema,
				"evidence_id": map[string]any{
					"type": "string",
					"description": "The id of the banked achievement this bullet is based on, from " +
						"experience_search. Required for add_bullet and replace_bullet.",
				},
			},
			"required": []string{"patch"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Patch      json.RawMessage `json:"patch"`
				EvidenceID string          `json:"evidence_id"`
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
			if err := h.requireEvidence(ctx, userID, p.Op, in.EvidenceID); err != nil {
				return nil, err
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

// requireEvidence enforces that a bullet traces to something the candidate asserted.
//
// The check runs in the service path, not in the system prompt, because a rule that lives
// only in a prompt is a rule a long conversation eventually loses — and this is the one
// rule the whole capability exists to keep. Every failure names what to do next, because
// that message is the model's only route to correcting itself inside the turn.
func (h *assistantHandlers) requireEvidence(ctx context.Context, userID int64, op cv.PatchOp, evidenceID string) error {
	if !bulletWritingOps[op] {
		return nil
	}
	if h.experience == nil {
		return nil
	}

	evidenceID = strings.TrimSpace(evidenceID)
	if evidenceID == "" {
		return errors.New("writing a bullet needs evidence_id — the id of the banked achievement it " +
			"is based on. Find it with experience_search; if the bank holds nothing on this point, ask " +
			"the candidate and record their answer with experience_add first")
	}
	id, err := uuid.Parse(evidenceID)
	if err != nil {
		return errors.New("evidence_id must be an achievement id from experience_search")
	}

	atom, err := h.experience.GetAtom(ctx, id, userID)
	if errors.Is(err, experience.ErrNotFound) {
		return errors.New("no banked achievement with that id — take evidence_id from experience_search")
	}
	if err != nil {
		return err
	}
	if !atom.Provenance.Publishable() {
		return errors.New("that achievement is recorded as your own reading rather than the candidate's " +
			"statement, so it cannot go on their CV. Ask them to confirm it, then record what they say " +
			"with experience_add and use the new id")
	}
	return nil
}
