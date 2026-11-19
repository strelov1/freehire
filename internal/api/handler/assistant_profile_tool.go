package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// noProfileResult is what get_profile answers when no profile was ever saved: a result, not an error.
// Its instruction sends the agent to the profile page, whose values persist and drive the ranked
// feed, subscriptions and fit analysis; preferences collected in chat do not.
type noProfileResult struct {
	Profile any `json:"profile"`
	// Account is the connected account's email, masked: the one thing a person cannot see from
	// inside the agent. The full address never reaches the model.
	Account string `json:"account,omitempty"`
	Next    string `json:"next"`
	// Background and CV (its deprecated alias, see profileToolResult) are still served: the
	// headline and summary live apart from the preferences row, so a candidate can have text
	// and no saved preferences, and update_profile can write one without the other.
	Background *resumeextract.Professional `json:"background,omitempty"`
	CV         *resumeextract.Professional `json:"cv,omitempty"`
}

// maskAccountEmail keeps the first character of the local part and the domain: j***@gmail.com.
// A value without a usable "@" is fully hidden.
func maskAccountEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(email)
	return string(first) + "***" + email[at:]
}

// noProfileNext tells the agent which account it is connected as, so a person who saved
// their profile under another account is sent to reconnect instead of re-entering it.
func noProfileNext(masked string) string {
	next := "The user has not saved a profile yet. If you have update_profile, offer to " +
		"save what they tell you and call it once they agree; otherwise point them at " +
		"/my/profile to fill it in. Either way it persists and drives their " +
		"recommendations, which answers given only in this conversation do not."
	if masked == "" {
		return next
	}
	return "This agent is connected as " + masked + ", and that account has no saved profile. " +
		"If it is not the account the user signs in with on the site, tell them to reconnect " +
		"while signed in to the right one. Otherwise: " + next
}

// getProfileTool reads the caller's saved profile: specializations, skills and location
// preferences, plus the structured CV without contact fields. It stops the agent re-asking what
// the user already told the product; call it before the first search.
func (h *assistantHandlers) getProfileTool() assistant.Tool {
	return assistant.Tool{
		Name:   "get_profile",
		Title:  "Read my job-search profile",
		Effect: assistant.EffectReads,
		Description: "Read the user's saved job-search profile: the roles and skills they " +
			"want, the skills they want to avoid, where and how they will work, and their CV " +
			"(headline, years, experience, education — contacts are not included). Call this " +
			"before asking the user what they are looking for; only ask about what the profile " +
			"does not answer. The background block (headline, summary, years, education) is part of the " +
			"profile, not a CV document — CV documents are read and edited with cv_get and cv_edit; " +
			"`cv` repeats it under its old name. The experience section reports the SHAPE of what they have told us — " +
			"which roles we know and how much evidence each carries — not the achievements " +
			"themselves. Takes no arguments.",
		Schema: assistant.EmptySchema,
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			// Registered on every assembly: the assistant's constructor takes each surface as a pointer
			// that may be nil, and a nil cvH would panic inside the SSE goroutine.
			if h.profile == nil || h.profile.userProfile == nil {
				return nil, unavailable("the saved profile")
			}
			profile, err := h.profile.userProfile.Get(ctx, userID)
			if errors.Is(err, userprofile.ErrNotFound) {
				masked := ""
				if h.queries != nil {
					if user, uerr := h.queries.GetUserByID(ctx, userID); uerr == nil {
						masked = maskAccountEmail(user.Email)
					}
				}
				text := h.profileTextBlock(ctx, userID)
				return noProfileResult{Background: text, CV: text, Account: masked, Next: noProfileNext(masked)}, nil
			}
			if err != nil {
				return nil, err
			}
			return h.profileToolResult(ctx, userID, profile), nil
		},
	}
}

// profileToolResult is what the agent sees. Unlike the HTTP profile response it reports the
// experience bank as a SHAPE (roles, evidence per role, stated skills with none), not content:
// a tool result is replayed into the model's context on every later turn, so a big bank would
// consume the window. Content is fetched per question through experience_search.
type profileToolResult struct {
	Specializations     []string `json:"specializations"`
	Skills              []string `json:"skills"`
	Seniorities         []string `json:"seniorities,omitempty"`
	JobLanguages        []string `json:"job_languages,omitempty"`
	ExcludedSkills      []string `json:"excluded_skills,omitempty"`
	LocationPreferences any      `json:"location_preferences,omitempty"`
	// Background is the candidate's professional self-description: headline, summary,
	// years, education, languages (work history is left to Experience). It is profile data,
	// not a CV document, which is why it is named for what it is and not "cv".
	Background *resumeextract.Professional `json:"background,omitempty"`
	// CV is the same block under its original name, kept so older readers do not break.
	//
	// Deprecated: drop it once agents read `background`; it doubles this block in every transcript.
	CV         *resumeextract.Professional `json:"cv,omitempty"`
	Experience *experienceSummary          `json:"experience,omitempty"`
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

// profileTextBlock is the candidate's headline, summary, years, education and languages, or
// nil when nothing is known. The block already carries the whole work history, which is the
// very thing the experience summary exists to keep out of the transcript, so it is dropped.
func (h *assistantHandlers) profileTextBlock(ctx context.Context, userID int64) *resumeextract.Professional {
	block := h.profile.structuredCV(ctx, userID)
	if block == nil {
		return nil
	}
	trimmed := *block
	trimmed.Experience = nil
	return &trimmed
}

// profileToolResult assembles the agent's view of a caller's profile.
func (h *assistantHandlers) profileToolResult(ctx context.Context, userID int64, profile userprofile.Profile) profileToolResult {
	out := profileToolResult{
		Specializations:     profile.Specializations,
		Skills:              profile.Skills,
		Seniorities:         profile.Seniorities,
		JobLanguages:        profile.JobLanguages,
		ExcludedSkills:      profile.ExcludedSkills,
		LocationPreferences: profile.LocationPreferences,
	}
	out.CV = h.profileTextBlock(ctx, userID)
	out.Background = out.CV
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
			Period:       perioddate.FormatRange(e.Start, e.End, e.Current),
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
