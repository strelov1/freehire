package cvmatch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/wordmatch"
)

// The two checks Job Title Match is made of. They sum to WeightTitle, but a category's
// reported weight is the sum of the checks it could actually evaluate — see titleCategory.
const (
	titleExactPoints    = 12
	titleCategoryPoints = 8
)

// titleCategory scores the vacancy's title against the document's rendered text: the title
// itself occurring in the CV, and — separately — the CV spanning the vacancy's role field.
//
// Normalization is lowercasing plus whitespace collapse, deliberately not the role
// fingerprint used elsewhere: that machinery exists to make two POSTINGS comparable and
// strips parenthetical and comma decorations a CV legitimately carries.
func titleCategory(jobTitle, cvText string) ScoredCategory {
	// Nominal weight until we know which checks can be evaluated: every category reports a
	// weight whether or not it was scored, and an unavailable one reporting 0 would read as a
	// category worth nothing rather than one nobody could score.
	c := ScoredCategory{ID: CategoryTitle, Label: "Job Title Match", Weight: WeightTitle}

	title := normalizeText(jobTitle)
	if title == "" {
		c.Reason = "this vacancy has no title to match against"
		return c
	}
	c.Available = true
	// Rebuilt from the checks that turn out to be evaluable — see addCheck.
	c.Weight = 0

	// The exact-title check is always evaluable: either the text states the title or it does
	// not. Matched on word boundaries, not as a bare substring — "Data Engineer" occurs
	// inside "data engineering", and crediting that as the title would hand full marks to
	// any CV that mentions the field in passing.
	exact := wordmatch.Contains(normalizeText(cvText), title, wordmatch.UnicodeBoundary)
	stated := strings.TrimSpace(jobTitle)
	addCheck(&c, exact, titleExactPoints,
		fmt.Sprintf("Your CV states the title %q", stated),
		fmt.Sprintf("Your CV never states the title %q", stated))

	// The role-field check is only evaluable when the dictionary resolves the vacancy's
	// title to a category. When it does not — e.g. "Product Engineer", which splits too
	// evenly between software and manufacturing for classify to guess — the check leaves
	// this category's denominator rather than costing points nobody could have earned.
	field := classify.Parse(jobTitle).Category
	if field == "" {
		return c
	}
	spans := slices.Contains(classify.Categories(cvText), field)
	addCheck(&c, spans, titleCategoryPoints,
		fmt.Sprintf("Your CV reads as %s work, the same field as this vacancy", humanizeCategory(field)),
		fmt.Sprintf("Your CV does not read as %s work", humanizeCategory(field)))
	return c
}

// addCheck folds one evaluable check into a category, stating the rule both halves of the
// unverifiable design turn on in one place: a check that CAN be evaluated always joins the
// denominator, and joins the numerator only when it passed. A check that cannot be
// evaluated is simply never added, so it costs nothing.
func addCheck(c *ScoredCategory, ok bool, points int, passText, failText string) {
	c.Weight += points
	if ok {
		c.Earned += points
		c.Items = append(c.Items, LineItem{Points: points, Text: passText, Status: StatusPass})
		return
	}
	c.Items = append(c.Items, LineItem{Points: points, Text: failText, Status: StatusWarn})
}

// humanizeCategory turns a canonical category slug into the phrase a candidate reads.
func humanizeCategory(category string) string {
	return strings.ReplaceAll(category, "_", " ")
}

// normalizeText lowercases and collapses runs of whitespace, so a heading set in caps or
// padded across a line break is the same claim as one that is not.
func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
