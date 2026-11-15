package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/db"
)

// experienceBankTools is what the assistant needs from the experience bank: a narrow handler-side
// interface that makes the provenance gate testable without a database.
type experienceBankTools interface {
	Retrieve(ctx context.Context, userID int64, q experience.Query, limit int) ([]experience.Match, error)
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (experience.Atom, error)
	AddAtom(ctx context.Context, userID int64, a experience.Atom, author experience.Author) (experience.Atom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a experience.Atom, author experience.Author) (experience.Atom, error)
	MergeAtoms(ctx context.Context, userID int64, a, b uuid.UUID) (experience.Atom, error)
}

// experienceSearchLimit caps what one search puts into the transcript: tool results are replayed
// into the model's context on every later turn.
const experienceSearchLimit = 8

// experienceReadLimit caps one read-by-id for the same reason, and the overflow is reported rather
// than dropped: the ids were named deliberately, so silent loss would leave the model believing it
// saw the whole set. Its own constant, not one shared with the frontend, which would drift.
const experienceReadLimit = 8

// assistantExperienceTools are registered under every preset: a candidate may articulate their
// experience in any chat, not only a tailoring session.
func (h *assistantHandlers) assistantExperienceTools(sessionID uuid.UUID) []assistant.Tool {
	return []assistant.Tool{
		h.experienceSearchTool(),
		h.experienceGetTool(),
		h.experienceEmploymentsTool(),
		h.experienceAddTool(sessionID),
		h.experienceUpdateTool(sessionID),
		h.experienceMergeTool(),
		h.experienceSetRequireContextTool(),
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

// experienceGetTool reads named achievements, the half of the bank the agent could not see. Every
// other id-bearing surface leads here (the opening message naming a selection, get_profile's
// soft_duplicate_clusters, a merge's result id). experience_search retrieves by meaning, so a UUID
// matched nothing and the agent was told the achievement did not exist.
func (h *assistantHandlers) experienceGetTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_get",
		Description: "Read specific achievements by id — their full claim, situation, metrics, skills and " +
			"the role each belongs to. Use this whenever you hold an id rather than a topic: ids named in the " +
			"candidate's opening message, a cluster from get_profile's soft_duplicate_clusters, or the id a " +
			"merge returned. experience_search finds achievements by MEANING and cannot find one by id. " +
			"ALWAYS read an achievement before proposing to merge it or writing an update over it.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "The achievement ids to read.",
					"minItems":    1,
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
			// Answer in the order asked, minus repeats: a reply that follows the question is one the agent can act on.
			wanted := make([]string, 0, len(in.IDs))
			seen := make(map[string]bool, len(in.IDs))
			for _, raw := range in.IDs {
				id := strings.TrimSpace(raw)
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				wanted = append(wanted, id)
			}
			if len(wanted) == 0 {
				return nil, errors.New("give at least one achievement id, from the opening message, " +
					"get_profile's soft_duplicate_clusters, or experience_search")
			}
			var unread []string
			if len(wanted) > experienceReadLimit {
				unread = wanted[experienceReadLimit:]
				wanted = wanted[:experienceReadLimit]
			}

			// One owner-scoped pass rather than a lookup per id: ownership is a property of the query, not a
			// check that could be forgotten.
			atoms, err := h.experience.ListAtoms(ctx, userID)
			if err != nil {
				return nil, err
			}
			byID := make(map[uuid.UUID]experience.Atom, len(atoms))
			for _, a := range atoms {
				byID[a.ID] = a
			}
			employments, err := h.experience.ListEmployments(ctx, userID)
			if err != nil {
				return nil, err
			}
			roles := make(map[uuid.UUID]experience.Employment, len(employments))
			for _, e := range employments {
				roles[e.ID] = e
			}

			found := make([]map[string]any, 0, len(wanted))
			var unresolved []string
			for _, want := range wanted {
				id, err := uuid.Parse(want)
				if err != nil {
					unresolved = append(unresolved, want)
					continue
				}
				atom, ok := byID[id]
				if !ok {
					// Someone else's achievement is reported exactly as a deleted one:
					// this tool is not an existence oracle for other people's rows.
					unresolved = append(unresolved, want)
					continue
				}
				match := experience.Match{Atom: atom}
				if atom.EmploymentID != nil {
					if role, ok := roles[*atom.EmploymentID]; ok {
						match.Employment = &role
					}
				}
				found = append(found, atomEntry(match))
			}

			out := map[string]any{"achievements": found}
			// A partial answer beats a refusal; naming what failed lets the agent correct itself in the same turn.
			if len(unresolved) > 0 {
				out["unresolved"] = unresolved
				out["unresolved_note"] = "These are not achievements on this candidate's bank — they may have " +
					"been merged away or deleted. Do not ask about them; find what you need with experience_search."
			}
			if len(unread) > 0 {
				out["unread"] = unread
				out["unread_note"] = "Not read, because one call reads at most " +
					strconv.Itoa(experienceReadLimit) + ". Call experience_get again with these."
			}
			return out, nil
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
		Schema: assistant.EmptySchema,
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			employments, err := h.experience.ListEmployments(ctx, userID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"employments": employments}, nil
		},
	}
}

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
func (h *assistantHandlers) experienceMergeTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_merge",
		Description: "Merge TWO achievements that are the same work into one richer atom. " +
			"Ask the candidate first. The system keeps the richer id, unions metrics and skills, " +
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

