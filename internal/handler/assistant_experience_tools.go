package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/experience"
)

// experienceBankTools is what the assistant needs from the experience bank. Narrow
// handler-side interface, as everywhere else in this layer: it makes the provenance gate
// testable without a database, which matters most for the one rule the whole capability
// exists to keep.
type experienceBankTools interface {
	Retrieve(ctx context.Context, userID int64, q experience.Query, limit int) ([]experience.Match, error)
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (experience.Atom, error)
	AddAtom(ctx context.Context, userID int64, a experience.Atom) (experience.Atom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a experience.Atom) (experience.Atom, error)
}

// experienceSearchLimit caps what one search puts into the transcript. A tool result is
// replayed into the model's context on every later turn, so an unbounded bank read would
// consume the window a few searches in.
const experienceSearchLimit = 8

// assistantExperienceTools are registered under EVERY preset. The moment a candidate
// articulates their experience is not scheduled: a chat about the market surfaces "I
// actually ran that migration at Sber" as readily as a tailoring session does, and the
// only version of this feature that works is the one that is listening at the time.
func (h *assistantHandlers) assistantExperienceTools(sessionID uuid.UUID) []assistant.Tool {
	return []assistant.Tool{
		h.experienceSearchTool(),
		h.experienceEmploymentsTool(),
		h.experienceAddTool(sessionID),
		h.experienceUpdateTool(),
	}
}

