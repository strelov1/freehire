package resumeextract

import "strings"

// This file holds the Talent Network public-profile projections (see
// openspec/changes/talent-network-profile-visibility). It is deliberately NOT part of
// structured.go: cmd/gen-contracts generates TypeScript from structured.go alone, and
// these projections are pure Go-side view logic with no wire contract of their own yet
// — the handler that serves them (a later task) decides what crosses the wire and gets
// its own generated type then, rather than this file leaking one prematurely.

// currentEmployerLabel is the generic label anonymous mode substitutes for a "current"
// experience entry's employer name. This is the literal wording from design.md's
// "Anonymous-mode masking is Professional() plus one extra step" decision — not a
// placeholder to be reworded later.
const currentEmployerLabel = "Current employer"

// notEndedLabels mirrors internal/experience/period_sort.go's isPresentLabel exactly
// (plus the empty string, which import_resume.go treats as "not ended" too): the
// free-form End labels a CV uses for a role that has not ended. Kept as a separate
// copy rather than imported — internal/experience already imports this package, so the
// reverse import would be circular. Keep the two in sync if the convention ever changes.
var notEndedLabels = map[string]bool{"": true, "present": true, "current": true, "now": true, "ongoing": true, "today": true}

// isCurrentEntry reports whether an experience entry's End label reads as "not ended",
// case-insensitively, per notEndedLabels above.
func isCurrentEntry(e Experience) bool {
	return notEndedLabels[strings.ToLower(strings.TrimSpace(e.End))]
}

// stripProjectLinks blanks Link on a copy of every project entry, leaving name and
// highlights untouched — a project's Link (often "github.com/<handle>" or a personal
// portfolio domain) is a stronger de-anonymizing identifier than the name Professional()
// already strips, so both public projections below must not carry it either. Masking
// only the field, not the whole entry, mirrors Anonymous' own treatment of a masked
// experience entry's employer: the rest of the project (name, highlights) is legitimate
// signal.
//
// Copy before mutating: Professional's Projects slice shares its backing array with
// s.Projects, and this must not mutate the caller's Structured (same concern Anonymous
// already documents for Experience).
func stripProjectLinks(projects []Project) []Project {
	if len(projects) == 0 {
		return projects
	}
	stripped := make([]Project, len(projects))
	copy(stripped, projects)
	for i := range stripped {
		stripped[i].Link = ""
	}
	return stripped
}

// Anonymous is Structured's anonymous-mode projection for the Talent Network public
// page: Professional() (already contact-free — no name/email/phone/links) with every
// "current" experience entry's employer replaced by the generic label above, and every
// project's Link stripped (stripProjectLinks — a project link is a stronger
// de-anonymizing identifier than the name Professional() already withholds). All other
// entries pass through unmodified: the design deliberately scopes the mask to the
// current role(s), not the whole work history, since the rest of the history is useful
// signal for anyone else the candidate shares the link with (see design.md, "Risks /
// Trade-offs").
//
// The mask is content-based (the End label), not positional: Structured.Experience's
// ordering (newest-first vs. oldest-first) is nowhere documented or enforced by the LLM
// extraction prompt or schema, so "the newest entry" cannot be determined reliably by
// array position alone (see design.md, "Masking is content-based (the End label), not
// positional."). If more than one entry reads as current (concurrent roles, or a
// sloppily-filled CV), every matching entry is masked — there is no reliable signal for
// picking "the real" one. If zero entries read as current, none are masked.
func (s Structured) Anonymous() Professional {
	p := s.Professional()
	p.Projects = stripProjectLinks(p.Projects)

	if len(p.Experience) == 0 {
		return p
	}

	// Copy before mutating: Professional's Experience slice shares its backing array
	// with s.Experience, and masking must not leak back into the caller's Structured.
	masked := make([]Experience, len(p.Experience))
	copy(masked, p.Experience)
	for i := range masked {
		if isCurrentEntry(masked[i]) {
			masked[i].Company = currentEmployerLabel
		}
	}
	p.Experience = masked

	return p
}

// Public is Structured's public-mode projection: the contact-free Professional fields
// plus the candidate's name. Structured carries no photo field today, so a public-mode
// photo (e.g. users.photo_object_key) is a separate seam a later task composes in from
// elsewhere — nothing here invents one.
type Public struct {
	FullName string `json:"full_name,omitempty"`
	Professional
}

// Public projects Structured onto the public-mode view: name shown, work history and
// skills shown unmodified (including any current employer — unlike Anonymous, which
// masks it), contact fields still withheld. Public mode reuses the same contact-stripped
// base as anonymous mode because the page is unauthenticated and publicly reachable (see
// design.md, "Public mode still strips contact info"). Every project's Link is stripped
// (stripProjectLinks) for the same reason contact fields are: the page is scrapeable by
// definition, and a project link is a personal URL regardless of whether the candidate's
// name is otherwise shown.
func (s Structured) Public() Public {
	p := s.Professional()
	p.Projects = stripProjectLinks(p.Projects)
	return Public{
		FullName:     s.FullName,
		Professional: p,
	}
}