// experienceSetRequireContextTool flips the per-user opt-in that requires situation
// context on interactive creates. Call only after the candidate agrees.
func (h *assistantHandlers) experienceSetRequireContextTool() assistant.Tool {
	return assistant.Tool{
		Name: "experience_set_require_context",
		Description: "Turn on or off the candidate's preference that new achievements must " +
			"include a short situation paragraph (context). Default is off. Call ONLY after " +
			"they agree. Import and edits are never gated by this.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled": map[string]any{
					"type":        "boolean",
					"description": "true to require context on new interactive creates; false to clear the preference.",
				},
			},
			"required":             []string{"enabled"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Enabled bool `json:"enabled"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.queries == nil {
				return nil, errors.New("cannot change this preference in this deployment")
			}
			if err := h.queries.SetUserExperienceRequireContext(ctx, db.SetUserExperienceRequireContextParams{
				ID: userID, ExperienceRequireContext: in.Enabled,
			}); err != nil {
				return nil, err
			}
			return map[string]any{"require_context": in.Enabled}, nil
		},
	}
}

// checkExperienceContextRequired mirrors the HTTP create gate for experience_add.
func (h *assistantHandlers) checkExperienceContextRequired(ctx context.Context, userID int64, contextText string) error {
	if strings.TrimSpace(contextText) != "" || h.queries == nil {
		return nil
	}
	on, err := h.queries.GetUserExperienceRequireContext(ctx, userID)
	if err != nil {
		return err
	}
	if on {
		return experience.ErrContextRequired
	}
	return nil
}

// provenanceFor decides how an atom entered the bank; the one place the honest wall is enforced on the
// write side. A quote found in the candidate's messages makes the claim theirs. One that is not (a
// paraphrase, a summary, an invention) is the model speaking, and is recorded as such rather than
// rejected, since the agent needs its hypothesis on record to ask the candidate about it. This is the
// only place a claim's standing can go UP, because something outside the model confirmed the words.
func (h *assistantHandlers) authorFor(ctx context.Context, sessionID uuid.UUID, said string) experience.Author {
	said = strings.TrimSpace(said)
	if said == "" || h.store == nil {
		return experience.AuthorAgent
	}
	transcript, err := h.store.Transcript(ctx, sessionID)
	if err != nil {
		// Unverifiable is not verified: failing closed costs a confirmation step, failing open puts an
		// unchecked claim on their CV.
		return experience.AuthorAgent
	}
	if assistant.UserSaid(transcript, said) {
		return experience.AuthorQuoted
	}
	return experience.AuthorAgent
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

// resolveEmployment parses the role an achievement attaches to and, when the value is not an id,
// refuses with the ids that would have worked. A model that guesses ("the ringcentral one") would
// otherwise burn one of its few rounds on experience_employments; the roles are a handful of
// owner-scoped rows.
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

// atomEntry is one achievement as the model sees it: the claim, the place, and whether it may be written
// into a CV as it stands. Shared by every tool that returns an achievement, so the same achievement
// found by search or read by id never arrives with different fields.
func atomEntry(m experience.Match) map[string]any {
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
		if m.Employment.Kind == experience.KindProject {
			entry["name"] = m.Employment.Company
		} else {
			entry["company"] = m.Employment.Company
		}
		entry["period"] = perioddate.FormatRange(m.Employment.Start, m.Employment.End, m.Employment.Current)
	}
	return entry
}

// searchResult shapes matches for the model.
func searchResult(matches []experience.Match) map[string]any {
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		out = append(out, atomEntry(m))
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
