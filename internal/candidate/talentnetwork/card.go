package talentnetwork

import (
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// The public catalogue card, and the one rule that makes it safe:
//
//	every string it carries is a value a DICTIONARY resolved, or a date. Never a
//	string the candidate typed.
//
// That is a stronger rule than "mask the fields that identify somebody", and the
// difference is the whole reason this type exists rather than reusing
// resumeextract.Anonymous. Anonymous replaces the current role's `company` column and
// leaves everything else alone, which is right for a page a candidate hands to one
// person and wrong for a page a crawler reads: an employer's name is usually sitting
// in the prose beside the column that was masked ("at <employer> I rebuilt the billing
// pipeline"), in the job title ("Backend Engineer @ <employer>"), in a project's name,
// or in the institution line.
//
// Masking those one by one would work until the extraction contract grows a field, at
// which point the new one is published by default. A whitelist of dictionary-resolved
// values fails the other way: something we should have shown is missing, and somebody
// notices and adds it.
//
// What that costs, stated plainly: languages, certifications and education carry no
// dictionary this block can reach, so they are absent from v1. vocab.EducationLevelValues
// exists but the text→level resolver lives in internal/job/jobfacts — block `job`,
// layer 5 — which `candidate` may not import. Serving them means first moving that
// dictionary down into `dict`, which is a change of its own; the fuller picture is what
// the approved-recruiter tier will carry.

// CandidateCard is one candidate as the public catalogue shows them.
type CandidateCard struct {
	// Seniority and Category describe the candidate as a whole: what their current (or
	// most recent) role resolves to. This is the card's heading, built rather than
	// quoted — see PrimaryTitle.
	Seniority string `json:"seniority,omitempty"`
	Category  string `json:"category,omitempty"`

	// TotalYears is the CV's own figure. A number cannot carry a name.
	TotalYears int `json:"total_years,omitempty"`

	// Skills are skilltag canonicals. A token the dictionary does not resolve emits
	// nothing, which is exactly the whitelisting this card needs — see skilltag's
	// "never guess" rule.
	Skills []string `json:"skills"`

	// Roles is the work history with everything nameable removed: no employer, no
	// location, no prose. What remains is the shape of a career, which is what a
	// recruiter reads a history for anyway.
	Roles []CandidateRole `json:"roles"`
}

// CandidateRole is one position: what it was, when, and what it was built with.
type CandidateRole struct {
	Seniority string `json:"seniority,omitempty"`
	Category  string `json:"category,omitempty"`

	Start   *perioddate.PeriodDate `json:"start,omitempty"`
	End     *perioddate.PeriodDate `json:"end,omitempty"`
	Current bool                   `json:"current,omitempty"`

	Stack []string `json:"stack,omitempty"`
}

// ProjectCard projects a stored CV onto the public catalogue card.
//
// It reads the whole Structured and returns only what a dictionary could vouch for. A
// role whose title resolves to nothing is KEPT, carrying its period and stack under
// empty seniority and category: dropping it would make a work history look shorter than
// it is, and a gap in a career reads worse than an unlabelled job.
func ProjectCard(s resumeextract.Structured) CandidateCard {
	primary := classify.Parse(PrimaryTitle(s))
	return CandidateCard{
		Seniority:  primary.Seniority,
		Category:   primary.Category,
		TotalYears: s.TotalYears,
		Skills:     canonicalSkills(s.Skills),
		Roles:      cardRoles(s.Experience),
	}
}

func cardRoles(experience []resumeextract.Experience) []CandidateRole {
	if len(experience) == 0 {
		return nil
	}
	roles := make([]CandidateRole, 0, len(experience))
	for _, e := range experience {
		c := classify.Parse(e.Title)
		roles = append(roles, CandidateRole{
			Seniority: c.Seniority,
			Category:  c.Category,
			Start:     e.Start,
			End:       e.End,
			// Reported the way rankOf reads it: an unset end means ongoing under the
			// extraction contract, so a role with neither flag nor end date is shown as
			// current rather than as one that ended at an unknown time.
			Current: e.Current || e.End == nil,
			Stack:   canonicalSkills(e.Stack),
		})
	}
	return roles
}

// canonicalSkills resolves free tokens to skilltag canonicals, dropping what it cannot
// place. WithResumeAcronyms is on because these tokens ARE the candidate's own claims
// about themselves — the tier exists for exactly this side of the match, unlike cvmatch,
// which mines a vacancy's prose and must not read a résumé-only acronym into it.
func canonicalSkills(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	return skilltag.Canonicalize(tokens, skilltag.WithResumeAcronyms())
}

// CatalogueMember is one entry in the public catalogue: the dictionary-checked card, plus the
// facts that live in columns rather than in the CV.
//
// Every field here survives the same rule ProjectCard enforces. Cities are the NORMALISED
// extraction (users.resume_cities), not the free-text location inside the CV, which
// carries values like "Austria, Klagenfurt 9020"; specializations are facets the
// candidate picked from a closed list; the timezone is an IANA zone. None of them can
// carry a sentence.
type CatalogueMember struct {
	Handle string        `json:"handle"`
	Card   CandidateCard `json:"card"`

	// Timezone is the IANA zone; TimezoneRegion is the part before the slash. The region
	// is what a recruiter asking "can we overlap for a call" actually means — there are
	// dozens of zones per continent — so it is what the filter reads.
	Timezone       string `json:"timezone,omitempty"`
	TimezoneRegion string `json:"timezone_region,omitempty"`

	Cities          []string `json:"cities"`
	Specializations []string `json:"specializations"`

	// UpdatedAt is when the structured extract was written, which is the freshest thing
	// the catalogue knows about a member. It orders the list.
	UpdatedAt time.Time `json:"updated_at"`
}
