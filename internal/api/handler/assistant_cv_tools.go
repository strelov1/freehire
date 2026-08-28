package handler

import (
	"bytes"
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
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// opBatch is an edit batch as it arrives from a model: either the array the schema asks for,
// or a string holding that array. Models package it both ways, and the packaging is a guess
// about the wire format that says nothing about the edit — refusing it spends a round of the
// turn's budget and teaches the model nothing about the CV.
//
// What is inside is read by the same strict rules either way. The decoder is re-armed with
// DisallowUnknownFields here because a custom unmarshaller receives the raw bytes and the
// caller's strictness does not reach through it, and that strictness is load-bearing: an
// undefined field silently dropped is how an agent once rewrote the wrong experience entry
// while reading success back.
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
	// A second value means the string held more than the one batch — two arrays, or an array
	// and some trailing text. Applying the first and dropping the rest is the same silent
	// helpfulness the unknown-field check exists to refuse.
	if dec.More() {
		return errors.New("ops holds more than one batch")
	}
	return nil
}

// assistantCVTools are the tools a CV-tailoring session gets on top of the shared
// ones. They are bound to the session's own CV and vacancy: the ids are closed
// over here rather than taken as arguments, so the model has no way to address a
// different CV — not even by guessing an id.
func (h *assistantHandlers) assistantCVTools(cvID uuid.UUID, jobID int64, batchID uuid.UUID) []assistant.Tool {
	return []assistant.Tool{
		h.cvContextTool(jobID),
		h.cvGetTool(cvID),
		h.cvEditTool(cvID, batchID),
		h.tailorReportTool(cvID),
		h.requestConfirmationTool(),
		h.cvJobMatchTool(cvID, jobID),
		h.cvEvidenceFidelityTool(),
		h.cvPageCountTool(cvID),
	}
}

// requestConfirmationTool puts a confirmation question in front of the candidate as a
// claim plus a Yes/No choice, instead of the agent writing it as free-text prose. It has
// no side effect: the client renders the buttons from the call's own arguments, and the
// candidate's answer arrives as an ordinary chat message on their NEXT turn — clicking Yes
// replays the claim text verbatim, which is what lets the unchanged verbatim-quote
// provenance check (internal/ai/assistant/message.go's UserSaid) recognise it as the
// candidate's own words on the agent's next experience_add retry. There is no dedicated
// confirmation endpoint and no new provenance value; this tool only changes how the
// question is put, not how an answer becomes citable.
func (h *assistantHandlers) requestConfirmationTool() assistant.Tool {
	return assistant.Tool{
		Name: "request_confirmation",
		Description: "Ask the candidate to confirm a claim before it can be written into the CV, instead of " +
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
			return map[string]any{"status": "awaiting_candidate_response"}, nil
		},
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
			// A tool error is a sentence the model can act on; a nil dereference here is a
			// panic inside the SSE writer's goroutine, where Registry.Call's error path
			// cannot reach it and Fiber's recover is not listening. Production always wires
			// this, so the guard is for the next partially-wired harness.
			if h.jobs == nil {
				return nil, errors.New("the tailoring context is unavailable in this deployment")
			}
			job, err := h.jobs.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}
			base, err := h.fit.TailoringContext(ctx, userID, job)
			if err != nil {
				return nil, err
			}
			return h.withBankEvidence(ctx, userID, base), nil
		},
	}
}

// cvJobMatchTool lets the tailoring agent read the deterministic CV-vs-vacancy score as
// feedback on what it has changed. It is a different signal from cv_context's Verdict/
// OverallScore: those come from the cached LLM fit analysis and MUST NOT be recomputed
// mid-tailoring (internal/candidate/matchanalysis, cv-tailoring spec), while this recomputes fresh off
// the CV's own rendered text every call — the same read GetCVJobMatch serves the workspace
// panel, which already refreshes after every saved edit. No model call, so it costs nothing
// to check after a batch of edits.
func (h *assistantHandlers) cvJobMatchTool(cvID uuid.UUID, jobID int64) assistant.Tool {
	return assistant.Tool{
		Name: "job_match",
		Description: "Read how well the CV currently scores against this vacancy — Requirements Coverage, " +
			"Keyword Match, Job Title Match, Seniority Fit — recomputed fresh from what the CV says right now, " +
			"not the cached fit analysis. Call it after a batch of edits to see their effect; no model call.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// Same reason cv_context guards: a nil dereference here is a panic on the SSE
			// writer's goroutine, which takes the process down rather than one request.
			// This tool needs both the CV surface and the vacancy read.
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
		},
	}
}

