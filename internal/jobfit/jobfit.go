// Package jobfit computes an on-demand, LLM-driven fit analysis for a single
// (candidate, job) pair: a five-dimension scored verdict centered on job-title
// alignment and relevant experience, plus an ATS-style requirement-match table.
//
// It runs a fixed three-stage prompt-chain (see analyzer.go) — Extract & Match (the
// ATS lens) → Recruiter verdict (the human lens) → Adversarial audit — over the
// provider-agnostic internal/llm client, NOT an autonomous agent: the step count and
// order are fixed and every input is gathered before the first call. This file owns
// the wire contract, the untrusted-output sanitize, and the deterministic weighted
// scoring; the model only scores the five dimensions, while overall_score and the
// verdict label are computed here so the headline number stays consistent with the
// dimensions. Pure and I/O-free (mirrors internal/atscheck / internal/verdict).
package jobfit

import (
	"math"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// Canonical dimension keys, in the fixed display/scoring order. The model returns
// the six scores by name; the wire Dimensions slice is built here so a dropped,
// reordered, or injected dimension can never reach the response.
const (
	DimTitleAlignment      = "title_alignment"
	DimExperienceRelevance = "experience_relevance"
	DimSeniorityFit        = "seniority_fit"
	DimSkillsCoverage      = "skills_coverage"
	DimCompanyContext      = "company_context"
	DimLocationFit         = "location_fit"
)

// Verdict labels (server-derived from overall_score — never taken from the model).
const (
	VerdictStrong   = "Strong Fit"
	VerdictGood     = "Good Fit"
	VerdictModerate = "Moderate Fit"
	VerdictWeak     = "Weak Fit"
	VerdictPoor     = "Poor Fit"
)

// Requirement priorities and match statuses (the ATS lens vocabulary).
const (
	PriorityRequired  = "required"
	PriorityPreferred = "preferred"

	StatusCovered     = "covered"      // present in the CV (verbatim or trivial inflection)
	StatusSynonymOnly = "synonym-only" // the concept is present under a different term
	StatusMissingHave = "missing-have" // profile evidences it but the CV never states it
	StatusMissingGap  = "missing-gap"  // a genuine gap — absent, no close equivalent held
)

// dimensionSpec pins each dimension's label and its weight (percent). Title alignment
// and experience relevance carry the most weight — the two signals an ATS keyword
// screen and a recruiter weigh most. The weights sum to 100.
type dimensionSpec struct {
	key    string
	label  string
	weight int
}

var dimensionSpecs = []dimensionSpec{
	{DimTitleAlignment, "Title & role alignment", 20},
	{DimExperienceRelevance, "Experience relevance", 25},
	{DimSeniorityFit, "Seniority fit", 15},
	{DimSkillsCoverage, "Skills coverage", 15},
	{DimCompanyContext, "Company & role context", 10},
	{DimLocationFit, "Location & work-mode fit", 15},
}

// Verdict thresholds (inclusive lower bounds), adapted from the reference framework.
const (
	thresholdStrong   = 75
	thresholdGood     = 60
	thresholdModerate = 45
	thresholdWeak     = 30
)

// Output bounds for untrusted model text (mirrors atscheck's caps).
const (
	maxCommentRunes     = 240
	maxListItemRunes    = 200
	maxRecommendRunes   = 400
	maxReqTextRunes     = 200
	maxReqEvidenceRunes = 240
	maxStrengths        = 6
	maxGaps             = 6
	maxRequirements     = 30
)

// Dimension is one scored fit dimension on the wire.
type Dimension struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Score   int    `json:"score"` // 0-100
	Comment string `json:"comment"`
}

// Requirement is one vacancy requirement classified against the CV (the ATS lens).
type Requirement struct {
	Text     string `json:"text"`
	Priority string `json:"priority"` // required | preferred
	Status   string `json:"status"`   // covered | synonym-only | missing-have | missing-gap
	Evidence string `json:"evidence"` // where it appears in the CV, or why it is absent
}

// Analysis is the full served fit verdict — the single wire contract exported to TS
// via cmd/gen-contracts.
type Analysis struct {
	Dimensions       []Dimension   `json:"dimensions"`
	RequirementMatch []Requirement `json:"requirement_match"`
	OverallScore     int           `json:"overall_score"`
	Verdict          string        `json:"verdict"`
	Strengths        []string      `json:"strengths"`
	Gaps             []string      `json:"gaps"`
	Recommendation   string        `json:"recommendation"`
}

// dimScore is one dimension as the model returns it (score + short rationale).
type dimScore struct {
	Score   int    `json:"score"`
	Comment string `json:"comment"`
}

