package handler

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// The experience-bank and tailored-CV tools of the signed-in MCP server: the surface the npm
// freehire-mcp package reaches over REST (/me/experience, /me/cvs). Each runs the contract
// handler behind that route, under auth.AgentContext, so an MCP agent meets exactly what a
// keyed REST caller meets: a redacted CV read, edits as the agent behind the evidence wall,
// provenance it cannot launder, no cascading place delete. cv_tailor also passes the paid-AI
// gate its route sits behind. Served to grants and full-scope keys alike, as the routes are.

var (
	cvIDProperty          = map[string]any{"type": "string", "description": "The tailored CV's id, as the CV list or a tailoring start returns it. Opaque."}
	employmentIDProperty  = map[string]any{"type": "string", "description": "The place's id, as the experience bank returns it. Opaque."}
	achievementIDProperty = map[string]any{"type": "string", "description": "The achievement's id, as the experience bank returns it; the same id a CV edit cites as evidence_id."}
	stringList            = func(description string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
	}
	employmentProperties = map[string]any{
		"kind":     map[string]any{"type": "string", "enum": []string{"job", "project"}, "description": "job or project; a new place defaults to job."},
		"company":  map[string]any{"type": "string", "description": "The company, or the project's name."},
		"role":     map[string]any{"type": "string", "description": "The candidate's title or role there."},
		"location": map[string]any{"type": "string", "description": "Free-text location."},
		"start":    map[string]any{"type": "string", "description": "When it began, e.g. \"Mar 2021\" or \"2021-03\"."},
		"end":      map[string]any{"type": "string", "description": "When it ended, same format; leave out while ongoing."},
		"current":  map[string]any{"type": "boolean", "description": "True while the candidate is still there."},
		"summary":  map[string]any{"type": "string", "description": "One line about the place."},
	}
)

// employmentArgs and atomArgs are what a bank write accepts; pointers so an update can tell
// a field left out from one cleared.
type employmentArgs struct {
	Kind     *string `json:"kind,omitempty"`
	Company  *string `json:"company,omitempty"`
	Role     *string `json:"role,omitempty"`
	Location *string `json:"location,omitempty"`
	Start    *string `json:"start,omitempty"`
	End      *string `json:"end,omitempty"`
	Current  *bool   `json:"current,omitempty"`
	Summary  *string `json:"summary,omitempty"`
}

