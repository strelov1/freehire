package resumeextract

import (
	"regexp"
	"strconv"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// The model restates dates; it does not compute them, so every date it returns is a claim
// about text we hold. Sanitize cannot test that claim — it sees only a well-formed, in-range
// year — which is how a résumé whose five roles all came back with the wrong years was
// stored as a success. This file checks the claim against the text the model was shown.
//
// Two grades of evidence, because they fail differently:
//
//   - A YEAR that appears nowhere in the CV is certainly invented. That date is dropped:
//     an undated role is true-but-incomplete, a wrongly dated one is false and reads as
//     fact on every CV and fit analysis built from it.
//   - A month/year PAIR missing from a CV that prints its dates numerically ("08/2023")
//     is probably wrong, but not certainly: a CV may mix "Aug 2023" with "05/2016", and
//     month names come in every language. So it is only grounds for asking the model again,
//     never for deleting what it said.

var (
	yearToken = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)
	// monthFirst is "08/2023", "8.2023", "08-2023"; yearFirst is "2023-08", "2023/8".
	monthFirst = regexp.MustCompile(`\b(0?[1-9]|1[0-2])\s?[./-]\s?((?:19|20)\d{2})\b`)
	yearFirst  = regexp.MustCompile(`\b((?:19|20)\d{2})\s?[./-]\s?(0?[1-9]|1[0-2])\b`)
)

// minNumericPairs is how many numeric month/year pairs a CV must print before a pair the
// model returned is held to them. One or two could be a lone "2021-03" in a project line
// while the work history is spelled out in words.
const minNumericPairs = 2

type dateEvidence struct {
	years map[int]bool
	pairs map[perioddate.PeriodDate]bool
}

func collectDateEvidence(text string) dateEvidence {
	ev := dateEvidence{years: map[int]bool{}, pairs: map[perioddate.PeriodDate]bool{}}

	for _, m := range yearToken.FindAllString(text, -1) {
		y, _ := strconv.Atoi(m)
		ev.years[y] = true
	}
	for _, m := range monthFirst.FindAllStringSubmatch(text, -1) {
		month, _ := strconv.Atoi(m[1])
		year, _ := strconv.Atoi(m[2])
		ev.pairs[perioddate.PeriodDate{Year: year, Month: month}] = true
	}
	for _, m := range yearFirst.FindAllStringSubmatch(text, -1) {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		ev.pairs[perioddate.PeriodDate{Year: year, Month: month}] = true
	}

	return ev
}

// unsupportedYear: the date names a year the CV never prints.
func (ev dateEvidence) unsupportedYear(d *perioddate.PeriodDate) bool {
	return d != nil && d.Year > 0 && !ev.years[d.Year]
}

// unsupportedPair: the CV prints its dates numerically, and this month/year is not among
// them. False for a year-only date, and for a CV that does not print numeric pairs.
func (ev dateEvidence) unsupportedPair(d *perioddate.PeriodDate) bool {
	return d != nil && d.Month != 0 && len(ev.pairs) >= minNumericPairs && !ev.pairs[*d]
}

// dateProblems counts the dates in s that the CV text does not support: hard are certainly
// wrong (year absent from the text), soft are doubtful (numeric pair absent).
func dateProblems(s Structured, cvText string) (hard, soft int) {
	ev := collectDateEvidence(cvText)

	count := func(d *perioddate.PeriodDate) {
		switch {
		case ev.unsupportedYear(d):
			hard++
		case ev.unsupportedPair(d):
			soft++
		}
	}
	for _, e := range s.Experience {
		count(e.Start)
		count(e.End)
	}
	for _, e := range s.Education {
		count(e.Year)
	}

	return hard, soft
}

// dropUnsupportedYears removes the dates whose year the CV never prints, and reports how
// many it removed. Soft problems are left alone: see the grading above.
func dropUnsupportedYears(s *Structured, cvText string) int {
	ev := collectDateEvidence(cvText)
	dropped := 0

	drop := func(d **perioddate.PeriodDate) {
		if ev.unsupportedYear(*d) {
			*d = nil
			dropped++
		}
	}
	for i := range s.Experience {
		drop(&s.Experience[i].Start)
		drop(&s.Experience[i].End)
	}
	for i := range s.Education {
		drop(&s.Education[i].Year)
	}

	return dropped
}
