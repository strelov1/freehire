package cvmatch

import (
	"fmt"
	"slices"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/vocab"
)

// seniorityCategory compares the grade the dictionaries read from the vacancy's title
// against the grade they read from the document.
//
// classify.Parse walks the seniority table highest-rank-first and returns the first alias
// it finds, so run over a whole CV it yields the strongest grade the document claims —
// which is the right reading for a CV listing a career's worth of titles.
//
// Either side resolving to nothing makes the category unavailable rather than a mismatch.
// A title that states no grade is not a junior one, and this is the lightest-weighted
// category precisely because it barely moves under tailoring: the candidate should not be
// pushed to chase it by inflating a title.
func seniorityCategory(jobTitle, cvText string) ScoredCategory {
	c := ScoredCategory{ID: CategorySeniority, Label: "Seniority Fit", Weight: WeightSeniority}

	want := gradeRank(classify.Parse(jobTitle).Seniority)
	if want < 0 {
		c.Reason = "this vacancy's title states no seniority"
		return c
	}
	got := gradeRank(classify.Parse(cvText).Seniority)
	if got < 0 {
		c.Reason = "your CV states no seniority to compare"
		return c
	}
	c.Available = true

	asked := humanizeGrade(vocab.SeniorityValues[want])
	reads := humanizeGrade(vocab.SeniorityValues[got])
	item := LineItem{Points: WeightSeniority}
	switch rungsApart(want, got) {
	case 0:
		c.Earned = WeightSeniority
		item.Text = fmt.Sprintf("Your CV reads at the %s grade this vacancy asks for", asked)
		item.Status = StatusPass
	case 1:
		c.Earned = WeightSeniority / 2
		item.Points = WeightSeniority / 2
		item.Text = fmt.Sprintf("Your CV reads at %s, one rung from the %s this vacancy asks for", reads, asked)
		item.Status = StatusWarn
	default:
		item.Text = fmt.Sprintf("Your CV reads at %s; this vacancy asks for %s", reads, asked)
		item.Status = StatusFail
	}
	c.Items = []LineItem{item}
	return c
}

func rungsApart(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// gradeRank is a grade's rung on the seniority ladder, or -1 when the dictionaries
// resolved none. The ladder's order is vocab.SeniorityValues' order — intern through
// c_level — which is what makes "one rung apart" a meaningful distance.
func gradeRank(grade string) int {
	if grade == "" {
		return -1
	}
	return slices.Index(vocab.SeniorityValues, grade)
}

// humanizeGrade turns a canonical grade into the phrase a candidate reads.
func humanizeGrade(grade string) string {
	if grade == "c_level" {
		return "C-level"
	}
	return grade
}
