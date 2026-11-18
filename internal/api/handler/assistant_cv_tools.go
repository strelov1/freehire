package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

// opBatch is an edit batch as it arrives from a model: either the array the schema asks for, or a
// string holding that array. Models package it both ways; refusing it spends a round and teaches
// nothing about the CV. The contents are read by the same strict rules either way: the decoder is
// re-armed with DisallowUnknownFields because a custom unmarshaller does not inherit the caller's
// strictness, and a silently dropped undefined field is how an agent once rewrote the wrong entry.
// evidenceGoesOnTheOp is the refusal a misplaced evidence_id gets. It names the place: the decoder's
// `unknown field "evidence_id"` names only the field, and the model re-sends what it sent.
const evidenceGoesOnTheOp = "evidence_id goes on each op inside `ops`, not on the call itself — " +
	"put it beside the kind, path and value of the edit whose claim it backs"

// adoptCallLevelEvidence reads a call-level evidence_id onto the op it can only have meant, the same
// leniency opBatch.UnmarshalJSON extends to a batch packaged as a string. Only when exactly one op has
// no evidence of its own. Refused otherwise:
//   - several ops: spreading one achievement's evidence across claims is what the provenance gate stops;
//   - an op that already names one: which claim the id backs is unclear, and picking is inventing.
func adoptCallLevelEvidence(ops opBatch, evidenceID string) error {
	evidenceID = strings.TrimSpace(evidenceID)
	if evidenceID == "" {
		return nil
	}
	if len(ops) != 1 || ops[0].EvidenceID != "" {
		return errors.New(evidenceGoesOnTheOp)
	}
	ops[0].EvidenceID = evidenceID
	return nil
}

type opBatch []cvedit.Op

func (b *opBatch) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var packed string
		if err := json.Unmarshal(data, &packed); err != nil {
			return err
		}
		data = []byte(packed)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode((*[]cvedit.Op)(b)); err != nil {
		return err
	}
	// A second value means the string held more than one batch. Applying the first and dropping the
	// rest is the silent helpfulness the unknown-field check exists to refuse.
	if dec.More() {
		return errors.New("ops holds more than one batch")
	}
	return nil
}

// assistantCVTools is the CV tool set of a conversation bound to a CV. Bound to a vacancy too (a
// tailoring session) it adds the tools that read the CV against that vacancy: cv_context, the run
// report, job_match and the cover letter. With no vacancy (jobID nil, a base CV conversation) those are
// not registered. The CV id closes over the session binding, so the model cannot address another CV.
func (h *assistantHandlers) assistantCVTools(cvID uuid.UUID, jobID *int64, batchID uuid.UUID) []assistant.Tool {
	tools := []assistant.Tool{
		h.cvGetTool(cvID),
		h.cvEditTool(cvID, batchID),
		h.requestConfirmationTool(),
		h.cvEvidenceFidelityTool(),
		h.cvPageCountTool(cvID),
	}
	if jobID == nil {
		return tools
	}
	return append(tools,
		h.cvContextTool(*jobID),
		h.tailorReportTool(cvID),
		h.cvJobMatchTool(cvID, *jobID),
		h.coverLetterDraftTool(*jobID),
	)
}

