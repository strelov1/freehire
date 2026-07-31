package cvmatch

// Input is everything the scorer reads. It is plain data: rendering the CV, extracting its
// text layer, tagging its skills and reading the cached requirement ledger all happen in
// the caller, which keeps this package pure, I/O-free and testable without a toolchain.
type Input struct {
	// CVText is the text layer of the tailored CV rendered to PDF — the artifact a parser
	// reads, not the document we meant to render.
	CVText string
	// CVSkills are the canonical skills parsed from that same text, so the score describes
	// the artifact and nothing outside it.
	CVSkills []string

	JobTitle string
	// JobSkills are the vacancy's canonical skills. They are both the keyword baseline and
	// the only set a requirement may be attributed a skill from.
	JobSkills []string

	// Requirements is the cached fit analysis's requirement ledger. HasAnalysis tells an
	// empty ledger apart from an absent one: a vacancy can genuinely state no requirements,
	// and that is not the same as never having been analysed.
	Requirements []Requirement
	HasAnalysis  bool
}

// Compute scores a tailored CV against the vacancy it was written for.
//
// Deterministic, pure and free: no model is called, so the caller may recompute this as
// often as the document changes. Categories are emitted heaviest-first and always all four,
// including the ones that could not be evaluated — a category the candidate can see is
// unavailable, with a reason, is worth more than a row that silently vanished.
func Compute(in Input) Score {
	requirements, checks := requirementsCategory(in.Requirements, in.HasAnalysis, in.CVSkills, in.JobSkills)
	keyword, matched, missing := keywordCategory(in.CVSkills, in.JobSkills)

	s := aggregate([]ScoredCategory{
		requirements,
		keyword,
		titleCategory(in.JobTitle, in.CVText),
		seniorityCategory(in.JobTitle, in.CVText),
	})
	s.MatchedSkills = matched
	s.MissingSkills = missing
	s.Requirements = checks
	return s
}
