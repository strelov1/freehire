// Package atscheck computes a deterministic ATS-readiness score for a CV from its
// plain text: structural checks (machine-readable, contact, sections, dates,
// length, bullets) plus a role keyword-match. It is pure and I/O-free (mirrors
// internal/verdict) — the handler supplies the CV text, the CV's parsed skills,
// and the role's top in-demand skills; an optional LLM layer (see analyzer.go)
// blends a content-quality score on top.
//
// The CV text comes from a plain-text PDF extractor, so layout facts
// (multi-column, tables, images) are NOT detectable here — only what text allows.
// The strongest signal we can read is emptiness: a scanned/image CV yields almost
// no text and fails machine_readable, the single biggest ATS killer.
package atscheck

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Status is a check outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Check is one line of the readiness checklist.
type Check struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	Label  string `json:"label"`
	Fix    string `json:"fix,omitempty"`
}

// Report is the full ATS-readiness result. JSON is the wire contract shared with
// the frontend (an optional LLM layer sets content-quality on top; see analyzer.go).
type Report struct {
	Overall      int     `json:"overall"`       // 0-100 blended
	Readability  int     `json:"readability"`   // 0-100 structural pass-rate
	KeywordMatch int     `json:"keyword_match"` // 0-100 role skills present in the CV text
	Checks       []Check `json:"checks"`
	// ContentQuality/Findings are set only when the optional LLM review ran (see
	// ApplyReview); nil/empty renders no AI section.
	ContentQuality *int     `json:"content_quality,omitempty"`
	Findings       []string `json:"findings,omitempty"`
}

// Three-way Overall weights when the LLM content-quality is present (sum to 1).
const (
	weightReadabilityQ = 0.45
	weightKeywordQ     = 0.30
	weightQualityQ     = 0.25
)

// ApplyReview folds an optional LLM review into the report: it attaches the
// content-quality + findings and re-blends Overall to include content-quality. A
// nil review leaves the deterministic report untouched.
func (r *Report) ApplyReview(rv *Review) {
	if rv == nil {
		return
	}
	cq := rv.ContentQuality
	r.ContentQuality = &cq
	r.Findings = rv.Findings
	r.Overall = clamp(int(math.Round(
		weightReadabilityQ*float64(r.Readability) +
			weightKeywordQ*float64(r.KeywordMatch) +
			weightQualityQ*float64(cq),
	)))
}

// Tunable scoring constants (calibrate against real CVs).
const (
	minReadableWords = 30   // below this the CV is treated as a scan/parse failure
	lengthMin        = 150  // healthy CV word band
	lengthMax        = 1200 //
	keywordPassPct   = 70   // keyword-match ≥ this → pass
	keywordWarnPct   = 40   // ≥ this → warn, else fail

	weightReadability = 0.6 // Overall = round(wR·Readability + wK·KeywordMatch)
	weightKeyword     = 0.4
)

var (
	emailRE  = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRE  = regexp.MustCompile(`(?:\+\d[\d\s().\-]{7,}\d)|(?:\(\d{3}\)\s?\d{3}[\s.\-]?\d{4})|(?:\b\d{3}[\s.\-]\d{3}[\s.\-]\d{4}\b)`)
	dateRE   = regexp.MustCompile(`\b(?:19|20)\d{2}\b|\b\d{1,2}[/.\-]\d{4}\b`)
	bulletRE = regexp.MustCompile(`(?m)^[ \t]*[-*•‣●·][ \t]+\S`)
)

var sectionKeywords = map[string][]string{
	"experience": {"experience", "employment", "work history", "опыт работы", "опыт"},
	"skills":     {"skills", "навыки", "технолог"},
}

// Score builds the deterministic report. cvSkills are the CV's parsed skill slugs
// (the caller runs skilltag.Parse); roleTopSkills are the role's top in-demand
// skills. Pure and I/O-free.
func Score(cvText string, cvSkills, roleTopSkills []string) Report {
	words := len(strings.Fields(cvText))
	lower := strings.ToLower(cvText)

	structural := []Check{
		machineReadable(words),
		contact(cvText),
		sections(lower),
		dates(cvText),
		length(words),
		bullets(cvText),
	}

	sum := 0.0
	for _, c := range structural {
		sum += statusScore(c.Status)
	}
	readability := int(math.Round(sum / float64(len(structural)) * 100))

	kwCheck, keyword := keywordMatch(cvSkills, roleTopSkills)

	overall := int(math.Round(weightReadability*float64(readability) + weightKeyword*float64(keyword)))
	return Report{
		Overall:      clamp(overall),
		Readability:  clamp(readability),
		KeywordMatch: clamp(keyword),
		Checks:       append(structural, kwCheck),
	}
}

