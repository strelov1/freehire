// Package matchanalysis computes an on-demand, LLM-driven fit analysis for a single
// (candidate, job) pair: a six-dimension scored verdict centered on job-title
// alignment and relevant experience, plus an ATS-style requirement-match table.
//
// It runs a fixed three-stage prompt-chain (see analyzer.go) — Extract & Match (the
// ATS lens) → Recruiter verdict (the human lens) → Adversarial audit — over the
// provider-agnostic internal/llm client, NOT an autonomous agent: the step count and
// order are fixed and every input is gathered before the first call. This file owns
// the wire contract, the untrusted-output sanitize, and the deterministic weighted
// scoring; the model only scores the six dimensions, while overall_score and the
// verdict label are computed here so the headline number stays consistent with the
// dimensions. Pure and I/O-free (mirrors internal/atscheck / internal/verdict).
package matchanalysis

import (
	"math"
	"strings"

	"github.com/strelov1/freehire/internal/hardconstraint"
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

// Evidence-strength grades for a covered/synonym-only requirement, strongest first.
// They rank how firmly the CV backs the match, so the audit and the served verdict
// can tell real ownership from a bare keyword. The two missing-* statuses carry none.
const (
	StrengthMetric         = "metric"         // an accomplishment with a number, scale, or measured outcome
	StrengthScope          = "scope"          // breadth of work: teams, systems, regions
	StrengthResponsibility = "responsibility" // clear ownership with tools or methods
	StrengthKeyword        = "keyword"        // the term is present but the evidence is a bare mention or duty-only
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

// Default sanitize caps for untrusted model text. Override at process start via
// SetBounds (cmd/server reads MATCH_ANALYSIS_* from the environment). A typo that
// yields a non-positive value falls back to the matching default so a bound can
// never be erased.
const (
	DefaultMaxCommentRunes       = 240  // per-dimension Comment
	DefaultMaxListItemRunes      = 200  // each Strengths / Gaps bullet
	DefaultMaxRecommendRunes     = 1200 // free-text Recommendation: two or three short prose paragraphs
	DefaultMaxReqTextRunes       = 200  // Requirement.Text
	DefaultMaxReqEvidenceRunes   = 240  // Requirement.Evidence
	DefaultMaxStrengths          = 6    // Strengths list length
	DefaultMaxGaps               = 6    // Gaps list length
	DefaultMaxRequirements       = 30   // RequirementMatch list length
	DefaultMaxSignals            = 5    // HiddenSignals list length
	DefaultMaxSignalQuoteRunes   = 200  // Signal.Quote
	DefaultMaxSignalInsightRunes = 200  // Signal.Insight
)

// Live sanitize caps — start at the defaults above; SetBounds may raise or lower them.
var (
	maxCommentRunes       = DefaultMaxCommentRunes
	maxListItemRunes      = DefaultMaxListItemRunes
	maxRecommendRunes     = DefaultMaxRecommendRunes
	maxReqTextRunes       = DefaultMaxReqTextRunes
	maxReqEvidenceRunes   = DefaultMaxReqEvidenceRunes
	maxStrengths          = DefaultMaxStrengths
	maxGaps               = DefaultMaxGaps
	maxRequirements       = DefaultMaxRequirements
	maxSignals            = DefaultMaxSignals
	maxSignalQuoteRunes   = DefaultMaxSignalQuoteRunes
	maxSignalInsightRunes = DefaultMaxSignalInsightRunes
)

// Bounds is the tunable sanitize ceiling for fit-analysis model output. Every field
// is a positive count (runes for text, items for lists). Zero / negative values are
// rejected by SetBounds and replaced with that field's Default*.
type Bounds struct {
	MaxCommentRunes       int
	MaxListItemRunes      int
	MaxRecommendRunes     int
	MaxReqTextRunes       int
	MaxReqEvidenceRunes   int
	MaxStrengths          int
	MaxGaps               int
	MaxRequirements       int
	MaxSignals            int
	MaxSignalQuoteRunes   int
	MaxSignalInsightRunes int
}

// DefaultBounds returns the hard-coded sanitize ceilings (the values historically
// baked into this package).
func DefaultBounds() Bounds {
	return Bounds{
		MaxCommentRunes:       DefaultMaxCommentRunes,
		MaxListItemRunes:      DefaultMaxListItemRunes,
		MaxRecommendRunes:     DefaultMaxRecommendRunes,
		MaxReqTextRunes:       DefaultMaxReqTextRunes,
		MaxReqEvidenceRunes:   DefaultMaxReqEvidenceRunes,
		MaxStrengths:          DefaultMaxStrengths,
		MaxGaps:               DefaultMaxGaps,
		MaxRequirements:       DefaultMaxRequirements,
		MaxSignals:            DefaultMaxSignals,
		MaxSignalQuoteRunes:   DefaultMaxSignalQuoteRunes,
		MaxSignalInsightRunes: DefaultMaxSignalInsightRunes,
	}
}

// SetBounds replaces the package sanitize caps. Any field below 1 falls back to
// that field's Default* — a typo must not erase a ceiling.
func SetBounds(b Bounds) {
	maxCommentRunes = boundOrDefault(b.MaxCommentRunes, DefaultMaxCommentRunes)
	maxListItemRunes = boundOrDefault(b.MaxListItemRunes, DefaultMaxListItemRunes)
	maxRecommendRunes = boundOrDefault(b.MaxRecommendRunes, DefaultMaxRecommendRunes)
	maxReqTextRunes = boundOrDefault(b.MaxReqTextRunes, DefaultMaxReqTextRunes)
	maxReqEvidenceRunes = boundOrDefault(b.MaxReqEvidenceRunes, DefaultMaxReqEvidenceRunes)
	maxStrengths = boundOrDefault(b.MaxStrengths, DefaultMaxStrengths)
	maxGaps = boundOrDefault(b.MaxGaps, DefaultMaxGaps)
	maxRequirements = boundOrDefault(b.MaxRequirements, DefaultMaxRequirements)
	maxSignals = boundOrDefault(b.MaxSignals, DefaultMaxSignals)
	maxSignalQuoteRunes = boundOrDefault(b.MaxSignalQuoteRunes, DefaultMaxSignalQuoteRunes)
	maxSignalInsightRunes = boundOrDefault(b.MaxSignalInsightRunes, DefaultMaxSignalInsightRunes)
}

// CurrentBounds returns a snapshot of the live sanitize caps.
func CurrentBounds() Bounds {
	return Bounds{
		MaxCommentRunes:       maxCommentRunes,
		MaxListItemRunes:      maxListItemRunes,
		MaxRecommendRunes:     maxRecommendRunes,
		MaxReqTextRunes:       maxReqTextRunes,
		MaxReqEvidenceRunes:   maxReqEvidenceRunes,
		MaxStrengths:          maxStrengths,
		MaxGaps:               maxGaps,
		MaxRequirements:       maxRequirements,
		MaxSignals:            maxSignals,
		MaxSignalQuoteRunes:   maxSignalQuoteRunes,
		MaxSignalInsightRunes: maxSignalInsightRunes,
	}
}

func boundOrDefault(n, def int) int {
	if n < 1 {
		return def
	}
	return n
}

// Dimension is one scored fit dimension on the wire.
type Dimension struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Score   int    `json:"score"` // 0-100
	Comment string `json:"comment"`
}

// Requirement is one vacancy requirement classified against the CV (the ATS lens).
type Requirement struct {
	Text             string `json:"text"`
	Priority         string `json:"priority"`          // required | preferred
	Status           string `json:"status"`            // covered | synonym-only | missing-have | missing-gap
	Evidence         string `json:"evidence"`          // where it appears in the CV, or why it is absent
	EvidenceStrength string `json:"evidence_strength"` // metric|scope|responsibility|keyword for positive statuses; empty for missing-*
}

// Signal is one interpretive read of the job posting's own wording — a verbatim quote plus
// what it implies about pace, ownership expectations, team stage, or culture. Freeform text
// bounded by length only (the same tier as Recommendation/comment), not a controlled
// vocabulary: there is no fixed set of "signal types" to coerce into.
type Signal struct {
	Quote   string `json:"quote"`
	Insight string `json:"insight"`
}

// Analysis is the full served fit verdict — the single wire contract exported to TS
// via cmd/gen-contracts.
type Analysis struct {
	Dimensions       []Dimension              `json:"dimensions"`
	RequirementMatch []Requirement            `json:"requirement_match"`
	HiddenSignals    []Signal                 `json:"hidden_signals"`
	OverallScore     int                      `json:"overall_score"`
	Verdict          string                   `json:"verdict"`
	Strengths        []string                 `json:"strengths"`
	Gaps             []string                 `json:"gaps"`
	Recommendation   string                   `json:"recommendation"`
	Blockers         []hardconstraint.Blocker `json:"blockers"`
}

// ApplyCeiling clamps the analysis's OverallScore down to ceil — the deterministic
// hard-constraint ceiling (hardconstraint.OverallCap over the caller's blockers) —
// and re-derives the Verdict from the capped score. A ceil of 100 (no unmet blocker)
// leaves the score untouched. Applied at serve time, not stored, so the ceiling
// always reflects the current dictionary (see the recompute-on-read design).
func ApplyCeiling(a *Analysis, ceil int) {
	if a == nil || ceil >= a.OverallScore {
		return
	}
	a.OverallScore = ceil
	a.Verdict = verdictFor(ceil)
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

// buildAnalysis assembles the served Analysis from the (sanitized) requirement match,
// recruiter verdict, and hidden signals: the six dimensions in fixed order, the weighted
// overall, and the derived verdict label.
func buildAnalysis(reqs []Requirement, v recruiterVerdict, signals []Signal) Analysis {
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
	if signals == nil {
		signals = []Signal{}
	}
	return Analysis{
		Dimensions:       dims,
		RequirementMatch: reqs,
		HiddenSignals:    signals,
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
		ds.Comment = llm.TrimTruncateRunes(ds.Comment, maxCommentRunes)
	}
	v.Strengths = cleanList(v.Strengths, maxStrengths, maxListItemRunes)
	v.Gaps = cleanList(v.Gaps, maxGaps, maxListItemRunes)
	v.Recommendation = llm.TrimTruncateRunes(v.Recommendation, maxRecommendRunes)
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
			Text:             llm.TruncateRunes(text, maxReqTextRunes),
			Priority:         coercePriority(r.Priority),
			Status:           status,
			Evidence:         llm.TrimTruncateRunes(r.Evidence, maxReqEvidenceRunes),
			EvidenceStrength: coerceEvidenceStrength(status, r.EvidenceStrength),
		})
		if len(out) >= maxRequirements {
			break
		}
	}
	return out
}