// cvPageCountTool renders the CV exactly as the candidate would download it and reports how
// many pages Typst actually laid it out onto — the number the "keep it to one or two pages"
// instruction cannot verify on its own, since the model never sees the rendered artifact, only
// the JSON document it is writing. Call it after a batch of edits, the same way job_match
// checks the score: no write, one render.
func (h *assistantHandlers) cvPageCountTool(cvID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_page_count",
		Description: "Render the CV as it currently stands and report how many pages it actually fills. Call " +
			"this after a batch of edits that might have pushed the CV over its page target — trimming or " +
			"tightening bullets does not always shorten a page the way it looks like it would from the text " +
			"alone. No model call.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
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
		},
	}
}

// cvEvidenceFidelityTool re-surfaces the text behind a citation the agent already used, so it
// can compare its own wording against what the evidence actually says and catch a bullet that
// claims more scope, seniority, or a bigger metric than it supports. This checks no new fact:
// the agent already saw the atom's claim once, from experience_search or cv_context, before
// writing — the value here is the forced second look, not new information. Read-only: no model
// call, no write to the CV, the report, or the bank.
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

// evidencedRequirement is a requirement plus what the candidate's own bank already holds for
// it. The evidence is named, not inlined whole: the agent needs enough to decide "I can write
// this" and the id to cite, and a tool result is replayed into its context on every later turn.
type evidencedRequirement struct {
	matchanalysis.Requirement
	Evidence []requirementEvidence `json:"evidence"`
}