type atomArgs struct {
	Claim        *string   `json:"claim,omitempty"`
	Context      *string   `json:"context,omitempty"`
	Metrics      *[]string `json:"metrics,omitempty"`
	Skills       *[]string `json:"skills,omitempty"`
	EmploymentID *string   `json:"employment_id,omitempty"`
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func withID(id map[string]any, rest map[string]any) map[string]any {
	out := maps.Clone(rest)
	out["id"] = id
	return out
}

// contractToolError renders a contract handler's error as REST would (classify), so a
// pgx.ErrNoRows or a missing fit analysis reaches the agent as its 404 or 409 sentence rather
// than as an internal fault.
func contractToolError(err error) error {
	if err == nil {
		return nil
	}
	if status, msg, report := classify(err); !report && status != statusClientClosedRequest {
		return fiber.NewError(status, msg)
	}
	return err
}

// okData is the `data` of a contract response, as the route would have sent it.
func okData(resp json.Marshaler, err error) (any, error) {
	if err != nil {
		return nil, contractToolError(err)
	}
	raw, err := resp.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

// overlay is the stored entry with the caller's fields laid over it: the routes REPLACE the
// whole row, so a partial correction must not blank what it did not name.
func overlay(current any, changes any) (jx.Raw, error) {
	base, err := jsonObject(current)
	if err != nil {
		return nil, err
	}
	patch, err := jsonObject(changes)
	if err != nil {
		return nil, err
	}
	if _, ok := patch["company"]; ok {
		delete(base, "name") // a project's label is served as name and wins over company on the way back in
	}
	maps.Copy(base, patch)
	return json.Marshal(base)
}

// jsonObject is v as the JSON object it marshals to.
func jsonObject(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(raw, &out)
}

func (h *assistantHandlers) cvMCPTools(origin string) []assistant.Tool {
	return append(h.experienceMCPTools(), h.tailoredCVMCPTools(origin)...)
}

func (h *assistantHandlers) experienceMCPTools() []assistant.Tool {
	return []assistant.Tool{
		{
			Name: "experience_list", Title: "Read my experience bank", Effect: assistant.EffectReads, Idempotent: true,
			Description: "The candidate's whole experience bank: every place (job or project) with its achievements, and " +
				"achievements attached to no place under `unplaced`. Each achievement has a provenance: cv_import, " +
				"stated_in_chat and manual are the candidate's own assertions and may be cited on a CV; agent_inferred " +
				"is a model's reading and may not.",
			Schema: objectSchema(map[string]any{}),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				if err := assistant.DecodeArgs(raw, &struct{}{}); err != nil {
					return nil, err
				}
				return okData(h.experienceH.ListExperience(auth.AgentContext(ctx, userID)))
			},
		},
		{
			Name: "experience_add_employment", Title: "Add a place to my experience bank", Effect: assistant.EffectAdds,
			Description: "Records a place where the candidate's work happened, a job or a side project; needs a company " +
				"or a role. Returns the stored place with its id.",
			Schema: objectSchema(employmentProperties),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in employmentArgs
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				if in.Kind == nil {
					in.Kind = new(experience.KindJob) // the bank refuses a place with no kind
				}
				body, err := json.Marshal(in)
				if err != nil {
					return nil, err
				}
				resp, err := h.experienceH.AddEmployment(auth.AgentContext(ctx, userID), body)
				if err != nil {
					return nil, contractToolError(err)
				}
				return json.RawMessage(resp.Data), nil
			},
		},
		{
			Name: "experience_add_achievement", Title: "Add an achievement to my experience bank", Effect: assistant.EffectAdds,
			Description: "Records one achievement the CANDIDATE stated, as the sentence a CV bullet would carry. It is " +
				"stored as their own assertion (provenance manual), which is what lets a CV cite it, so it must be their " +
				"words, never an inference. A claim already in the bank, under any spelling, is refused (409) rather than " +
				"duplicated. Returns the stored achievement with its id.",
			Schema: objectSchema(map[string]any{
				"claim":         map[string]any{"type": "string", "description": "The achievement as one CV-bullet-grade sentence."},
				"context":       map[string]any{"type": "string", "description": "How it was done, in a sentence or two."},
				"metrics":       stringList("Numbers as stated, e.g. \"20s -> 1s\"."),
				"skills":        stringList("Canonical skill slugs, e.g. go, kubernetes."),
				"employment_id": employmentIDProperty,
			}, "claim"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in atomArgs
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				body, err := json.Marshal(in)
				if err != nil {
					return nil, err
				}
				resp, err := h.experienceH.AddAtom(auth.AgentContext(ctx, userID), body)
				if err != nil {
					return nil, contractToolError(err)
				}
				return json.RawMessage(resp.Data), nil
			},
		},
		{
			Name: "experience_update_employment", Title: "Correct a place in my experience bank", Effect: assistant.EffectOverwrites, Idempotent: true,
			Description: "Corrects a place already in the bank: only the fields passed change, the rest stay as stored. " +
				"Returns the updated place.",
			Schema: objectSchema(withID(employmentIDProperty, employmentProperties), "id"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID string `json:"id"`
					employmentArgs
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				ctx = auth.AgentContext(ctx, userID)
				employments, err := h.experienceH.bank.ListEmployments(ctx, userID)
				if err != nil {
					return nil, err
				}
				i := slices.IndexFunc(employments, func(e experience.Employment) bool { return e.ID.String() == strings.TrimSpace(in.ID) })
				if i < 0 {
					return nil, fiber.NewError(fiber.StatusNotFound, "no such place in the experience bank")
				}
				body, err := overlay(employments[i], in.employmentArgs)
				if err != nil {
					return nil, err
				}
				resp, err := h.experienceH.UpdateEmployment(ctx, body, apigen.UpdateEmploymentParams{ID: employments[i].ID.String()})
				if err != nil {
					return nil, contractToolError(err)
				}
				return json.RawMessage(resp.Data), nil
			},
		},
		{
			Name: "experience_update_achievement", Title: "Correct an achievement in my experience bank", Effect: assistant.EffectOverwrites, Idempotent: true,
			Description: "Corrects an achievement already in the bank: only the fields passed change; metrics and skills " +
				"replace the whole list when given. A correction keeps the achievement's provenance, so an agent_inferred " +
				"one stays uncitable. Returns the updated achievement.",
			Schema: objectSchema(map[string]any{
				"id":            achievementIDProperty,
				"claim":         map[string]any{"type": "string", "description": "The corrected sentence."},
				"context":       map[string]any{"type": "string", "description": "How it was done."},
				"metrics":       stringList("Replaces the whole metrics list."),
				"skills":        stringList("Replaces the whole skills list."),
				"employment_id": employmentIDProperty,
			}, "id"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID string `json:"id"`
					atomArgs
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				id, err := uuid.Parse(strings.TrimSpace(in.ID))
				if err != nil {
					return nil, fiber.NewError(fiber.StatusNotFound, "no such achievement in the experience bank")
				}
				ctx = auth.AgentContext(ctx, userID)
				current, err := h.experienceH.bank.GetAtom(ctx, id, userID)
				if err != nil {
					return nil, contractToolError(experienceError(err))
				}
				body, err := overlay(current, in.atomArgs)
				if err != nil {
					return nil, err
				}
				resp, err := h.experienceH.UpdateAtom(ctx, body, apigen.UpdateAtomParams{ID: id.String()})
				if err != nil {
					return nil, contractToolError(err)
				}
				return json.RawMessage(resp.Data), nil
			},
		},
		{
			Name: "experience_remove_achievement", Title: "Delete an achievement from my experience bank", Effect: assistant.EffectOverwrites,
			Description: "Deletes one achievement from the bank, and nothing else. There is no undo.",
			Schema:      objectSchema(map[string]any{"id": achievementIDProperty}, "id"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID string `json:"id"`
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				if err := h.experienceH.DeleteAtom(auth.AgentContext(ctx, userID), apigen.DeleteAtomParams{ID: strings.TrimSpace(in.ID)}); err != nil {
					return nil, contractToolError(err)
				}
				return map[string]any{"removed": in.ID}, nil
			},
		},
		{
			Name: "experience_remove_employment", Title: "Delete a place from my experience bank", Effect: assistant.EffectOverwrites,
			Description: "Deletes a place from the bank. Refused (409) while achievements are still attached to it, " +
				"since they would be deleted with it. There is no undo.",
			Schema: objectSchema(map[string]any{"id": employmentIDProperty}, "id"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID string `json:"id"`
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				if err := h.experienceH.DeleteEmployment(auth.AgentContext(ctx, userID), apigen.DeleteEmploymentParams{ID: strings.TrimSpace(in.ID)}); err != nil {
					return nil, contractToolError(err)
				}
				return map[string]any{"removed": in.ID}, nil
			},
		},
	}
}

