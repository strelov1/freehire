// Package cvmatch scores a tailored CV against the vacancy it was written for, and
// against nothing else. It reads the document's rendered text and the vacancy; it never
// reads the base CV, the experience bank or the candidate's structured résumé — those
// describe the candidate, and this score describes the document.
//
// It is deterministic and dictionary-only. Where a dictionary resolves nothing the answer
// is "unverifiable", never a guess and never a zero: an input we cannot check is not a
// failure the candidate caused. That rule is the whole design, and it is why this package
// is a sibling of internal/atscheck rather than a sixth category inside it — every atscheck
// category is computable from any text, and none of these four are.
package cvmatch

// Coverage is a requirement's re-derived outcome. Unverifiable is not a middle grade
// between the other two: it means the requirement named no canonical skill, so the
// document cannot be checked against it either way.
type Coverage string

const (
	Covered      Coverage = "covered"
	Missing      Coverage = "missing"
	Unverifiable Coverage = "unverifiable"
)

// Category IDs. Exported: the SPA keys its rows by them.
const (
	CategoryRequirements = "requirements_coverage"
	CategoryKeyword      = "keyword_match"
	CategoryTitle        = "job_title_match"
	CategorySeniority    = "seniority_fit"
)

// Category weights, out of 100. They travel in the response rather than being re-declared
// by the client, so a re-weighting is a server change and the panel's impact labelling can
// never disagree with the score it labels.
const (
	WeightRequirements = 40
	WeightKeyword      = 30
	WeightTitle        = 20
	WeightSeniority    = 10
)

// ScoredCategory is one weighted scoring dimension. An unavailable category carries a reason and
// no items, and is excluded from both sides of the overall — see Score.
type ScoredCategory struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	Earned    int        `json:"earned"`
	Weight    int        `json:"weight"`
	Available bool       `json:"available"`
	Reason    string     `json:"reason,omitempty"`
	Items     []LineItem `json:"items,omitempty"`
}

// RequirementCheck is one posting requirement re-derived against the current document.
// Text and Priority come from the cached fit analysis — they describe the posting and do
// not depend on any CV. Coverage does not: it is recomputed here, because the cached
// status was determined against the candidate's base profile.
type RequirementCheck struct {
	Text     string   `json:"text"`
	Priority string   `json:"priority"` // required | preferred
	Coverage Coverage `json:"coverage"`
	// Skills are the canonical skills the requirement's own text yielded. Empty means the
	// requirement is unverifiable.
	Skills []string `json:"skills,omitempty"`
	// Missing are the Skills the document does not contain.
	Missing []string `json:"missing,omitempty"`
	// CachedStatus is the status the earlier analysis recorded, carried ONLY for an
	// unverifiable requirement so the panel can show it labelled as coming from that
	// analysis. A re-derived requirement never carries it: a stale verdict beside a live
	// one is worse than no verdict.
	CachedStatus string `json:"cached_status,omitempty"`
}

// Score is the full job-match result.
type Score struct {
	Overall    int              `json:"overall"`
	Categories []ScoredCategory `json:"categories"`
	// Contributing names the categories the overall was computed over, so a score taken
	// across three categories is never mistaken for one taken across four.
	Contributing  []string           `json:"contributing"`
	MatchedSkills []string           `json:"matched_skills,omitempty"`
	MissingSkills []string           `json:"missing_skills,omitempty"`
	Requirements  []RequirementCheck `json:"requirements,omitempty"`
}
