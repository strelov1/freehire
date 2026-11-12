package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/resume"
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
	// Headline and Summary are the profile's own text, kept in resume.Owned (the block the
	// profile page writes through PUT /me/resume/contacts), not in the preferences row. A
	// pointer tells "omitted" from "" — an explicit empty string clears the field.
	Headline *string `json:"headline"`
	Summary  *string `json:"summary"`
}

func (in updateProfileArgs) hasPreferences() bool {
	return in.Specializations != nil || in.Skills != nil || in.AddSkills != nil || in.ExcludedSkills != nil ||
		in.Seniorities != nil || in.JobLanguages != nil || in.LocationPreferences != nil
}

func (in updateProfileArgs) hasText() bool { return in.Headline != nil || in.Summary != nil }

// preferenceFields and textFields name what a call asked to change, for the report of what
// was saved. Both stores can be written by one call and either can fail on its own.
func (in updateProfileArgs) preferenceFields() []string {
	var out []string
	for _, f := range []struct {
		name string
		sent bool
	}{
		{"specializations", in.Specializations != nil}, {"skills", in.Skills != nil},
		{"add_skills", in.AddSkills != nil}, {"excluded_skills", in.ExcludedSkills != nil},
		{"seniorities", in.Seniorities != nil}, {"job_languages", in.JobLanguages != nil},
		{"location_preferences", in.LocationPreferences != nil},
	} {
		if f.sent {
			out = append(out, f.name)
		}
	}
	return out
}

func (in updateProfileArgs) textFields() []string {
	var out []string
	if in.Headline != nil {
		out = append(out, "headline")
	}
	if in.Summary != nil {
		out = append(out, "summary")
	}
	return out
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
	// Headline and Summary are set only when the call wrote them: the text as saved.
	Headline *string `json:"headline,omitempty"`
	Summary  *string `json:"summary,omitempty"`
	// Truncated names text fields that were cut to the profile page's length limit.
	Truncated []string `json:"truncated,omitempty"`
	// Saved and NotSaved say exactly which of the fields in the call landed. The preferences
	// and the headline/summary live in two stores, so a call can half succeed; NotSaved and
	// Error appear only then.
	Saved    []string `json:"saved"`
	NotSaved []string `json:"not_saved,omitempty"`
	Error    string   `json:"error,omitempty"`
	Next     string   `json:"next,omitempty"`
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
		Description: "Change the user's saved profile: the preferences (roles, skills, seniority, languages, " +
			"location) and the headline and summary text shown on their profile page. This changes their " +
			"account, so say what you are about to change and get the user's agreement first. Call " +
			"get_profile first. Send only the fields that change; a field you leave out stays as it is, and " +
			"a list you send REPLACES the stored one (use add_skills to add without touching the rest). " +
			"Skills are matched against freehire's skill dictionary; any it does not know are NOT saved and " +
			"come back in not_in_dictionary — offer to call suggest_dictionary_term for those. Headline and " +
			"summary are cut to the profile page's length limits, and the result names any field that was cut " +
			"in truncated. The result lists which fields were saved and, if some failed, which were not. " +
			"This changes the profile, not a CV document: the documents sent to employers are edited with cv_edit.",
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
				"headline":        map[string]any{"type": "string", "description": "The profile headline, one line (for example a role and level). Sent as an empty string it is cleared."},
				"summary":         map[string]any{"type": "string", "description": "The profile summary, a short paragraph about who they are professionally. Sent as an empty string it is cleared."},
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

// updateProfileAttempts bounds the read-modify-write retry when a concurrent profile write
// (the profile page, an experience-bank skill merge) lands between our read and our write.
const updateProfileAttempts = 3

// updateProfile writes the preferences and the profile text. They are two stores with no
// shared transaction, so each is attempted on its own and the result lists what landed.
func (h *assistantHandlers) updateProfile(ctx context.Context, userID int64, in updateProfileArgs) (any, error) {
	if h.profile == nil || h.profile.userProfile == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "the saved profile is not available")
	}
	if in.Skills != nil && in.AddSkills != nil {
		return nil, fmt.Errorf("%w: send either skills (replace) or add_skills (add), not both", assistant.ErrInvalidArguments)
	}
	if !in.hasPreferences() && !in.hasText() {
		return nil, fmt.Errorf("%w: nothing to change, send at least one field", assistant.ErrInvalidArguments)
	}

	var (
		out             profileUpdateResult
		saved, notSaved []string
		failure         error
		prefsWritten    bool
	)
	if in.hasPreferences() {
		p, unknown, err := h.savePreferences(ctx, userID, in)
		if err != nil {
			notSaved, failure = append(notSaved, in.preferenceFields()...), err
		} else {
			saved, prefsWritten = append(saved, in.preferenceFields()...), true
			out = profileResult(p)
			out.NotInDictionary = unknown
		}
	}
	if in.hasText() {
		o, truncated, err := h.saveProfileText(ctx, userID, in)
		if err != nil {
			notSaved, failure = append(notSaved, in.textFields()...), errors.Join(failure, err)
		} else {
			saved = append(saved, in.textFields()...)
			out.Truncated = truncated
			if in.Headline != nil {
				out.Headline = &o.Headline
			}
			if in.Summary != nil {
				out.Summary = &o.Summary
			}
		}
	}
	if len(saved) == 0 {
		return nil, failure // nothing landed: a plain failed call
	}
	if !prefsWritten {
		// A text-only call still answers with the preferences as they stand.
		if cur, err := h.profile.userProfile.Get(ctx, userID); err == nil {
			view := profileResult(cur)
			view.Headline, view.Summary, view.Truncated = out.Headline, out.Summary, out.Truncated
			out = view
		}
	}
	out.Saved = saved
	if failure != nil {
		out.NotSaved, out.Error = notSaved, callerErrorText(failure)
		out.Next = "Only the fields in saved were changed; the ones in not_saved were not. Tell the user, and retry those if they still want them."
	} else if len(out.NotInDictionary) > 0 {
		out.Next = "The skills in not_in_dictionary were not saved. Offer to request them with suggest_dictionary_term."
	}
	return out, nil
}

