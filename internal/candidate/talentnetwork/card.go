package talentnetwork

import (
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/certification"
	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/edulevel"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// The public catalogue card, and the one rule that makes it safe:
//
//	every string it carries is a value a DICTIONARY resolved, or a date. Never a
//	string the candidate typed, with ONE exception: CandidateRole.Title.
//
// The exception: a work history reduced to "Senior · Backend" per row loses what a reader
// came for, and a title the dictionary cannot place ("Technical Lead | Team Lead") reduces
// to nothing. The cost is an employer's name riding along inside the title. Scrubbing was
// declined: separators like "|", " - ", "@" sit in ordinary titles far more often than
// beside an employer, so cut-on-symbol mangles the common case, and cut-on-known-company
// misses spellings like "embedded at League Inc.". So the title is published verbatim, the
// candidate is told where they decide, and the exception stays to this ONE field.
//
// This is stronger than "mask the fields that identify somebody", which is why this type
// exists instead of reusing resumeextract.Anonymous: that replaces only the current role's
// `company`, but an employer's name also sits in prose ("at <employer> I rebuilt..."), job
// titles, project names and the institution line. Masking field by field would publish any
// field the extraction contract later grows by default; a whitelist of dictionary-resolved
// values fails the other way (something is missing until someone adds it).
//
// Cost: languages have no dictionary this block can reach, so they are absent. Education
// carries only the degree LEVEL (internal/dict/edulevel) and year, and certifications only
// canonical names (internal/dict/certification); institution, field of study, issuer and
// certification dates are free text a dictionary cannot vouch for.

// CandidateCard is one candidate as the public catalogue shows them.
type CandidateCard struct {
	// Seniority and Category describe the candidate as a whole: what their current (or
	// most recent) role resolves to. The heading is built, not quoted; see PrimaryTitle.
	Seniority string `json:"seniority,omitempty"`
	Category  string `json:"category,omitempty"`

	// TotalYears is the CV's own figure. A number cannot carry a name.
	TotalYears int `json:"total_years,omitempty"`

	// Skills are skilltag canonicals; an unresolved token emits nothing.
	Skills []string `json:"skills"`

	// Roles is the work history with everything nameable removed: no employer, location or prose.
	Roles []CandidateRole `json:"roles"`

	// Education carries only what internal/dict/edulevel resolves from each entry's degree
	// text, plus its year. An entry whose degree resolves to nothing is dropped, not kept
	// under an empty label (unlike roles, nobody expects a complete degree list).
	Education []EducationEntry `json:"education,omitempty"`

	// Certifications are internal/dict/certification canonicals; unresolved names emit nothing.
	Certifications []string `json:"certifications,omitempty"`
}

// EducationEntry is one education item, reduced to what a dictionary can vouch for:
// the degree's level and the year, never the institution.
type EducationEntry struct {
	Level string                 `json:"level,omitempty"`
	Year  *perioddate.PeriodDate `json:"year,omitempty"`
}

// CandidateRole is one position: what it was, when, and what it was built with.
type CandidateRole struct {
	// Title is the candidate's own words, the card's ONE exception to the dictionary-only
	// rule (see the header). Seniority and Category are the fallback for an absent title
	// and what every filter reads; the raw string is never matched on.
	Title string `json:"title,omitempty"`

	Seniority string `json:"seniority,omitempty"`
	Category  string `json:"category,omitempty"`

	Start   *perioddate.PeriodDate `json:"start,omitempty"`
	End     *perioddate.PeriodDate `json:"end,omitempty"`
	Current bool                   `json:"current,omitempty"`

	Stack []string `json:"stack,omitempty"`
}

// ProjectCard projects a stored CV onto the public catalogue card.
//
// It returns only what a dictionary could vouch for. A role whose title resolves to
// nothing is KEPT with empty seniority and category: dropping it would shorten the work
// history, and a gap reads worse than an unlabelled job.
func ProjectCard(s resumeextract.Structured) CandidateCard {
	primary := classify.Parse(PrimaryTitle(s))
	return CandidateCard{
		Seniority:      primary.Seniority,
		Category:       primary.Category,
		TotalYears:     s.TotalYears,
		Skills:         canonicalSkills(s.Skills),
		Roles:          cardRoles(s.Experience),
		Education:      cardEducation(s.Education),
		Certifications: certification.Canonicalize(s.Certifications),
	}
}

// cardEducation resolves each degree via edulevel.ForDegree, dropping unresolved entries
// (see CandidateCard.Education).
func cardEducation(education []resumeextract.Education) []EducationEntry {
	if len(education) == 0 {
		return nil
	}
	entries := make([]EducationEntry, 0, len(education))
	for _, e := range education {
		level := edulevel.ForDegree(e.Degree)
		if level == "" {
			continue
		}
		entries = append(entries, EducationEntry{Level: level, Year: e.Year})
	}
	return entries
}

func cardRoles(experience []resumeextract.Experience) []CandidateRole {
	if len(experience) == 0 {
		// Never nil: Roles has no `omitempty`, and a nil slice marshals to JSON `null`,
		// which the frontend readers (card.roles.length) crash on.
		return []CandidateRole{}
	}
	roles := make([]CandidateRole, 0, len(experience))
	for _, e := range experience {
		c := classify.Parse(e.Title)
		roles = append(roles, CandidateRole{
			Title:     e.Title,
			Seniority: c.Seniority,
			Category:  c.Category,
			Start:     e.Start,
			End:       e.End,
			// Same as rankOf: an unset end means ongoing under the extraction contract.
			Current: e.Current || e.End == nil,
			Stack:   canonicalSkills(e.Stack),
		})
	}
	return roles
}

// canonicalSkills resolves free tokens to skilltag canonicals, dropping what it cannot
// place. WithResumeAcronyms is on because these tokens are the candidate's own claims;
// cvmatch, which mines a vacancy's prose, must not read résumé-only acronyms.
func canonicalSkills(tokens []string) []string {
	// Never nil (see cardRoles). Canonicalize returns nil whenever nothing resolves, not
	// only for empty input, so the coalesce must cover the result.
	if resolved := skilltag.Canonicalize(tokens, skilltag.WithResumeAcronyms()); resolved != nil {
		return resolved
	}
	return []string{}
}

// CatalogueMember is one entry in the public catalogue: the dictionary-checked card, plus
// the facts that live in columns rather than in the CV.
//
// Every field obeys the ProjectCard rule: cities are the NORMALISED extraction
// (users.resume_cities), not the free-text CV location; the timezone is an IANA zone;
// specializations come from vocab.CategoryValues.
//
// Specializations are NOT a duplicate of Card.Category: Category is what the candidate has
// DONE (latest title through classify), Specializations what they SAY they want. A backend
// engineer who ticks `ml_ai` is findable only because the two can disagree.
type CatalogueMember struct {
	Handle string        `json:"handle"`
	Card   CandidateCard `json:"card"`

	// Timezone is the IANA zone; TimezoneRegion is the part before the slash, which is what
	// the filter reads (dozens of zones per continent).
	Timezone       string `json:"timezone,omitempty"`
	TimezoneRegion string `json:"timezone_region,omitempty"`

	Cities          []string `json:"cities"`
	Specializations []string `json:"specializations"`

	// UpdatedAt is when the structured extract was written; it orders the list.
	UpdatedAt time.Time `json:"updated_at"`

	// HasPhoto says only whether GET /talent/{handle}/photo has something to serve, never
	// the object key. A boolean carries no name, and it saves list cards a wasted 404 request.
	HasPhoto bool `json:"has_photo"`
}