// requestConfirmationTool puts a confirmation question in front of the candidate as a claim plus a
// Yes/No choice. No side effect: the client renders the buttons from the call's own arguments, and the
// answer arrives as an ordinary chat message on the NEXT turn. Clicking Yes replays the claim text
// verbatim, which lets the verbatim-quote provenance check (internal/ai/assistant/message.go's UserSaid)
// recognise it as the candidate's own words on the agent's next experience_add retry.
func (h *assistantHandlers) requestConfirmationTool() assistant.Tool {
	return assistant.Tool{
		Name: "request_confirmation",
		Description: "Ask the candidate to confirm an experience claim or a proposed achievement merge, instead of " +
			"asking in free text. Pass the exact claim text — the candidate sees it with Yes/No buttons, and " +
			"Yes replays that exact text as their next message, which is what makes it citable.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claim": map[string]any{
					"type":        "string",
					"description": "The exact claim text to confirm, verbatim — this is what Yes replays back.",
				},
				"question": map[string]any{
					"type":        "string",
					"description": "A short question putting the claim to the candidate.",
				},
			},
			"required":             []string{"claim", "question"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Claim    string `json:"claim"`
				Question string `json:"question"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// The claim is the whole tool: the client renders the card from it, so an empty one draws nothing and
			// the candidate is never asked; the agent then takes silence for a confirmation and its next
			// experience_add is stamped agent_inferred. Schema `required` does not stop this: DecodeArgs does not
			// enforce it and healToolArguments turns a mangled argument string into "{}". An empty question is
			// allowed on purpose: the client falls back to the claim alone.
			if strings.TrimSpace(in.Claim) == "" {
				return nil, errors.New("claim is required: pass the exact claim text the candidate must confirm, verbatim")
			}
			return map[string]any{"status": "awaiting_candidate_response"}, nil
		},
	}
}

// tailorReportTool records what a run made of each requirement, so the workspace can show the outcome
// beside the fit analysis. The whole report is written on every call, with no append: a requirement
// closed later arrives as the same list with one entry changed, so the stored value is always current.
// The result is a receipt, not the report: tool results are replayed into the model's context on every
// later turn, so echoing a long report back would be paid for again and again.
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
								"description": "closed_bank: bank evidence fully supports this entire requirement, including named tools and duration. " +
									"closed_candidate: the candidate confirmed the entire requirement and their words were banked. " +
									"partial: related evidence exists, but some part remains unverified; name it in unverified. " +
									"open: the bank holds nothing — this is what to ask about. " +
									"not_reached: the run ended before you got to it.",
							},
							"note": map[string]any{
								"type":        "string",
								"description": "One short line: what you changed, or why it is still open.",
							},
							"unverified": map[string]any{
								"type":        "string",
								"description": "What evidence is still missing for the FULL requirement: named technology, duration, scope, or responsibility. Empty only when no part remains unverified. Related experience is not proof of a specific tool or duration.",
							},
						},
						"required":             []string{"requirement", "status", "unverified"},
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
			clean, err := cv.SanitizeAutopilotReport(in.Items)
			if err != nil {
				return nil, err
			}
			if err := h.cv.cvStore.SetAutopilotReport(ctx, cvID, userID, clean); err != nil {
				return nil, cvToolError(err)
			}
			counts := map[string]int{"saved": len(clean), "closed": 0, "partial": 0, "open": 0, "not_reached": 0}
			for _, entry := range clean {
				if entry.Status == cv.AutopilotClosedBank || entry.Status == cv.AutopilotClosedCandidate {
					counts["closed"]++
				} else {
					counts[string(entry.Status)]++
				}
			}
			return counts, nil
		},
	}
}

// cvContextTool serves the reasoning context for the tailoring: the cached fit analysis split into
// requirements the candidate can evidence (reframe them) and genuine gaps (ask before writing).
func (h *assistantHandlers) cvContextTool(jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "cv_context",
		Description: "Read the fit analysis for the vacancy this CV is being tailored to: the vacancy's " +
			"requirements split into missing_have (the candidate has the evidence, the CV omits it — reframe an " +
			"existing bullet) and missing_gap (a real gap — ASK the candidate before writing anything). " +
			"job.requirements is a separate, fuller list: everything the posting itself asks for, in its own " +
			"words, including what the CV already covers — those are worth keeping prominent, not editing away.",
		Schema: assistant.EmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			// A tool error is a sentence the model can act on; a nil dereference would panic inside the SSE
			// writer's goroutine, out of Registry.Call's error path and Fiber's recover. The guard is for a
			// partially-wired harness.
			if h.jobs == nil {
				return nil, errors.New("the tailoring context is unavailable in this deployment")
			}
			job, err := h.jobs.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}
			base, err := h.fit.TailoringContext(ctx, userID, job, postingRequirements(job))
			if err != nil {
				return nil, err
			}
			return h.withBankEvidence(ctx, userID, base), nil
		}),
	}
}

// cvJobMatchTool lets the tailoring agent read the deterministic CV-vs-vacancy score as feedback on
// what it has changed. Unlike cv_context's Verdict/OverallScore (cached LLM fit analysis, which MUST NOT
// be recomputed mid-tailoring), this recomputes off the CV's rendered text every call, the same read
// GetCVJobMatch serves the workspace panel. No model call, so it is free to check after a batch.
func (h *assistantHandlers) cvJobMatchTool(cvID uuid.UUID, jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "job_match",
		Description: "Read how well the CV currently scores against this vacancy — Requirements Coverage, " +
			"Keyword Match, Job Title Match, Seniority Fit — recomputed fresh from what the CV says right now, " +
			"not the cached fit analysis. Call it after a batch of edits to see their effect; no model call.",
		Schema: assistant.EmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			// Same reason cv_context guards: a nil dereference is a panic on the SSE writer's goroutine, which
			// takes the process down. Needs both the CV surface and the vacancy read.
			if h.cv == nil || h.jobs == nil {
				return nil, errors.New("the job-match score is unavailable in this deployment")
			}
			rec, err := h.cv.cvStore.GetForModel(ctx, cvID, userID)
			if err != nil {
				return nil, cvToolError(err)
			}
			tmpl, err := cv.ResolveTemplate(rec.TemplateID)
			if err != nil {
				return nil, cvToolError(err)
			}
			job, err := h.jobs.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}
			analysis, hasAnalysis := h.fit.Optional(ctx, userID, jobID)
			score, err := h.cv.cvJobMatchScore(ctx, rec.Document, tmpl, cvmatch.Input{
				JobTitle:     job.Title,
				JobSkills:    job.Skills,
				Requirements: cvJobMatchRequirements(analysis),
				HasAnalysis:  hasAnalysis,
			})
			if err != nil {
				return map[string]any{"available": false, "reason": "could not compute a score right now"}, nil
			}
			if len(score.Contributing) == 0 {
				return map[string]any{"available": false, "reason": "nothing about this vacancy could be matched automatically"}, nil
			}
			return map[string]any{"available": true, "score": score}, nil
		}),
	}
}

// cvPageCountTool renders the CV as the candidate would download it and reports how many pages Typst
// laid it out onto: the model never sees the rendered artifact, only the JSON it is writing, so it
// cannot verify "keep it to one or two pages" itself. No write, one render.
func (h *assistantHandlers) cvPageCountTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_page_count",
		Description: "Render the CV as it currently stands and report how many pages it actually fills. Call " +
			"this after a batch of edits that might have pushed the CV over its page target — trimming or " +
			"tightening bullets does not always shorten a page the way it looks like it would from the text " +
			"alone. No model call.",
		Schema: assistant.EmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			rec, err := h.cv.cvStore.GetForModel(ctx, cvID, userID)
			if err != nil {
				return nil, cvToolError(err)
			}
			tmpl, err := cv.ResolveTemplate(rec.TemplateID)
			if err != nil {
				return nil, cvToolError(err)
			}
			pages, err := h.cv.renderedCVPageCount(ctx, rec.Document, tmpl)
			if err != nil {
				return map[string]any{"available": false, "reason": "could not render the CV right now"}, nil
			}
			return map[string]any{"available": true, "pages": pages}, nil
		}),
	}
}

// cvEvidenceFidelityTool re-surfaces the text behind a citation the agent already used, so it can
// compare its wording against the evidence and catch a bullet claiming more scope, seniority or a
// bigger metric than it supports. The value is the forced second look, not new information.
// Read-only: no model call, no write to the CV, the report, or the bank.
func (h *assistantHandlers) cvEvidenceFidelityTool() assistant.Tool {
	return assistant.Tool{
		Name: "check_evidence_fidelity",
		Description: "Re-read the evidence behind an evidence_id you just cited in cv_edit, so you can compare " +
			"your own wording against it. Call this after writing a bullet, summary, technology, or skill that " +
			"cites evidence, and revise with cv_edit if what you wrote claims more scope, seniority, or a bigger " +
			"metric than this evidence actually supports. No model call, and nothing about the CV changes.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"evidence_id": map[string]any{
					"type":        "string",
					"description": "The evidence_id you cited in the cv_edit you want to double-check.",
				},
			},
			"required":             []string{"evidence_id"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				EvidenceID string `json:"evidence_id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			id, err := uuid.Parse(strings.TrimSpace(in.EvidenceID))
			if err != nil {
				return nil, errors.New("evidence_id must be an achievement id from experience_search or cv_context")
			}
			atom, err := h.experience.GetAtom(ctx, id, userID)
			if errors.Is(err, experience.ErrNotFound) {
				return nil, fmt.Errorf("no banked achievement with id %s — check the evidence_id you cited", id)
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"claim":   atom.Claim,
				"context": atom.Context,
				"metrics": atom.Metrics,
			}, nil
		},
	}
}