type requirementEvidence struct {
	ID           string `json:"id"`
	Claim        string `json:"claim"`
	CanWriteCV   bool   `json:"can_write_cv"`
	NeedsContext bool   `json:"needs_context,omitempty"`
	NeedsMetrics bool   `json:"needs_metrics,omitempty"`
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
	Job          fitanalysis.TailoringJob `json:"job"`
	Verdict      string                   `json:"verdict"`
	OverallScore int                      `json:"overall_score"`
	MissingHave  []evidencedRequirement   `json:"missing_have"`
	MissingGap   []evidencedRequirement   `json:"missing_gap"`
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
func (h *assistantHandlers) withBankEvidence(ctx context.Context, userID int64, base fitanalysis.TailoringContext) agentTailorContext {
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
				nc, nm := experience.Richness(m.Atom)
				found = append(found, requirementEvidence{
					ID:           m.Atom.ID.String(),
					Claim:        m.Atom.Claim,
					CanWriteCV:   m.Atom.Provenance.Publishable(),
					NeedsContext: nc,
					NeedsMetrics: nm,
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

// cvOpSchema mirrors cvedit.Op field by field: the kind, the address, and the payload each
// kind reads. The address vocabulary is generated from the document's own structure rather
// than restated here, so a field added to the CV becomes addressable without anyone editing
// this schema — the drift that once dropped an operation out of a model's view.
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

// cvEditTool applies a batch of edits to the tailored CV.
//
// A batch rather than one edit per call: closing one requirement usually takes several edits
// — rewrite the bullet, add the technology, adjust the summary — and a turn has a step
// ceiling. Sending them together also makes them one entry in the candidate's history, which
// is what they actually did.
//
// Two rules the tool no longer states because the editor enforces them. What the agent may
// address is a path policy, so the contact block is closed to it wherever it is reached from.
// And writing a claim requires citing banked evidence with publishable provenance: the honest
// wall made structural rather than instructional, in the service path rather than a prompt.
// One uncited edit refuses the whole batch.
func (h *assistantHandlers) cvEditTool(cvID uuid.UUID, batchID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "cv_edit",
		Description: "Edit the CV. Send every edit that belongs together in one call — they land as one " +
			"entry in the candidate's history and cost one round instead of several. Address things by " +
			"their path from cv_get. Job roles use `experience[i].…`; portfolio and side-project entries " +
			"use `projects[i].…` (name, link, bullets) — the template prints the Projects heading when " +
			"that array is non-empty, so do not invent a heading field. Each experience (and project) " +
			"holds at most " + fmt.Sprintf("%d", cv.MaxBullets) +
			" bullets; when full, " +
			"`set` an existing index or `remove` one before inserting — an insert past the cap is refused " +
			"and no existing bullet is deleted. Anything that states what the candidate did (a bullet, a " +
			"summary, a technology, a skill) needs `evidence_id` from experience_search; if the bank holds " +
			"nothing on the point, ask the candidate and record their answer with experience_add first. " +
			"Contact details are not editable here. If this batch closes a requirement from cv_context, " +
			"pass `requirement` and `requirement_status` — the report updates in this same call, so you " +
			"do not need a separate tailor_report call for it.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ops": map[string]any{
					"type":        "array",
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
					"enum":        []string{string(cv.AutopilotClosedBank), string(cv.AutopilotClosedCandidate)},
					"description": "closed_bank if the bank already had evidence; closed_candidate if the candidate just confirmed it in this conversation. Required when requirement is set.",
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
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			for i, op := range in.Ops {
				// Validate the address against the document's structure before anything is
				// applied, so a typo names itself instead of failing deep inside the batch.
				if _, err := cvedit.ParsePath(string(op.Path)); err != nil {
					return nil, fmt.Errorf("edit %d: %w", i+1, err)
				}
			}
			requirement := strings.TrimSpace(in.Requirement)
			status := cv.AutopilotStatus(in.RequirementStatus)
			if requirement != "" && status != cv.AutopilotClosedBank && status != cv.AutopilotClosedCandidate {
				return nil, fmt.Errorf("requirement_status must be %q or %q when requirement is set",
					cv.AutopilotClosedBank, cv.AutopilotClosedCandidate)
			}
			// A model names the positions it SAW: it read the document once and wrote every
			// index against that reading. Applied in sequence those addresses shift out from
			// under each other, so a batch removing two lines of one list would refuse itself.
			// The conversion happens here and not inside the editor, because the editor's other
			// callers — the whole-document save and undo — state their indices sequentially and
			// would be corrupted by it.
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
			// Merged only after Commit succeeds: a refused batch must not leave the report
			// claiming a requirement was closed by an edit that never landed. But once Commit
			// has landed, this merge is best-effort — the document already carries the edit, and
			// failing the whole tool call now would tell the model the edit never happened,
			// inviting a retry that reapplies ops Commit has no content-level dedup against.
			if requirement != "" {
				if err := h.cv.cvStore.MergeAutopilotEntry(ctx, cvID, userID, cv.AutopilotEntry{
					Requirement: requirement,
					Status:      status,
					Note:        in.Note,
				}); err != nil {
					log.Printf("assistant: recording autopilot entry for requirement %q: %v", requirement, err)
				}
			}
			// A receipt, not the document: a tool result is replayed into the model's
			// context on every later turn of the session, so echoing the CV back would be
			// paid for again and again.
			return map[string]any{"updated_at": meta.UpdatedAt, "applied": len(rev.Ops), "recorded_as": rev.Title}, nil
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

// bankGate answers the editor's evidence question from the experience bank.
//
// The rule it enforces is the one the whole tailoring capability exists to keep: a sentence
// about what the candidate did cannot reach the page unless it traces to something THEY
// asserted — on their CV, in their own words, or by typing it themselves. An agent's own
// inference is banked and searchable, and stops here.
//
// Every refusal names what to do next, because for a model that message is its only route to
// correcting itself inside the turn.
type bankGate struct{ bank experienceBankTools }

func (g bankGate) Publishable(ctx context.Context, userID int64, evidenceID string) error {
	id, err := uuid.Parse(evidenceID)
	if err != nil {
		return errors.New("evidence_id must be an achievement id from experience_search")
	}
	atom, err := g.bank.GetAtom(ctx, id, userID)
	if errors.Is(err, experience.ErrNotFound) {
		// Named, not "that id": a batch cites one per operation, so a bare refusal leaves the
		// model guessing which of them to go back for.
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
