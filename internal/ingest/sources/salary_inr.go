package sources

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// inrSalary reads an Indian salary LABEL — the display string an Indian platform puts in
// its own salary field ("₹12L – ₹18L", "12-18 Lacs P.A.", "INR 50,000 per month") — into
// rupee bounds, the currency and the period. It is for a structured field, not for prose:
// the whole label must be a salary, so a sentence that merely mentions one reads as none.
//
// The unit decides the period only where Indian usage fixes it: a lakh or crore figure is
// annual CTC, the unit every Indian salary is quoted in, so "₹12L" is ₹12,00,000 a year.
// A plain rupee figure is annual for one employer and a monthly stipend for the next
// ("₹10,000 – ₹15,000" on an internship), so it is read only when the label states its
// period. "Not Disclosed" and a label with no figure state no salary.
func inrSalary(label string) (lo, hi *int, currency, period string) {
	s := strings.ToLower(strings.TrimSpace(label))
	s = inrCleanup.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	m := inrLabelRE.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, "", ""
	}
	upTo, a, unitA, b, unitB, per := m[1] != "", m[2], m[3], m[4], m[5], m[6]
	if b == "" {
		unitB = unitA
	} else if unitA == "" && !strings.Contains(a, ",") {
		// "12-18 Lacs": the unit is written once, after the range. A grouped figure
		// ("₹75,000 – ₹1.5L") is plain rupees beside a lakh one — a monthly range whose
		// period the label does not state, so it does not inherit.
		unitA = unitB
	}
	period = inrPeriod(per)
	if period == "" {
		if unitA == "" || unitB == "" {
			return nil, nil, "", "" // plain rupees with no stated period: unknown unit
		}
		period = "year"
	}
	va, okA := inrAmount(a, unitA)
	if !okA {
		return nil, nil, "", ""
	}
	if b == "" {
		if upTo {
			return nil, &va, "INR", period
		}
		return &va, nil, "INR", period
	}
	vb, okB := inrAmount(b, unitB)
	if !okB || vb < va {
		return nil, nil, "", ""
	}
	return &va, &vb, "INR", period
}

// inrCleanup folds the currency marks and dashes a label is written with into one form.
var inrCleanup = strings.NewReplacer(
	"₹", " inr ", "rs.", " inr ", "rs ", " inr ", "–", "-", "—", "-",
	"p.a.", " pa ", "p.a", " pa ", "p.m.", " pm ", "ctc:", " ", "ctc", " ",
	"salary:", " ", "salary range:", " ", "budget:", " ",
)

// inrLabelRE is the whole label: an optional "up to", an amount with its unit, an
// optional second bound, and an optional period. Currency marks may stand before either
// amount; "inr" may also close the label.
var inrLabelRE = regexp.MustCompile(`^(?:inr )?(up ?to )?(?:inr )?([0-9][0-9,]*(?:\.[0-9]+)?) ?(l|lac|lacs|lakh|lakhs|lpa|cr|crore|crores)?\b(?: ?(?:-|to) ?(?:inr )?([0-9][0-9,]*(?:\.[0-9]+)?) ?(l|lac|lacs|lakh|lakhs|lpa|cr|crore|crores)?\b)?(?: ?(?:inr))?(?: ?(pa|per annum|per year|/ ?year|/ ?yr|annually|a year|pm|per month|/ ?month|/ ?mo|monthly|a month))?$`)

// inrPeriod maps a stated period to freehire's vocabulary, "" when none is stated.
func inrPeriod(p string) string {
	switch strings.ReplaceAll(p, " ", "") {
	case "":
		return ""
	case "pm", "permonth", "/month", "/mo", "monthly", "amonth":
		return "month"
	}
	return "year"
}

// inrAmount converts an amount and its unit to rupees. A non-positive amount is no salary.
func inrAmount(num, unit string) (int, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	switch unit {
	case "l", "lac", "lacs", "lakh", "lakhs", "lpa":
		v *= 1e5
	case "cr", "crore", "crores":
		v *= 1e7
	}
	return int(math.Round(v)), true
}