// recruiterVerdict is the recruiter/audit stage output: the six scores by name plus
// the free-text verdict fields. The named fields (not a free array) keep the weighted
// overall deterministic even when the model is internally inconsistent.
type recruiterVerdict struct {
	TitleAlignment      dimScore `json:"title_alignment"`
	ExperienceRelevance dimScore `json:"experience_relevance"`
	SeniorityFit        dimScore `json:"seniority_fit"`
	SkillsCoverage      dimScore `json:"skills_coverage"`
	CompanyContext      dimScore `json:"company_context"`
	LocationFit         dimScore `json:"location_fit"`
	Strengths           []string `json:"strengths"`
	Gaps                []string `json:"gaps"`
	Recommendation      string   `json:"recommendation"`
}

// buildAnalysis assembles the served Analysis from the (sanitized) requirement match
// and recruiter verdict: the five dimensions in fixed order, the weighted overall, and
// the derived verdict label.
func buildAnalysis(reqs []Requirement, v recruiterVerdict) Analysis {
	scores := map[string]dimScore{
		DimTitleAlignment:      v.TitleAlignment,
		DimExperienceRelevance: v.ExperienceRelevance,
		DimSeniorityFit:        v.SeniorityFit,
		DimSkillsCoverage:      v.SkillsCoverage,
		DimCompanyContext:      v.CompanyContext,
		DimLocationFit:         v.LocationFit,
	}
	dims := make([]Dimension, 0, len(dimensionSpecs))
	weighted := 0.0
	for _, spec := range dimensionSpecs {
		ds := scores[spec.key]
		dims = append(dims, Dimension{Key: spec.key, Label: spec.label, Score: ds.Score, Comment: ds.Comment})
		weighted += float64(ds.Score) * float64(spec.weight) / 100
	}
	overall := clamp(int(math.Round(weighted)))
	if reqs == nil {
		reqs = []Requirement{}
	}
	strengths := v.Strengths
	if strengths == nil {
		strengths = []string{}
	}
	gaps := v.Gaps
	if gaps == nil {
		gaps = []string{}
	}
	return Analysis{
		Dimensions:       dims,
		RequirementMatch: reqs,
		OverallScore:     overall,
		Verdict:          verdictFor(overall),
		Strengths:        strengths,
		Gaps:             gaps,
		Recommendation:   v.Recommendation,
	}
}

// verdictFor maps an overall score to its label by inclusive threshold.
func verdictFor(overall int) string {
	switch {
	case overall >= thresholdStrong:
		return VerdictStrong
	case overall >= thresholdGood:
		return VerdictGood
	case overall >= thresholdModerate:
		return VerdictModerate
	case overall >= thresholdWeak:
		return VerdictWeak
	default:
		return VerdictPoor
	}
}

// sanitizeVerdict clamps every dimension score to 0-100 and trims/bounds the free-text
// fields, so no out-of-range or oversized model output is scored or served.
func sanitizeVerdict(v *recruiterVerdict) {
	for _, ds := range []*dimScore{
		&v.TitleAlignment, &v.ExperienceRelevance, &v.SeniorityFit, &v.SkillsCoverage, &v.CompanyContext, &v.LocationFit,
	} {
		ds.Score = clamp(ds.Score)
		ds.Comment = llm.TruncateRunes(strings.TrimSpace(ds.Comment), maxCommentRunes)
	}
	v.Strengths = cleanList(v.Strengths, maxStrengths, maxListItemRunes)
	v.Gaps = cleanList(v.Gaps, maxGaps, maxListItemRunes)
	v.Recommendation = llm.TruncateRunes(strings.TrimSpace(v.Recommendation), maxRecommendRunes)
}

// sanitizeRequirements coerces each requirement to the controlled vocabulary, drops the
// ones that cannot be trusted (blank text or an out-of-vocabulary status — never
// relabelled, which would misreport a match), trims text, and caps the list.
func sanitizeRequirements(in []Requirement) []Requirement {
	out := make([]Requirement, 0, len(in))
	for _, r := range in {
		text := strings.TrimSpace(r.Text)
		status := strings.TrimSpace(strings.ToLower(r.Status))
		if text == "" || !validStatus[status] {
			continue
		}
		out = append(out, Requirement{
			Text:     llm.TruncateRunes(text, maxReqTextRunes),
			Priority: coercePriority(r.Priority),
			Status:   status,
			Evidence: llm.TruncateRunes(strings.TrimSpace(r.Evidence), maxReqEvidenceRunes),
		})
		if len(out) >= maxRequirements {
			break
		}
	}
	return out
}

var validStatus = map[string]bool{
	StatusCovered: true, StatusSynonymOnly: true, StatusMissingHave: true, StatusMissingGap: true,
}

// coercePriority normalises the priority to required/preferred, defaulting anything
// unrecognised (nice-to-have, blank, …) to preferred.
func coercePriority(p string) string {
	if strings.EqualFold(strings.TrimSpace(p), PriorityRequired) {
		return PriorityRequired
	}
	return PriorityPreferred
}

// cleanList trims, drops blanks, rune-bounds each entry, and caps the count.
func cleanList(in []string, maxCount, maxRunes int) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		out = append(out, llm.TruncateRunes(s, maxRunes))
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

func clamp(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}