// sanitizeSignals drops any signal missing a quote or an insight — an interpretive claim with
// nothing to ground it is not useful half-formed, so it is dropped rather than coerced — trims
// and length-bounds what remains, and caps the count.
func sanitizeSignals(in []Signal) []Signal {
	out := make([]Signal, 0, len(in))
	for _, s := range in {
		quote := strings.TrimSpace(s.Quote)
		insight := strings.TrimSpace(s.Insight)
		if quote == "" || insight == "" {
			continue
		}
		out = append(out, Signal{
			Quote:   llm.TruncateRunes(quote, maxSignalQuoteRunes),
			Insight: llm.TruncateRunes(insight, maxSignalInsightRunes),
		})
		if len(out) >= maxSignals {
			break
		}
	}
	return out
}

var validStatus = map[string]bool{
	StatusCovered: true, StatusSynonymOnly: true, StatusMissingHave: true, StatusMissingGap: true,
}

var positiveStatus = map[string]bool{
	StatusCovered: true, StatusSynonymOnly: true,
}

var validEvidenceStrength = map[string]bool{
	StrengthMetric: true, StrengthScope: true, StrengthResponsibility: true, StrengthKeyword: true,
}

// coerceEvidenceStrength normalises the model's evidence grade to the controlled
// vocabulary: a covered/synonym-only requirement keeps a recognised grade and
// defaults anything unrecognised or blank to keyword (the weakest positive tier);
// a missing-* requirement has no evidence and so carries no strength.
func coerceEvidenceStrength(status, strength string) string {
	if !positiveStatus[status] {
		return ""
	}
	s := strings.TrimSpace(strings.ToLower(strength))
	if validEvidenceStrength[s] {
		return s
	}
	return StrengthKeyword
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