func profileResult(p userprofile.Profile) profileUpdateResult {
	out := profileUpdateResult{
		Specializations: p.Specializations, Skills: p.Skills, ExcludedSkills: p.ExcludedSkills,
		Seniorities: p.Seniorities, JobLanguages: p.JobLanguages,
	}
	if len(p.LocationPreferences) > 0 {
		out.LocationPreferences = p.LocationPreferences
	}
	return out
}

// callerErrorText is the part of an error the agent may be told: a fiber error carries a
// message written for the caller; anything else is an internal fault whose text stays out.
func callerErrorText(err error) string {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Message
	}
	return "the save failed on freehire's side"
}

// savePreferences is the guarded read-modify-write of the preferences row, retried when a
// concurrent write moves updated_at between the read and the write.
func (h *assistantHandlers) savePreferences(ctx context.Context, userID int64, in updateProfileArgs) (userprofile.Profile, []string, error) {
	for attempt := 1; ; attempt++ {
		p, unknown, err := h.savePreferencesOnce(ctx, userID, in)
		if !errors.Is(err, userprofile.ErrConflict) || attempt == updateProfileAttempts {
			return p, unknown, err
		}
	}
}

func (h *assistantHandlers) savePreferencesOnce(ctx context.Context, userID int64, in updateProfileArgs) (userprofile.Profile, []string, error) {
	cur, err := h.profile.userProfile.Get(ctx, userID)
	if err != nil && !errors.Is(err, userprofile.ErrNotFound) {
		return userprofile.Profile{}, nil, err
	}

	loc := in.LocationPreferences
	if loc == nil && len(cur.LocationPreferences) > 0 {
		loc = &userprofile.LocationPreferences{}
		if err := json.Unmarshal(cur.LocationPreferences, loc); err != nil {
			return userprofile.Profile{}, nil, err
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

	specs, levels, languages := pick(in.Specializations, cur.Specializations),
		pick(in.Seniorities, cur.Seniorities), pick(in.JobLanguages, cur.JobLanguages)
	var saved userprofile.Profile
	if cur.UpdatedAt == nil {
		// Another writer may have created the profile since our missing-row read.
		saved, err = h.profile.userProfile.CreateIfAbsent(ctx, userID, specs, skills, levels,
			excluded, cur.ExcludedSources, cur.ExcludedCompanies, loc, languages...)
	} else {
		saved, err = h.profile.userProfile.SaveIfUnchanged(ctx, userID, *cur.UpdatedAt, specs, skills, levels,
			excluded, cur.ExcludedSources, cur.ExcludedCompanies, loc, languages...)
	}
	if errors.Is(err, userprofile.ErrConflict) {
		return userprofile.Profile{}, nil, err
	}
	if err != nil {
		return userprofile.Profile{}, nil, profileError(err)
	}
	return saved, unknown, nil
}

// saveProfileText sets the headline and/or summary the way the profile page does: read the
// candidate's resume.Owned block, change those two fields, and write the block back through
// SetCandidateOwned, which applies the page's length limits. Email, phone, links and every
// other Owned field go back exactly as read. The *Set flag is what lets an empty string
// stand as an explicit clear.
//
// shortcut: the write is not version-guarded, same as PUT /me/resume/contacts; a page save
// landing between the read and the write is overwritten. Upgrade with a guarded write in
// resume.Store if this is ever seen.
func (h *assistantHandlers) saveProfileText(ctx context.Context, userID int64, in updateProfileArgs) (resume.Owned, []string, error) {
	if h.profile.resume == nil {
		return resume.Owned{}, nil, fiber.NewError(fiber.StatusServiceUnavailable, "the profile text is not available")
	}
	owned, err := h.profile.resume.CandidateOwned(ctx, userID)
	if err != nil {
		return resume.Owned{}, nil, err
	}
	if in.Headline != nil {
		owned.Headline, owned.HeadlineSet = *in.Headline, true
	}
	if in.Summary != nil {
		owned.Summary, owned.SummarySet = *in.Summary, true
	}
	saved, err := h.profile.resume.SetCandidateOwned(ctx, userID, owned)
	if err != nil {
		return resume.Owned{}, nil, err
	}
	var truncated []string
	if in.Headline != nil && saved.Headline != strings.TrimSpace(*in.Headline) {
		truncated = append(truncated, "headline")
	}
	if in.Summary != nil && saved.Summary != strings.TrimSpace(*in.Summary) {
		truncated = append(truncated, "summary")
	}
	return saved, truncated, nil
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