func (h *assistantHandlers) tailoredCVMCPTools(origin string) []assistant.Tool {
	cvTool := func(name, title, description string, effect assistant.Effect, run func(ctx context.Context, userID int64, id string) (any, error)) assistant.Tool {
		return assistant.Tool{
			Name: name, Title: title, Description: description, Effect: effect, Idempotent: effect == assistant.EffectReads,
			Schema: objectSchema(map[string]any{"id": cvIDProperty}, "id"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID string `json:"id"`
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				return run(auth.AgentContext(ctx, userID), userID, strings.TrimSpace(in.ID))
			},
		}
	}
	return []assistant.Tool{
		{
			Name: "cv_list", Title: "List my tailored CVs", Effect: assistant.EffectReads, Idempotent: true,
			Description: "The candidate's tailored CVs, newest edit first, each with its id and the vacancy it was written for.",
			Schema:      objectSchema(map[string]any{}),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				if err := assistant.DecodeArgs(raw, &struct{}{}); err != nil {
					return nil, err
				}
				return okData(h.cv.ListCVs(auth.AgentContext(ctx, userID)))
			},
		},
		{
			Name: "cv_tailor", Title: "Start a CV tailored to a vacancy", Effect: assistant.EffectAdds, Idempotent: true,
			Description: "Starts a copy of the candidate's CV tailored to one vacancy and returns its id, the base it was " +
				"copied from and the cached fit analysis. The same vacancy again returns the existing copy. Needs a paid " +
				"plan and spends one tailoring from today's allowance the first time (402 otherwise); 409 when the " +
				"account has no CV on the site to start from. No model is called: the reframing is the caller's.",
			Schema: objectSchema(map[string]any{"job_slug": vacancySlugProperty}, "job_slug"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					JobSlug string `json:"job_slug"`
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				slug := strings.TrimSpace(in.JobSlug)
				if err := requireSlug("job_slug", slug); err != nil {
					return nil, err
				}
				// The gate POST /me/cvs/tailor sits behind (paidAI), so this door is not a way around paying.
				if err := requirePaidAI(ctx, entitlementOf(h.cv.plans), userID, plan.FeatureTailor); err != nil {
					return nil, err
				}
				res, err := h.cv.TailorCV(plan.WithPaidAI(auth.AgentContext(ctx, userID)), &apigen.CvTailorRequest{JobSlug: apigen.NewOptString(slug)})
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, jobNotFound(slug)
				}
				if err != nil {
					return nil, contractToolError(err)
				}
				switch res := res.(type) {
				case *apigen.CvPlanRefusal:
					return nil, assistant.WithLogReason(fiber.NewError(fiber.StatusPaymentRequired, res.Error), "tailoring allowance spent")
				case *apigen.TailorCVCreated:
					return okData(res, nil)
				}
				return nil, errors.New("cv_tailor: unexpected response")
			},
		},
		cvTool("cv_context", "Read a tailored CV's fit context", "The cached fit analysis a tailored CV is aimed at: "+
			"verdict, recommendation, and the vacancy's requirements split into missing_have (evidence exists to reframe) "+
			"and missing_gap (nothing banked yet). 409 when the CV is not tailored or has no fit analysis.",
			assistant.EffectReads, func(ctx context.Context, _ int64, id string) (any, error) {
				resp, err := h.cv.GetCVTailorContext(ctx, apigen.GetCVTailorContextParams{ID: id})
				if err != nil {
					return nil, contractToolError(err)
				}
				return json.RawMessage(resp.Data), nil
			}),
		cvTool("cv_get", "Read a tailored CV", "A tailored CV with its full document (summary, experience bullets, "+
			"skill groups, education), without the candidate's contact block. Edit paths are 0-indexed over this document.",
			assistant.EffectReads, func(ctx context.Context, _ int64, id string) (any, error) {
				return okData(h.cv.GetCV(ctx, apigen.GetCVParams{ID: id}))
			}),
		{
			Name: "cv_edit", Title: "Edit a tailored CV", Effect: assistant.EffectOverwrites,
			Description: "Applies a batch of path-addressed edits to a tailored CV, all or none, as one entry the candidate " +
				"can undo on the site. Edits are made as the agent: the contact block is refused, and every op stating " +
				"what the candidate did must cite evidence_id, an achievement they asserted; one uncited op rejects the " +
				"batch. A bad path is a 422 and leaves the CV untouched. Returns the CV's updated metadata.",
			Schema: objectSchema(map[string]any{
				"id": cvIDProperty,
				"ops": map[string]any{
					"type": "array", "minItems": 1, "description": "Every edit that belongs together.",
					"items": objectSchema(map[string]any{
						"kind":        map[string]any{"type": "string", "enum": []string{"set", "insert", "remove", "move"}, "description": "set replaces a node, insert adds one, remove deletes one, move reorders within its list."},
						"path":        map[string]any{"type": "string", "description": "Where, 0-indexed over the CV document: summary, experience[2].bullets[1], skills[0].items[3], education[1].degree."},
						"value":       map[string]any{"description": "The new content; left out for remove and move."},
						"to":          map[string]any{"type": "integer", "description": "Destination index, for move only."},
						"evidence_id": map[string]any{"type": "string", "description": "The banked achievement backing this claim."},
					}, "kind", "path"),
				},
				"note": map[string]any{"type": "string", "description": "A one-line reason, shown to the candidate as the agent's words."},
			}, "id", "ops"),
			Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
				var in struct {
					ID   string            `json:"id"`
					Ops  []json.RawMessage `json:"ops"`
					Note string            `json:"note,omitempty"`
				}
				if err := assistant.DecodeArgs(raw, &in); err != nil {
					return nil, err
				}
				body, err := json.Marshal(struct {
					Ops  []json.RawMessage `json:"ops"`
					Note string            `json:"note,omitempty"`
				}{in.Ops, in.Note})
				if err != nil {
					return nil, err
				}
				return okData(h.cv.PatchCV(auth.AgentContext(ctx, userID), body, apigen.PatchCVParams{ID: strings.TrimSpace(in.ID)}))
			},
		},
		cvTool("cv_render", "Render a tailored CV to PDF", "Renders a tailored CV to its ATS PDF and returns pdf_url, "+
			"where the signed-in candidate downloads it, with the file's size. The PDF carries the contact block.",
			assistant.EffectReads, func(ctx context.Context, userID int64, id string) (any, error) {
				if h.cv.pdf.renderer == nil {
					return nil, fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
				}
				cvUUID, err := cvID(id)
				if err != nil {
					return nil, err
				}
				pdf, err := h.cv.pdf.render(ctx, userID, cvUUID)
				if err != nil {
					return nil, contractToolError(mapCVError(err))
				}
				return map[string]any{
					"pdf_url":    strings.TrimSuffix(origin, "/") + "/api/v1/me/cvs/" + cvUUID.String() + "/pdf",
					"size_bytes": len(pdf),
				}, nil
			}),
	}
}

// requirePaidAI is the paid-AI gate (checkPaidAI) for a tool: a free account is refused with
// the plan's own sentence, and a plan that cannot be read never authorises.
func requirePaidAI(ctx context.Context, plans paidEntitlement, userID int64, feature plan.Feature) error {
	if plans == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable")
	}
	_, err := plans.RequirePaid(ctx, userID)
	if errors.Is(err, plan.ErrPaymentRequired) {
		return assistant.WithLogReason(fiber.NewError(fiber.StatusPaymentRequired, refusalMessage(feature, false, false)+" A paid plan is required."), "paid plan required")
	}
	if err != nil {
		return assistant.WithLogReason(fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable"), "plan read failed")
	}
	return nil
}
