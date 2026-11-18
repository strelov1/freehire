package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

// experienceAddTool records one achievement. The `said` argument is the honest wall's mechanism: the
// model supplies the candidate's own words verbatim and the service checks them against the session
// transcript. A real quote makes the atom the candidate's assertion; anything else is recorded as
// agent_inferred and barred from a CV until the candidate confirms it.
func (h *assistantHandlers) experienceAddTool(sessionID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "experience_add",
		Description: "Record ONE achievement in the candidate's experience bank, so it is available " +
			"in every later session and in the CVs they build. Set `said` to the candidate's own words " +
			"from this conversation, copied exactly — that is what marks the achievement as theirs. " +
			"Leave `said` out when you are recording your own reading of something rather than what " +
			"they stated; such an entry is kept but can never be written into a CV until they confirm it.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claim": map[string]any{
					"type":        "string",
					"description": "The achievement as one CV-bullet-grade sentence, e.g. \"Cut message-posting latency from 20s to 1s\".",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "How it was done, in a sentence or two. This is the raw material for reframing it against a vacancy later.",
				},
				"metrics": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "The numbers, as stated: \"20s -> 1s\", \"150+ engineers\", \"40% cost\".",
				},
				"skills": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Canonical skill slugs the achievement demonstrates (go, kubernetes, kafka).",
				},
				"employment_id": map[string]any{
					"type":        "string",
					"description": "The id from experience_employments for the role this happened in. Omit when it belongs to no single role.",
				},
				"said": map[string]any{
					"type": "string",
					"description": "The candidate's own words backing this achievement, copied verbatim from " +
						"their message in this conversation. Omit if they did not say it.",
				},
			},
			"required":             []string{"claim"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Claim        string   `json:"claim"`
				Context      string   `json:"context"`
				Metrics      []string `json:"metrics"`
				Skills       []string `json:"skills"`
				EmploymentID string   `json:"employment_id"`
				Said         string   `json:"said"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			employmentID, err := h.resolveEmployment(ctx, userID, in.EmploymentID)
			if err != nil {
				return nil, err
			}

			if err := h.checkExperienceContextRequired(ctx, userID, in.Context); err != nil {
				return nil, err
			}
			atom := experience.Atom{
				EmploymentID: employmentID,
				Claim:        in.Claim,
				Context:      in.Context,
				Metrics:      in.Metrics,
				Skills:       in.Skills,
			}
			stored, err := h.experience.AddAtom(ctx, userID, atom, h.authorFor(ctx, sessionID, in.Said))
			if errors.Is(err, experience.ErrAlreadyBanked) {
				// Not a failure: the candidate learns it is already recorded, and the model stops retrying.
				return map[string]any{"already_banked": true, "claim": in.Claim}, nil
			}
			if err != nil {
				return nil, err
			}
			return addResult(stored), nil
		},
	}
}

// experienceUpdateTool sharpens an atom the bank already holds — a metric that surfaced, a
// detail the first telling left out.
func (h *assistantHandlers) experienceUpdateTool(sessionID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "experience_update",
		Description: "Refine an achievement already in the bank: add the metric that just came up, " +
			"correct a detail, attach it to the right role. Address it by the id a search returned. " +
			"The fields you send replace the ones stored; omit a field to leave it as it is. Changing " +
			"claim, context or metrics re-earns provenance the same way experience_add does: cite the " +
			"candidate's own words in said, or the edit is recorded as your inference, not theirs.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string", "description": "The atom's id, from experience_search."},
				"claim":         map[string]any{"type": "string", "description": "A replacement claim sentence."},
				"context":       map[string]any{"type": "string", "description": "A replacement context paragraph."},
				"metrics":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The full replacement metric list."},
				"skills":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The full replacement skill list."},
				"employment_id": map[string]any{"type": "string", "description": "The role this belongs to, from experience_employments."},
				"said": map[string]any{
					"type": "string",
					"description": "The candidate's own words backing the new claim, context or metrics, copied " +
						"verbatim from their message in this conversation. Required to keep a confirmed " +
						"achievement confirmed when you change what it says; omit if they did not say it.",
				},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID           string    `json:"id"`
				Claim        *string   `json:"claim"`
				Context      *string   `json:"context"`
				Metrics      *[]string `json:"metrics"`
				Skills       *[]string `json:"skills"`
				EmploymentID *string   `json:"employment_id"`
				Said         string    `json:"said"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			id, err := uuid.Parse(strings.TrimSpace(in.ID))
			if err != nil {
				return nil, errors.New("id must be an atom id from experience_search")
			}

			atom, err := h.experience.GetAtom(ctx, id, userID)
			if err != nil {
				return nil, err
			}
			// Changed content re-derives authorship instead of carrying it over, or a confirmed atom would stay
			// "confirmed" however far the model edits it. An edit touching no content keeps its standing.
			author := experience.AuthorRewrite
			if in.Claim != nil || in.Context != nil || in.Metrics != nil {
				author = h.authorFor(ctx, sessionID, in.Said)
			}
			if in.Claim != nil {
				atom.Claim = *in.Claim
			}
			if in.Context != nil {
				atom.Context = *in.Context
			}
			if in.Metrics != nil {
				atom.Metrics = *in.Metrics
			}
			if in.Skills != nil {
				atom.Skills = *in.Skills
			}
			if in.EmploymentID != nil {
				employmentID, err := optionalUUID(*in.EmploymentID)
				if err != nil {
					return nil, err
				}
				atom.EmploymentID = employmentID
			}

			stored, err := h.experience.UpdateAtom(ctx, id, userID, atom, author)
			if err != nil {
				return nil, err
			}
			return addResult(stored), nil
		},
	}
}