// experienceSearchTool finds the candidate's own evidence for a requirement.
func (h *assistantHandlers) experienceSearchTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_search",
		Description: "Search the candidate's experience bank for evidence backing a requirement. " +
			"Call this BEFORE asking them whether they have done something — most of the time they " +
			"have already told us, in an earlier session or on their CV. Returns the matching " +
			"achievements with the role each came from, best match first. An empty result is a real " +
			"answer: the bank holds nothing on that point, so ask them.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The requirement in words, e.g. \"operating Kubernetes in production\" or \"led a team\".",
				},
				"skills": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Canonical skill slugs the requirement names (go, react, kubernetes). Optional — many requirements name none.",
				},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Query  string   `json:"query"`
				Skills []string `json:"skills"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if strings.TrimSpace(in.Query) == "" && len(in.Skills) == 0 {
				return nil, errors.New("give a query, a skill list, or both")
			}
			matches, err := h.experience.Retrieve(ctx, userID,
				experience.Query{Text: in.Query, Skills: in.Skills}, experienceSearchLimit)
			if err != nil {
				return nil, err
			}
			return searchResult(matches), nil
		},
	}
}

// experienceEmploymentsTool lists the places the bank already knows, so a new atom attaches
// to one instead of creating a fourth spelling of the same employer.
func (h *assistantHandlers) experienceEmploymentsTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_employments",
		Description: "List the candidate's known roles and projects, with their ids. Call this before " +
			"recording an achievement so it attaches to the role it belongs to. Takes no arguments.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			employments, err := h.experience.ListEmployments(ctx, userID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"employments": employments}, nil
		},
	}
}

// experienceAddTool records one achievement.
//
// The `said` argument is the honest wall's mechanism. The model does not choose the
// provenance — it supplies the candidate's own words, verbatim, and the service checks
// them against the session transcript. A quote that is really there makes the atom the
// candidate's assertion; anything else is the model's own, recorded as agent_inferred and
// barred from reaching a CV until the candidate confirms it.
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

			atom := experience.Atom{
				EmploymentID: employmentID,
				Claim:        in.Claim,
				Context:      in.Context,
				Metrics:      in.Metrics,
				Skills:       in.Skills,
				Provenance:   h.provenanceFor(ctx, sessionID, in.Said),
			}
			stored, err := h.experience.AddAtom(ctx, userID, atom)
			if errors.Is(err, experience.ErrAlreadyBanked) {
				// Not a failure: the candidate learns it is already recorded, and the
				// model stops trying to record it again.
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
func (h *assistantHandlers) experienceUpdateTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_update",
		Description: "Refine an achievement already in the bank: add the metric that just came up, " +
			"correct a detail, attach it to the right role. Address it by the id a search returned. " +
			"The fields you send replace the ones stored; omit a field to leave it as it is.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string", "description": "The atom's id, from experience_search."},
				"claim":         map[string]any{"type": "string", "description": "A replacement claim sentence."},
				"context":       map[string]any{"type": "string", "description": "A replacement context paragraph."},
				"metrics":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The full replacement metric list."},
				"skills":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The full replacement skill list."},
				"employment_id": map[string]any{"type": "string", "description": "The role this belongs to, from experience_employments."},
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

			stored, err := h.experience.UpdateAtom(ctx, id, userID, atom)
			if err != nil {
				return nil, err
			}
			return addResult(stored), nil
		},
	}
}

// provenanceFor decides how an atom entered the bank, and it is the one place the honest
// wall is actually enforced on the write side.
//
// The model supplies the candidate's words; the service checks them against what the
// candidate really said in this session. A quote that appears in their messages makes the
// atom theirs. A quote that does not — a paraphrase, a summary, an invention — is the
// model speaking, and is recorded as such rather than rejected: the agent needs its own
// hypothesis on record in order to ask the candidate about it.
func (h *assistantHandlers) provenanceFor(ctx context.Context, sessionID uuid.UUID, said string) experience.Provenance {
	said = strings.TrimSpace(said)
	if said == "" || h.store == nil {
		return experience.ProvenanceAgentInferred
	}
	transcript, err := h.store.Transcript(ctx, sessionID)
	if err != nil {
		// Unverifiable is not verified. Failing closed costs the candidate a confirmation
		// step; failing open would put an unchecked claim on their CV.
		return experience.ProvenanceAgentInferred
	}
	if assistant.UserSaid(transcript, said) {
		return experience.ProvenanceStatedInChat
	}
	return experience.ProvenanceAgentInferred
}

// optionalUUID parses an id that may be absent, returning nil for an empty string.
func optionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errors.New("employment_id must be an id from experience_employments")
	}
	return &id, nil
}

// resolveEmployment parses the role an achievement attaches to, and — when the value is not
// an id — refuses with the ids that WOULD have worked.
//
// The list is the whole point. A model that guesses ("the ringcentral one") used to be told
// only that it was wrong, spend a round on experience_employments, and retry: one of the eight
// rounds a turn has, burned on a lookup the refusal could have carried. The candidate's roles
// are a handful of rows, already owner-scoped, and they are what the retry needs.
func (h *assistantHandlers) resolveEmployment(ctx context.Context, userID int64, raw string) (*uuid.UUID, error) {
	id, err := optionalUUID(raw)
	if err == nil {
		return id, nil
	}
	if h.experience == nil {
		return nil, err
	}
	employments, listErr := h.experience.ListEmployments(ctx, userID)
	if listErr != nil {
		return nil, err // the original message; a failed lookup is not the model's problem to fix
	}
	if len(employments) == 0 {
		return nil, errors.New("this candidate has no roles on file yet, so employment_id must be left out — " +
			"record the achievement without one")
	}
	var b strings.Builder
	b.WriteString("employment_id must be one of this candidate's roles: ")
	for i, e := range employments {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s = %s", e.ID, strings.TrimSpace(e.Company+" "+e.Role))
	}
	b.WriteString(". Leave it out if none of them is where this happened.")
	return nil, errors.New(b.String())
}

// searchResult shapes matches for the model: the claim it can reframe, the place it
// happened, and whether it may be written into a CV as it stands.
func searchResult(matches []experience.Match) map[string]any {
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		entry := map[string]any{
			"id":           m.Atom.ID,
			"claim":        m.Atom.Claim,
			"skills":       m.Atom.Skills,
			"confirmed":    m.Atom.Provenance.Publishable(),
			"can_write_cv": m.Atom.Provenance.Publishable(),
		}
		if m.Atom.Context != "" {
			entry["context"] = m.Atom.Context
		}
		if len(m.Atom.Metrics) > 0 {
			entry["metrics"] = m.Atom.Metrics
		}
		if m.Employment != nil {
			entry["role"] = m.Employment.Role
			entry["company"] = m.Employment.Company
			entry["period"] = strings.TrimSpace(m.Employment.Start + " – " + m.Employment.End)
		}
		out = append(out, entry)
	}
	return map[string]any{"evidence": out}
}

// addResult tells the model what was stored and, crucially, whether it may use it.
func addResult(a experience.Atom) map[string]any {
	out := map[string]any{
		"id":           a.ID,
		"claim":        a.Claim,
		"skills":       a.Skills,
		"can_write_cv": a.Provenance.Publishable(),
	}
	if !a.Provenance.Publishable() {
		out["next"] = "Recorded as your own reading, not the candidate's statement, because no verbatim " +
			"quote from their messages backed it. Ask them to confirm it before writing it into a CV."
	}
	return out
}