func statusScore(s Status) float64 {
	switch s {
	case StatusPass:
		return 1
	case StatusWarn:
		return 0.5
	default:
		return 0
	}
}

func machineReadable(words int) Check {
	if words < minReadableWords {
		return Check{ID: "machine_readable", Status: StatusFail, Label: "Text is machine-readable",
			Fix: "Export a text-based PDF (not a scan or image) so an ATS can read it."}
	}
	return Check{ID: "machine_readable", Status: StatusPass, Label: "Text is machine-readable"}
}

func contact(cv string) Check {
	c := Check{ID: "contact", Label: "Contact info (email and phone)"}
	hasEmail := emailRE.MatchString(cv)
	hasPhone := phoneRE.MatchString(cv)
	switch {
	case hasEmail && hasPhone:
		c.Status = StatusPass
	case hasEmail || hasPhone:
		c.Status = StatusWarn
		c.Fix = "Add both an email and a phone number near the top."
	default:
		c.Status = StatusFail
		c.Fix = "Add an email and a phone number so recruiters can reach you."
	}
	return c
}

func sections(lower string) Check {
	c := Check{ID: "sections", Label: "Standard sections (Experience, Skills)"}
	var missing []string
	for _, name := range []string{"experience", "skills"} {
		if !hasAny(lower, sectionKeywords[name]) {
			missing = append(missing, name)
		}
	}
	switch len(missing) {
	case 0:
		c.Status = StatusPass
	case 1:
		c.Status = StatusWarn
		c.Fix = fmt.Sprintf("Add a clearly labelled %s section.", missing[0])
	default:
		c.Status = StatusFail
		c.Fix = "Add clearly labelled Experience and Skills sections."
	}
	return c
}

func dates(cv string) Check {
	if dateRE.MatchString(cv) {
		return Check{ID: "dates", Status: StatusPass, Label: "Dated work history"}
	}
	return Check{ID: "dates", Status: StatusWarn, Label: "Dated work history",
		Fix: "Add start/end dates (years) to each role so tenure is parseable."}
}

func length(words int) Check {
	c := Check{ID: "length", Label: "Reasonable length"}
	switch {
	case words < lengthMin:
		c.Status = StatusWarn
		c.Fix = "The CV looks short — expand your experience with concrete detail."
	case words > lengthMax:
		c.Status = StatusWarn
		c.Fix = "The CV looks long — trim to the most relevant one to two pages."
	default:
		c.Status = StatusPass
	}
	return c
}

func bullets(cv string) Check {
	if len(bulletRE.FindAllString(cv, -1)) >= 2 {
		return Check{ID: "bullets", Status: StatusPass, Label: "Bulleted achievements"}
	}
	return Check{ID: "bullets", Status: StatusWarn, Label: "Bulleted achievements",
		Fix: "Use bullet points for your achievements instead of dense paragraphs."}
}

// keywordMatch scores how many of the role's top skills appear in the CV's parsed
// skills, and names the top missing ones.
func keywordMatch(cvSkills, roleTopSkills []string) (Check, int) {
	c := Check{ID: "keyword_match", Label: "Role keywords present in the CV"}
	if len(roleTopSkills) == 0 {
		c.Status = StatusWarn
		c.Fix = "Select a target role to score keyword match."
		return c, 0
	}
	owned := make(map[string]bool, len(cvSkills))
	for _, s := range cvSkills {
		owned[s] = true
	}
	matched := 0
	var missing []string
	for _, s := range roleTopSkills {
		if owned[s] {
			matched++
		} else {
			missing = append(missing, s)
		}
	}
	pct := int(math.Round(float64(matched) / float64(len(roleTopSkills)) * 100))
	switch {
	case pct >= keywordPassPct:
		c.Status = StatusPass
	case pct >= keywordWarnPct:
		c.Status = StatusWarn
	default:
		c.Status = StatusFail
	}
	if len(missing) > 0 {
		c.Fix = "Spell out these in-demand skills where you've used them: " + strings.Join(topN(missing, 5), ", ") + "."
	}
	return c, pct
}

func hasAny(lower string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func topN(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
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