// experienceMergeTool folds two near-duplicate achievements into one. The owner
// confirms the pair in chat; the service chooses the richer keep.
func (h *assistantHandlers) experienceMergeTool(sessionID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name: "experience_merge",
		Description: "Merge TWO achievements that are the same work into one richer atom. " +
			"First call returns a confirmation claim without changing anything. Show it with request_confirmation " +
			"and wait for the candidate's next turn before calling again with the same ids. " +
			"The system keeps the richer id, unions metrics and skills, " +
			"keeps the better context, and deletes the other. Both must be unplaced or share a role. " +
			"Returns the kept atom and the deleted id.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Exactly two atom ids from experience_search or get_profile's soft_duplicate_clusters.",
					"minItems":    2,
					"maxItems":    2,
				},
			},
			"required":             []string{"ids"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				IDs []string `json:"ids"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if len(in.IDs) != 2 {
				return nil, experience.ErrInvalidMerge
			}
			a, err := uuid.Parse(strings.TrimSpace(in.IDs[0]))
			if err != nil {
				return nil, errors.New("ids must be atom ids from experience_search")
			}
			b, err := uuid.Parse(strings.TrimSpace(in.IDs[1]))
			if err != nil {
				return nil, errors.New("ids must be atom ids from experience_search")
			}
			if a == b {
				return nil, experience.ErrInvalidMerge
			}
			if a.String() > b.String() {
				a, b = b, a
			}
			first, err := h.experience.GetAtom(ctx, a, userID)
			if err != nil {
				return nil, err
			}
			second, err := h.experience.GetAtom(ctx, b, userID)
			if err != nil {
				return nil, err
			}
			claim := fmt.Sprintf("Merge these two achievements:\n1. %s\n2. %s", first.Claim, second.Claim)
			confirmed := false
			if h.store != nil {
				if _, err := h.store.Session(ctx, sessionID, userID); err != nil {
					return nil, err
				}
				messages, err := h.store.RecentTranscript(ctx, sessionID, 100)
				if err != nil {
					return nil, err
				}
				confirmed = assistant.UserConfirmed(messages, claim)
			}
			if !confirmed {
				return map[string]any{
					"status": "awaiting_candidate_response", "claim": claim,
					"next": "Call request_confirmation with this exact claim, then wait for the candidate. Nothing was merged.",
				}, nil
			}
			kept, err := h.experience.MergeAtoms(ctx, userID, a, b)
			if err != nil {
				return nil, err
			}
			deleted := a
			if kept.ID == a {
				deleted = b
			}
			return map[string]any{
				"kept":       addResult(kept),
				"deleted_id": deleted,
			}, nil
		},
	}
}
