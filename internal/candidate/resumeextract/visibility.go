package resumeextract

// This file holds the Talent Network public-profile projections. It is NOT part of
// structured.go because cmd/gen-contracts generates TypeScript from that file alone and
// these projections are Go-side view logic with no wire contract.

// currentEmployerLabel replaces the employer name of a "current" experience entry in
// anonymous mode.
const currentEmployerLabel = "Current employer"

// isCurrentEntry reports whether an experience entry is ongoing. A nil End counts too
// (the systemPrompt defines it so): a fail-closed guard against a model that returns
// end:null but forgets current:true, since under-masking a current employer is the leak
// this feature exists to prevent.
func isCurrentEntry(e Experience) bool {
	return e.Current || e.End == nil
}

// stripProjectLinks blanks Link on a copy of every project entry. A link (often
// "github.com/<handle>" or a portfolio domain) identifies the candidate more strongly
// than the name, so public projections must not carry it; name and highlights stay.
//
// It copies because Professional's Projects slice shares its backing array with s.Projects.
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

// Anonymous is the anonymous-mode projection for the Talent Network public page:
// Professional() with every current entry's employer replaced by currentEmployerLabel
// and every project Link stripped. The rest of the history passes through as useful signal.
//
// The mask is content-based, not positional: Experience ordering is neither documented
// nor enforced, so "the newest entry" cannot be found by index. Every entry that reads as
// current is masked; if none does, none is.
func (s Structured) Anonymous() Professional {
	p := s.Professional()
	p.Projects = stripProjectLinks(p.Projects)

	if len(p.Experience) == 0 {
		return p
	}

	// Copy: Professional's Experience shares its backing array with s.Experience.
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

// There is deliberately no name-bearing public projection: no caller may select one, and
// an unreachable one is a careless call away from publishing a name.
