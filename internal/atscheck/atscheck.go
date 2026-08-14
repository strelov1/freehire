// Package atscheck computes a deterministic ATS-readiness score for a CV from its
// plain text as five weighted categories (Keyword Strength, Format Compliance,
// Section Completeness, Content Quality, Length & Density) whose maxima sum to 100.
// Each category carries per-item point attribution; the overall score is the sum of
// the category scores, and the potential score adds back every recoverable point.
// It is pure and I/O-free (mirrors internal/verdict) — the handler supplies the CV
// text, the CV's parsed skills, and the role's top in-demand skills; an optional LLM
// layer (see analyzer.go) refines the Content Quality category on top.
//
// The CV text comes from a plain-text PDF extractor, so layout facts (multi-column,
// tables, images) are NOT detectable here — only what text allows. The strongest
// signal we can read is emptiness: a scanned/image CV yields almost no text and
// fails machine_readable, the single biggest ATS killer.
package atscheck

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// Status is a check outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Category IDs.
const (
	categoryKeyword  = "keyword_strength"
	categoryFormat   = "format_compliance"
	categorySections = "section_completeness"
	categoryContent  = "content_quality"
	categoryLength   = "length_density"
)

// Category maxima. Keyword (40) is named; Format (8+4+2+2+2+2=20), Sections
// (4+3+3+2+2+1=15), Content (6+6+3=15), and Length (5+5=10) are the sums of their
// item weights below. All five sum to 100.
const (
	keywordMax = 40
	contentMax = 15
)

// keywordRecommendMax caps how many missing role skills are surfaced as recommended
// keywords.
const keywordRecommendMax = 20

// LineItem is one attributed reason inside a category: awarded points when it
// passed, recoverable points when it did not.
type LineItem struct {
	Points int    `json:"points"`
	Text   string `json:"text"`
	Status Status `json:"status"`
}

// ScoreCategory is one weighted scoring dimension.
type ScoreCategory struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	Score int        `json:"score"` // Max − recoverable points
	Max   int        `json:"max"`
	Items []LineItem `json:"items"`
}

// Report is the full ATS-readiness result. JSON is the wire contract shared with
// the frontend (an optional LLM layer refines Content Quality; see analyzer.go).
type Report struct {
	Overall             int             `json:"overall"`   // sum of category scores, 0-100
	Potential           int             `json:"potential"` // overall + recoverable points, capped 100
	Categories          []ScoreCategory `json:"categories"`
	StrongKeywords      []string        `json:"strong_keywords"`
	RecommendedKeywords []string        `json:"recommended_keywords"`
	// Reviewed is true once an LLM review has been folded in (see ApplyReview) — the
	// SPA reads it to switch the "Run" button to "Re-run".
	Reviewed bool `json:"reviewed"`
	// Suggestions is set only when the optional LLM review ran (see ApplyReview);
	// empty renders no suggestions section.
	Suggestions []string `json:"suggestions,omitempty"`
}

// Tunable scoring constants (calibrate against real CVs).
const (
	minReadableWords = 30   // below this the CV is treated as a scan/parse failure
	lengthMin        = 150  // healthy CV word band
	lengthMax        = 1200 //
	minActionVerbs   = 3    // bullet lines starting with a strong verb → Content passes
	minQuantified    = 2    // quantified results → Content passes
	minDensitySkills = 3    // distinct parsed skills → Density passes
	minSummarySkills = 2    // skill tags in the summary section → Summary keyword-density passes
	maxSummaryWords  = 70   // recruiters scan a summary in ~6 seconds; longer buries the point
	minRoleBullets   = 3    // per-role bullet count band → Bullets-per-role passes
	maxRoleBullets   = 5    //
)

