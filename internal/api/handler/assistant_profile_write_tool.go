package handler

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// updateProfileArgs is a partial update: a field left out keeps what the profile holds.
// A list that IS sent replaces the stored one, so `[]` clears it. Plain slices tell the
// two apart because encoding/json leaves an absent field nil and decodes `[]` to an empty,
// non-nil slice.
type updateProfileArgs struct {
	Specializations     []string                         `json:"specializations"`
	Skills              []string                         `json:"skills"`
	AddSkills           []string                         `json:"add_skills"`
	ExcludedSkills      []string                         `json:"excluded_skills"`
	Seniorities         []string                         `json:"seniorities"`
	JobLanguages        []string                         `json:"job_languages"`
	LocationPreferences *userprofile.LocationPreferences `json:"location_preferences"`
}

// profileUpdateResult is the profile as saved, plus what the dictionary refused.
type profileUpdateResult struct {
	Specializations     []string `json:"specializations"`
	Skills              []string `json:"skills"`
	ExcludedSkills      []string `json:"excluded_skills"`
	Seniorities         []string `json:"seniorities"`
	JobLanguages        []string `json:"job_languages"`
	LocationPreferences any      `json:"location_preferences,omitempty"`
	// NotInDictionary are skills that were NOT saved because the skill dictionary does not
	// resolve them. They are reported rather than guessed at: a skill we cannot canonicalise
	// would never match a vacancy's tags.
	NotInDictionary []string `json:"not_in_dictionary,omitempty"`
	Next            string   `json:"next,omitempty"`
}

// profileMCPTools are the profile writes served to an OAuth grant. PUT /me/profile is
// cookie-only, so a full-scope API key does not get them either.
func (h *assistantHandlers) profileMCPTools() []assistant.Tool {
	return []assistant.Tool{h.updateProfileTool()}
}

// updateProfileTool changes the saved job-search preferences. It reads the profile, applies
// only the fields the caller sent, and saves through userprofile.Service, so the same
// normalisation and caps as the profile page apply.
func (h *assistantHandlers) updateProfileTool() assistant.Tool {
	stringList := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
	}
	geo := map[string]any{"type": "object", "properties": map[string]any{
		"regions":   stringList("Regions: " + strings.Join(vocab.RegionValues, ", ") + "."),
		"countries": stringList("ISO 3166-1 alpha-2 country codes."),
	}}
	return assistant.Tool{
		Name:   "update_profile",
		Title:  "Update my job-search profile",
		Effect: assistant.EffectOverwrites,
		Description: "Change the user's saved job-search profile. This changes their account, so say what " +
			"you are about to change and get the user's agreement first. Call get_profile first. Send only " +
			"the fields that change; a field you leave out stays as it is, and a list you send REPLACES " +
			"the stored one (use add_skills to add without touching the rest). Skills are matched against " +
			"freehire's skill dictionary; any it does not know are NOT saved and come back in " +
			"not_in_dictionary — offer to call suggest_dictionary_term for those. This edits preferences " +
			"only, not the CV text.",
		Idempotent: true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"specializations": stringList("Job categories wanted, from the specialization vocabulary (see facets)."),
				"skills":          stringList("The full list of skills wanted. Replaces the stored list."),
				"add_skills":      stringList("Skills to add to the stored list. Cannot be combined with skills."),
				"excluded_skills": stringList("Skills to avoid. Replaces the stored list; a skill that is also wanted is dropped from it."),
				"seniorities":     stringList("Levels wanted: " + strings.Join(vocab.SeniorityValues, ", ") + "."),
				"job_languages":   stringList("Two-letter codes of the languages vacancies may be written in."),
				"location_preferences": map[string]any{
					"type": "object",
					"description": "Where and how they will work. Replaces the whole block, so send every part " +
						"that should stay. Read it from get_profile first.",
					"properties": map[string]any{
						"work_modes": stringList("Any of: " + strings.Join(vocab.WorkModeValues, ", ") + "."),
						"remote":     geo,
						"base": map[string]any{"type": "object", "properties": map[string]any{
							"country": map[string]any{"type": "string"},
							"city":    map[string]any{"type": "string"},
						}},
						"relocation": map[string]any{"type": "object", "properties": map[string]any{
							"open":      map[string]any{"type": "boolean"},
							"regions":   stringList("Regions they would move to."),
							"countries": stringList("ISO country codes they would move to."),
							"cities":    stringList("Cities they would move to."),
						}},
					},
				},
			},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in updateProfileArgs
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			return h.updateProfile(ctx, userID, in)
		},
	}
}

func (h *assistantHandlers) updateProfile(ctx context.Context, userID int64, in updateProfileArgs) (any, error) {
	if h.profile == nil || h.profile.userProfile == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "the saved profile is not available")
	}
	if in.Skills != nil && in.AddSkills != nil {
		return nil, errors.New("send either skills (replace) or add_skills (add), not both")
	}
	if in.Specializations == nil && in.Skills == nil && in.AddSkills == nil && in.ExcludedSkills == nil &&
		in.Seniorities == nil && in.JobLanguages == nil && in.LocationPreferences == nil {
		return nil, errors.New("nothing to change: send at least one field")
	}

	cur, err := h.profile.userProfile.Get(ctx, userID)
	if err != nil && !errors.Is(err, userprofile.ErrNotFound) {
		return nil, err
	}

	loc := in.LocationPreferences
	if loc == nil && len(cur.LocationPreferences) > 0 {
		loc = &userprofile.LocationPreferences{}
		if err := json.Unmarshal(cur.LocationPreferences, loc); err != nil {
			return nil, err
		}
	}

	var unknown []string
	skills := cur.Skills
	switch {
	case in.Skills != nil:
		skills = dictionarySkills(in.Skills, &unknown)
	case in.AddSkills != nil:
		skills = append(slices.Clone(cur.Skills), dictionarySkills(in.AddSkills, &unknown)...)
	}
	excluded := cur.ExcludedSkills
	if in.ExcludedSkills != nil {
		excluded = dictionarySkills(in.ExcludedSkills, &unknown)
	}

	saved, err := h.profile.userProfile.Save(ctx, userID,
		pick(in.Specializations, cur.Specializations), skills, pick(in.Seniorities, cur.Seniorities),
		excluded, cur.ExcludedSources, cur.ExcludedCompanies, loc, pick(in.JobLanguages, cur.JobLanguages)...)
	if err != nil {
		return nil, profileError(err)
	}

	out := profileUpdateResult{
		Specializations: saved.Specializations, Skills: saved.Skills, ExcludedSkills: saved.ExcludedSkills,
		Seniorities: saved.Seniorities, JobLanguages: saved.JobLanguages, NotInDictionary: unknown,
	}
	if len(saved.LocationPreferences) > 0 {
		out.LocationPreferences = saved.LocationPreferences
	}
	if len(unknown) > 0 {
		out.Next = "The skills in not_in_dictionary were not saved. Offer to request them with suggest_dictionary_term."
	}
	return out, nil
}

// pick is the sent list when there is one, else the stored one.
func pick(sent, stored []string) []string {
	if sent != nil {
		return sent
	}
	return stored
}

// dictionarySkills keeps the skills the dictionary resolves, in their canonical form, and
// appends the ones it does not to unknown. Never a guess: an unresolved skill is dropped.
func dictionarySkills(values []string, unknown *[]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if c := skilltag.Canonicalize([]string{v}, skilltag.WithResumeAcronyms()); len(c) > 0 {
			out = append(out, c...)
		} else if strings.TrimSpace(v) != "" {
			*unknown = append(*unknown, v)
		}
	}
	return out
}
