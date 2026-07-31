package cvmatch

import (
	"fmt"
	"math"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// Requirement is one posting requirement as the cached fit analysis recorded it. Only Text
// and Priority are trusted here — they describe the vacancy and hold for any CV. The status
// the analysis reached was determined against the candidate's BASE profile, so it is
// carried under CachedStatus and used only where this package cannot judge for itself.
type Requirement struct {
	Text         string
	Priority     string // required | preferred
	CachedStatus string
}

// How much a requirement counts inside the category. A must-have carries more than a
// nice-to-have, so a CV that covers what the employer called required outscores one that
// covers the same number of preferred ones.
const (
	priorityWeightRequired  = 2
	priorityWeightPreferred = 1
)

// maxRequirementGram is the longest phrase looked up when reading a requirement's text.
// Three words covers the multi-word skill names the dictionary holds ("distributed
// systems", "amazon web services") without walking every span of a long sentence.
const maxRequirementGram = 3

// requirementsCategory re-derives each requirement against the current document.
//
// It never copies the cached statuses: those were reached against the base profile, and the
// whole point of this panel is that the document has since been edited.
func requirementsCategory(reqs []Requirement, hasAnalysis bool, cvSkills, jobSkills []string) (ScoredCategory, []RequirementCheck) {
	c := ScoredCategory{ID: CategoryRequirements, Label: "Requirements Coverage", Weight: WeightRequirements}
	if !hasAnalysis {
		c.Reason = "run the fit analysis to check this vacancy's requirements"
		return c, nil
	}

	owned := make(map[string]bool, len(cvSkills))
	for _, s := range cvSkills {
		owned[s] = true
	}

	checks := make([]RequirementCheck, 0, len(reqs))
	var coveredWeight, checkableWeight, covered, checkable int
	for _, r := range reqs {
		check := RequirementCheck{Text: r.Text, Priority: r.Priority}
		check.Skills = requirementSkills(r.Text, jobSkills)
		if len(check.Skills) == 0 {
			// Nothing in this line names a skill the vacancy states, so there is nothing to
			// check the document against. The earlier analysis's opinion travels with it,
			// labelled, because it is all anyone has for this requirement.
			check.Coverage = Unverifiable
			check.CachedStatus = r.CachedStatus
			checks = append(checks, check)
			continue
		}
		for _, s := range check.Skills {
			if !owned[s] {
				check.Missing = append(check.Missing, s)
			}
		}
		// Partial coverage of a requirement naming several skills is not coverage.
		check.Coverage = Covered
		if len(check.Missing) > 0 {
			check.Coverage = Missing
		}

		weight := priorityWeight(r.Priority)
		checkableWeight += weight
		checkable++
		if check.Coverage == Covered {
			coveredWeight += weight
			covered++
		}
		checks = append(checks, check)
	}

	// Nothing to divide by. The checks still travel — the panel shows them labelled — but
	// there is no score to report, and reporting zero would read as zero coverage.
	if checkableWeight == 0 {
		c.Reason = "none of this vacancy's requirements name a skill that can be checked automatically"
		return c, checks
	}
	c.Available = true

	c.Earned = int(math.Round(float64(coveredWeight) / float64(checkableWeight) * WeightRequirements))
	c.Items = []LineItem{{
		Points: c.Earned,
		Text:   fmt.Sprintf("Your CV covers %d of %d requirements", covered, checkable),
		Status: tallyStatus(covered, checkable),
	}}
	if covered < checkable {
		c.Items = append(c.Items, LineItem{
			Points: c.Weight - c.Earned,
			Text:   "Evidence the requirements you have genuinely met",
			Status: StatusWarn,
		})
	}
	return c, checks
}

func priorityWeight(priority string) int {
	if strings.EqualFold(priority, "preferred") {
		return priorityWeightPreferred
	}
	return priorityWeightRequired
}

// requirementSkills returns the vacancy's own canonical skills that this requirement's text
// names, in the vacancy's order.
//
// It deliberately does NOT tag the requirement's text with skilltag.Parse. Parse applies a
// corroboration rule — an ambiguous alias ("go", "react", "swift") tags only when the same
// text carries another strong technical term — which is right for a whole document and
// wrong for a single line: "5+ years of Go" would resolve to nothing at all. The vacancy's
// skills were already resolved from its full description, where that context existed, so
// here we only attribute what is already known rather than discovering anything new. That
// also means a requirement can never be attributed a skill outside the vacancy's own set.
func requirementSkills(text string, jobSkills []string) []string {
	if len(jobSkills) == 0 {
		return nil
	}
	stated := make(map[string]bool, len(jobSkills))
	for _, s := range jobSkills {
		stated[s] = true
	}

	words := requirementWords(text)
	grams := make([]string, 0, len(words)*maxRequirementGram)
	for n := 1; n <= maxRequirementGram; n++ {
		for i := 0; i+n <= len(words); i++ {
			grams = append(grams, strings.Join(words[i:i+n], " "))
		}
	}

	// Canonicalize resolves each span on its own terms — it is built for tokens a caller
	// asserts rather than prose to interpret, which is exactly why its results are then
	// filtered down to what the vacancy actually states.
	named := make(map[string]bool)
	for _, canonical := range skilltag.Canonicalize(grams) {
		if stated[canonical] {
			named[canonical] = true
		}
	}

	var out []string
	for _, s := range jobSkills {
		if named[s] {
			out = append(out, s)
		}
	}
	return out
}

// requirementPunctuation is stripped from the edges of a word before it is looked up.
// A leading dot is kept (".net"); a trailing one ends a sentence.
const requirementPunctuation = ",;:!?()[]{}\"'“”«»·•/\\|"

func requirementWords(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		w := strings.TrimRight(strings.Trim(f, requirementPunctuation), ".")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}