// result goes into a model's context, and that model reads attacker-controlled text in the same turn.
func (h *assistantHandlers) cvGetTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name:        "cv_get",
		Description: "Read the current CV document being tailored, so edits are grounded in what it actually says.",
		Schema:      assistant.EmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			rec, err := h.cv.cvStore.GetForModel(ctx, cvID, userID)
			if err != nil {
				return nil, cvToolError(err)
			}
			return map[string]any{"title": rec.Title, "template_id": rec.TemplateID, "document": rec.Document}, nil
		}),
	}
}

// bankGate answers the editor's evidence question from the experience bank. A sentence about what the
// candidate did cannot reach the page unless it traces to something THEY asserted (their CV, their own
// words, or typed themselves); an agent's inference is banked and searchable, and stops here. Every
// refusal names what to do next, since that message is the model's only route to self-correction.
type bankGate struct{ bank experienceBankTools }

func (g bankGate) Publishable(ctx context.Context, userID int64, evidenceID string) error {
	id, err := uuid.Parse(evidenceID)
	if err != nil {
		return errors.New("evidence_id must be an achievement id from experience_search")
	}
	atom, err := g.bank.GetAtom(ctx, id, userID)
	if errors.Is(err, experience.ErrNotFound) {
		// Named, not "that id": a batch cites one per operation, so a bare refusal leaves the model guessing.
		return fmt.Errorf("no banked achievement with id %s — take evidence_id from experience_search", id)
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