var (
	emailRE  = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRE  = regexp.MustCompile(`(?:\+\d[\d\s().\-]{7,}\d)|(?:\(\d{3}\)\s?\d{3}[\s.\-]?\d{4})|(?:\b\d{3}[\s.\-]\d{3}[\s.\-]\d{4}\b)`)
	dateRE   = regexp.MustCompile(`\b(?:19|20)\d{2}\b|\b\d{1,2}[/.\-]\d{4}\b`)
	bulletRE = regexp.MustCompile(`(?m)^[ \t]*[-*•‣●·][ \t]+\S`)
	quantRE  = regexp.MustCompile(`\d+(?:\.\d+)?\s?%|\$\s?\d|\b\d+x\b`)
	// The `(?:^|[^A-Za-z0-9])` boundary keeps a look-alike host like "evillinkedin.com/x"
	// from counting as a linked profile — without it the vendor domain matches wherever
	// it appears as a substring.
	linkedinRE = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])linkedin\.com/\S+`)
	// dateRangeRE matches a role's date range ("2021 - 2026", "Jan 2023 – Present",
	// "Mar 2020 - Dec 2022") and captures the start year, which is all roleBlocks needs
	// to order roles and count their bullets. The end side tolerates a leading month
	// word the same way the start side's \b does implicitly.
	dateRangeRE = regexp.MustCompile(`(?i)\b((?:19|20)\d{2})\s*[-–—]\s*(?:present|(?:[a-z]+\.?\s+)?(?:19|20)\d{2})\b`)
)

var sectionKeywords = map[string][]string{
	"experience": {"experience", "employment", "work history", "опыт работы", "опыт"},
	"skills":     {"skills", "навыки", "технолог"},
	"education":  {"education", "образование"},
	"summary":    {"summary", "profile", "objective", "о себе"},
}

// actionVerbs are strong résumé openers checked at the start of bullet lines.
var actionVerbs = set(
	"built", "led", "shipped", "designed", "developed", "improved", "reduced",
	"scaled", "launched", "migrated", "architected", "delivered", "increased",
	"decreased", "cut", "drove", "owned", "created", "implemented", "optimized",
	"automated", "established", "spearheaded", "managed", "mentored", "introduced",
	"rebuilt", "streamlined", "accelerated",
)

// Score builds the deterministic report. cvSkills are the CV's parsed skill slugs
// (the caller runs skilltag.Parse); roleTopSkills are the role's top in-demand
// skills. Pure and I/O-free.
func Score(cvText string, cvSkills, roleTopSkills []string) Report {
	words := len(strings.Fields(cvText))
	lower := strings.ToLower(cvText)

	keyword, strong, recommended := keywordCategory(cvSkills, roleTopSkills)
	r := Report{
		Categories: []ScoreCategory{
			keyword,
			formatCategory(cvText, words),
			sectionsCategory(lower, cvText),
			contentCategory(cvText),
			lengthCategory(words, cvSkills),
		},
		StrongKeywords:      strong,
		RecommendedKeywords: recommended,
	}
	r.recompute()
	return r
}

// recompute derives each category's Score/Max from its items and the report's
// Overall (sum of scores) and Potential (Overall + recoverable points).
func (r *Report) recompute() {
	overall, recoverable := 0, 0
	for i := range r.Categories {
		score, max := 0, 0
		for _, it := range r.Categories[i].Items {
			max += it.Points
			if it.Status == StatusPass {
				score += it.Points
			} else {
				recoverable += it.Points
			}
		}
		r.Categories[i].Score = score
		r.Categories[i].Max = max
		overall += score
	}
	r.Overall = clamp(overall)
	r.Potential = clamp(overall + recoverable)
}

// keywordCategory scores how many of the role's top skills appear in the CV's
// parsed skills, and splits them into strong (present) and recommended (missing).
func keywordCategory(cvSkills, roleTopSkills []string) (ScoreCategory, []string, []string) {
	c := ScoreCategory{ID: categoryKeyword, Label: "Keyword Strength"}
	if len(roleTopSkills) == 0 {
		c.Items = []LineItem{{Points: keywordMax, Text: "Select a target role to score keyword match", Status: StatusWarn}}
		return c, nil, nil
	}
	owned := set(cvSkills...)
	var strong, missing []string
	for _, s := range roleTopSkills {
		if owned[s] {
			strong = append(strong, s)
		} else {
			missing = append(missing, s)
		}
	}
	score := int(math.Round(float64(len(strong)) / float64(len(roleTopSkills)) * keywordMax))
	if len(strong) > 0 {
		c.Items = append(c.Items, LineItem{
			Points: score,
			Text:   fmt.Sprintf("%d of %d in-demand role keywords present", len(strong), len(roleTopSkills)),
			Status: StatusPass,
		})
	}
	if len(missing) > 0 {
		c.Items = append(c.Items, LineItem{
			Points: keywordMax - score,
			Text:   "Add the recommended keywords where you've genuinely used them",
			Status: StatusWarn,
		})
	}
	return c, strong, topN(missing, keywordRecommendMax)
}

func formatCategory(cv string, words int) ScoreCategory {
	return ScoreCategory{ID: categoryFormat, Label: "Format Compliance", Items: []LineItem{
		item(words >= minReadableWords, 8, "Text is machine-readable",
			"Export a text-based PDF (not a scan or image) so an ATS can read it", StatusFail),
		item(emailRE.MatchString(cv) && phoneRE.MatchString(cv), 4, "Contact info present (email and phone)",
			"Add both an email and a phone number near the top", StatusWarn),
		item(len(bulletRE.FindAllString(cv, -1)) >= 2, 2, "Bulleted achievements",
			"Use bullet points for your achievements instead of dense paragraphs", StatusWarn),
		item(dateRE.MatchString(cv), 2, "Dated work history",
			"Add start/end dates (years) to each role so tenure is parseable", StatusWarn),
		item(linkedinRE.MatchString(cv), 2, "LinkedIn profile linked",
			"Add your LinkedIn profile URL near the top", StatusWarn),
		chronologyItem(cv, 2),
	}}
}

// chronologyItem rewards a résumé that lists roles most-recent-first, the order both an
// ATS and a recruiter expect. Fewer than two dated roles in the Experience section can't
// be out of order, so it passes vacuously; zero dated roles fails, matching the "Dated
// work history" item above.
func chronologyItem(cv string, weight int) LineItem {
	blocks := roleBlocks(cv)
	if len(blocks) == 0 {
		return LineItem{Points: weight, Text: "Add start/end dates to each role so recency reads in order", Status: StatusWarn}
	}
	ok := true
	for i := 1; i < len(blocks); i++ {
		if blocks[i].startYear > blocks[i-1].startYear {
			ok = false
			break
		}
	}
	return item(ok, weight, "Roles are listed most recent first",
		"List your roles most recent first (reverse-chronological order)", StatusWarn)
}

func sectionsCategory(lower, cvText string) ScoreCategory {
	// Weights re-balanced to fit the summary keyword-density and summary-length items
	// while keeping the category maximum at 15 (and the five-category total at 100).
	return ScoreCategory{ID: categorySections, Label: "Section Completeness", Items: []LineItem{
		item(hasAny(lower, sectionKeywords["experience"]), 4, "Experience section",
			"Add a clearly labelled Experience section", StatusWarn),
		item(hasAny(lower, sectionKeywords["skills"]), 3, "Skills section",
			"Add a clearly labelled Skills section", StatusWarn),
		item(hasAny(lower, sectionKeywords["education"]), 3, "Education section",
			"Add an Education section", StatusWarn),
		item(hasAny(lower, sectionKeywords["summary"]), 2, "Professional summary",
			"Add a short professional summary at the top", StatusWarn),
		summaryDensityItem(cvText, 2),
		summaryLengthItem(cvText, 1),
	}}
}

// summaryLengthItem rewards a summary a recruiter can read in one glance — the same
// ~6-second-scan rationale as summaryDensityItem, applied to length instead of density.
// An absent summary (0 words) fails, matching the "Professional summary" item above.
func summaryLengthItem(cvText string, weight int) LineItem {
	n := len(strings.Fields(summaryText(cvText)))
	return item(n > 0 && n <= maxSummaryWords, weight, "Summary is concise (≤70 words)",
		"Trim your summary to about 70 words or fewer", StatusWarn)
}

// summaryDensityItem rewards a keyword-dense professional summary — recruiters scan
// it in ~6 seconds, so it should carry the CV's concrete skills.
func summaryDensityItem(cvText string, weight int) LineItem {
	n := len(skilltag.Parse(summaryText(cvText), skilltag.WithResumeAcronyms()))
	return item(n >= minSummarySkills, weight, "Summary carries role keywords",
		"Add a few role keywords to your professional summary", StatusWarn)
}

// summaryText returns the text under the CV's summary heading, up to the next section
// heading (or "" when there is no summary section).
func summaryText(cvText string) string {
	var out []string
	in := false
	for _, line := range strings.Split(cvText, "\n") {
		if section, ok := headingSection(line); ok {
			in = section == "summary"
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// headingSection reports which section a line heads, matching a heading line whose
// normalized text equals one of a section's keywords.
func headingSection(line string) (string, bool) {
	norm := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(strings.ToLower(line)), ":"))
	if norm == "" {
		return "", false
	}
	for section, kws := range sectionKeywords {
		for _, k := range kws {
			if norm == k {
				return section, true
			}
		}
	}
	return "", false
}

// contentCategory is the deterministic Content Quality proxy used when no LLM review
// is present (see ApplyReview for the LLM path).
func contentCategory(cv string) ScoreCategory {
	return ScoreCategory{ID: categoryContent, Label: "Content Quality", Items: []LineItem{
		item(actionVerbLines(cv) >= minActionVerbs, 6, "Bullets lead with strong action verbs",
			"Start bullets with strong action verbs (Built, Led, Shipped…)", StatusWarn),
		item(len(quantRE.FindAllString(cv, -1)) >= minQuantified, 6, "Quantified, measurable results",
			"Quantify achievements with concrete numbers (%, ×, $)", StatusWarn),
		bulletsPerRoleItem(cv, 3),
	}}
}

// bulletsPerRoleItem rewards each Experience-section role carrying 3 to 5 bullet points —
// enough to show depth, not so many a recruiter skims past them. Zero dated roles fails,
// matching the "Dated work history" format item.
func bulletsPerRoleItem(cv string, weight int) LineItem {
	blocks := roleBlocks(cv)
	if len(blocks) == 0 {
		return LineItem{Points: weight, Text: "Give each recent role 3 to 5 bullet points", Status: StatusWarn}
	}
	ok := true
	for _, b := range blocks {
		if b.bullets < minRoleBullets || b.bullets > maxRoleBullets {
			ok = false
			break
		}
	}
	return item(ok, weight, "Each role has 3 to 5 bullet points",
		"Give each recent role 3 to 5 bullet points", StatusWarn)
}

// roleBlock is one Experience-section role: the start year from its date range, and how
// many bullet lines follow it before the next role or section.
type roleBlock struct {
	startYear int
	bullets   int
}

// roleBlocks scans the CV's Experience section for role headers stating a date range
// ("2021 - 2026", "Jan 2023 – Present") and counts the bullet lines under each, in
// document order. It never looks outside Experience, so an Education line carrying its
// own date range ("BSc, MIT (2014 - 2018)") is not mistaken for a role.
func roleBlocks(cvText string) []roleBlock {
	var blocks []roleBlock
	inExperience := false
	for _, line := range strings.Split(cvText, "\n") {
		if section, ok := headingSection(line); ok {
			inExperience = section == "experience"
			continue
		}
		if !inExperience {
			continue
		}
		if m := dateRangeRE.FindStringSubmatch(line); m != nil {
			year, _ := strconv.Atoi(m[1])
			blocks = append(blocks, roleBlock{startYear: year})
			continue
		}
		if len(blocks) > 0 && bulletRE.MatchString(line) {
			blocks[len(blocks)-1].bullets++
		}
	}
	return blocks
}

func lengthCategory(words int, cvSkills []string) ScoreCategory {
	return ScoreCategory{ID: categoryLength, Label: "Length & Density", Items: []LineItem{
		item(words >= lengthMin && words <= lengthMax, 5, "Reasonable length",
			lengthFix(words), StatusWarn),
		item(len(set(cvSkills...)) >= minDensitySkills, 5, "Good keyword density",
			"Surface more concrete skills tied to your work", StatusWarn),
	}}
}

// ApplyReview folds an optional LLM review into the report: it replaces the Content
// Quality category with the LLM's content-quality score, attaches the suggestions,
// and re-sums Overall/Potential. A nil review leaves the report untouched.
func (r *Report) ApplyReview(rv *Review) {
	if rv == nil {
		return
	}
	cq := clamp(rv.ContentQuality)
	score := int(math.Round(float64(cq) / 100 * contentMax))
	for i := range r.Categories {
		if r.Categories[i].ID != categoryContent {
			continue
		}
		items := []LineItem{{Points: score, Text: fmt.Sprintf("AI-assessed content quality: %d/100", cq), Status: StatusPass}}
		if score < contentMax {
			items = append(items, LineItem{Points: contentMax - score, Text: "Apply the numbered Suggestions below to close the gap", Status: StatusWarn})
		}
		r.Categories[i].Items = items
	}
	r.Reviewed = true
	r.Suggestions = rv.Suggestions
	r.recompute()
}

// actionVerbLines counts lines whose first word (after any bullet marker) is a
// strong action verb.
func actionVerbLines(cv string) int {
	n := 0
	for _, line := range strings.Split(cv, "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "-*•‣●· \t")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if actionVerbs[strings.ToLower(fields[0])] {
			n++
		}
	}
	return n
}

func lengthFix(words int) string {
	if words < lengthMin {
		return "The CV looks short — expand your experience with concrete detail"
	}
	return "The CV looks long — trim to the most relevant one to two pages"
}

// item builds a line item: a pass awards its weight, otherwise the weight is
// recoverable and the failText/failStatus applies.
func item(ok bool, weight int, passText, failText string, failStatus Status) LineItem {
	if ok {
		return LineItem{Points: weight, Text: passText, Status: StatusPass}
	}
	return LineItem{Points: weight, Text: failText, Status: failStatus}
}

func hasAny(lower string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
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
