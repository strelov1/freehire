package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/skilltag"
	"github.com/strelov1/freehire/internal/userprofile"
)

// noProfileResult is what get_profile answers when the caller has never saved a
// profile. It is a result, not an error: the agent has learned something true and
// should act on it. The instruction matters as much as the fact — collecting the
// same preferences in chat would produce a better answer this once and nothing
// durable, where the profile page's values persist and drive the ranked feed,
// subscriptions and fit analysis.
type noProfileResult struct {
	Profile any    `json:"profile"`
	Next    string `json:"next"`
}

// getProfileTool reads the caller's own saved profile: the specializations, skills
// and location preferences they curated, plus their structured CV with the contact
// fields left out.
//
// This is the tool that stops the agent interrogating a user for what they have
// already told the product. Registered for every session, and worth calling before
// the first search of a conversation.
func (h *assistantHandlers) getProfileTool() assistant.Tool {
	return assistant.Tool{
		Name: "get_profile",
		Description: "Read the user's saved job-search profile: the roles and skills they " +
			"want, the skills they want to avoid, where and how they will work, and their CV " +
			"(headline, years, experience, education — contacts are not included). Call this " +
			"before asking the user what they are looking for; only ask about what the profile " +
			"does not answer. The experience section reports the SHAPE of what they have told us — " +
			"which roles we know and how much evidence each carries — not the achievements " +
			"themselves; read those with experience_search when you need them. Takes no arguments.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			profile, err := h.profile.userProfile.Get(ctx, userID)
			if errors.Is(err, userprofile.ErrNotFound) {
				return noProfileResult{
					Next: "The user has not saved a profile yet. Point them at /my/profile to " +
						"fill it in — it persists and drives their recommendations — rather than " +
						"collecting the same answers in this conversation.",
				}, nil
			}
			if err != nil {
				return nil, err
			}
			return h.profileToolResult(ctx, userID, profile), nil
		},
	}
}

// profileToolResult is what the agent sees. It differs from the HTTP profile response in
// one deliberate way: the experience bank is reported as a SHAPE — the roles, how much
// evidence each carries, and which stated skills have none — rather than as content.
//
// A tool result is persisted in the transcript and replayed into the model's context on
// every later turn. A two-hundred-atom bank replayed each turn would consume the window
// and defeat trimming, so content is fetched per question through experience_search and
// the summary tells the agent where it is worth asking one.
type profileToolResult struct {
	Specializations     []string                    `json:"specializations"`
	Skills              []string                    `json:"skills"`
	ExcludedSkills      []string                    `json:"excluded_skills,omitempty"`
	LocationPreferences any                         `json:"location_preferences,omitempty"`
	CV                  *resumeextract.Professional `json:"cv,omitempty"`
	Experience          *experienceSummary          `json:"experience,omitempty"`
}

// experienceSummary is the bank at a glance.
type experienceSummary struct {
	Employments []employmentSummary `json:"employments"`
	// SkillsWithoutEvidence are skills the candidate claims on their profile that nothing
	// in the bank supports. This is the interviewer's work list, and the tailoring agent's
	// warning: a skill here cannot be written onto a CV, however central the vacancy makes it.
	SkillsWithoutEvidence []string `json:"skills_without_evidence,omitempty"`
	TotalAchievements     int      `json:"total_achievements"`
	// SoftDuplicateClusters are id-lists of near-paraphrase achievements (no claim text).
	SoftDuplicateClusters [][]string `json:"soft_duplicate_clusters,omitempty"`
	NeedsContextCount     int        `json:"needs_context_count,omitempty"`
	NeedsMetricsCount     int        `json:"needs_metrics_count,omitempty"`
	RequireContext        bool       `json:"require_context,omitempty"`
}

const (
	softDupClusterCap   = 8
	softDupClusterIDCap = 6
)

type employmentSummary struct {
	ID           uuid.UUID `json:"id"`
	Company      string    `json:"company,omitempty"`
	Role         string    `json:"role,omitempty"`
	Period       string    `json:"period,omitempty"`
	Current      bool      `json:"current,omitempty"`
	Achievements int       `json:"achievements"`
}

// profileToolResult assembles the agent's view of a caller's profile.
func (h *assistantHandlers) profileToolResult(ctx context.Context, userID int64, profile userprofile.Profile) profileToolResult {
	out := profileToolResult{
		Specializations:     profile.Specializations,
		Skills:              profile.Skills,
		ExcludedSkills:      profile.ExcludedSkills,
		LocationPreferences: profile.LocationPreferences,
		CV:                  h.profile.structuredCV(ctx, userID),
	}
	// The CV block already carries the whole work history, which is the very thing this
	// summary exists to keep out of the transcript.
	if out.CV != nil {
		trimmed := *out.CV
		trimmed.Experience = nil
		out.CV = &trimmed
	}
	if summary := h.experienceSummary(ctx, userID, profile.Skills); summary != nil {
		out.Experience = summary
	}
	return out
}

// experienceSummary counts the bank rather than reading it out.
func (h *assistantHandlers) experienceSummary(ctx context.Context, userID int64, claimed []string) *experienceSummary {
	if h.experience == nil {
		return nil
	}
	employments, err := h.experience.ListEmployments(ctx, userID)
	if err != nil {
		return nil
	}
	atoms, err := h.experience.ListAtoms(ctx, userID)
	if err != nil {
		return nil
	}
	if len(employments) == 0 && len(atoms) == 0 {
		return nil
	}

	counts := map[uuid.UUID]int{}
	evidenced := map[string]bool{}
	out := &experienceSummary{TotalAchievements: len(atoms)}
	for _, a := range atoms {
		if a.EmploymentID != nil {
			counts[*a.EmploymentID]++
		}
		for _, skill := range a.Skills {
			evidenced[skill] = true
		}
		nc, nm := experience.Richness(a)
		if nc {
			out.NeedsContextCount++
		}
		if nm {
			out.NeedsMetricsCount++
		}
	}
	for i, cluster := range experience.SoftDuplicateClusters(atoms) {
		if i >= softDupClusterCap {
			break
		}
		ids := make([]string, 0, softDupClusterIDCap)
		for j, id := range cluster {
			if j >= softDupClusterIDCap {
				break
			}
			ids = append(ids, id.String())
		}
		out.SoftDuplicateClusters = append(out.SoftDuplicateClusters, ids)
	}
	for _, e := range employments {
		out.Employments = append(out.Employments, employmentSummary{
			ID: e.ID, Company: e.Company, Role: e.Role, Current: e.Current,
			Period:       strings.TrimSpace(e.Start + " – " + e.End),
			Achievements: counts[e.ID],
		})
		for _, skill := range e.Stack {
			evidenced[skill] = true
		}
	}
	for _, skill := range skilltag.Canonicalize(claimed, skilltag.WithResumeAcronyms()) {
		if !evidenced[skill] {
			out.SkillsWithoutEvidence = append(out.SkillsWithoutEvidence, skill)
		}
	}
	if h.queries != nil {
		if on, err := h.queries.GetUserExperienceRequireContext(ctx, userID); err == nil {
			out.RequireContext = on
		}
	}
	return out
}
