package resumeextract

import (
	"regexp"
	"strconv"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// Every date the model returns is a claim about the CV text; Sanitize only sees a
// well-formed year, so this file checks the claim against the text the model was shown.
//
// Two grades of evidence, because they fail differently:
//
//   - A YEAR that appears nowhere in the CV is certainly invented, so the date is dropped:
//     an undated role is true-but-incomplete, a wrongly dated one reads as fact downstream.
//   - A month/year PAIR missing from a CV that prints its dates numerically ("08/2023")
//     is probably wrong, but not certainly (a CV may mix "Aug 2023" with "05/2016", and
//     month names come in every language). It only justifies asking the model again.

var (
	// yearCandidate finds four digits that look like a year; the caller checks the digits on
	// either side. Go's \b is no boundary between a letter and a digit, so "FY2016" would
	// be invisible to a \b-anchored pattern.
	yearCandidate = regexp.MustCompile(`(?:19|20)\d{2}`)
	// monthFirst is "08/2023", "8.2023"; yearFirst is "2023-08", "2023/8". Neither allows a
	// space around the separator: yearFirst would read the seam of "05/2016 - 06/2024" as (2016, 06).
	monthFirst = regexp.MustCompile(`\b(0?[1-9]|1[0-2])[./-]((?:19|20)\d{2})\b`)
	yearFirst  = regexp.MustCompile(`\b((?:19|20)\d{2})[./-](0?[1-9]|1[0-2])\b`)
	// shortYear is a two-digit year as CVs print it: "Jan '16", "01/16", "Sep 16". A bare
	// two-digit number is not one ("14 plants"), so it needs an apostrophe, a month before
	// the separator, or an English month name before it.
	// shortEnd is the end of an abbreviated range, "2016–24". The trailing group rejects
	// "2016 - 06/2024", where "06" is the next date's month.
	shortEnd  = regexp.MustCompile(`\b(?:19|20)\d{2}\s*[–—-]\s*(\d{2})\b([./-]\d)?`)
	shortYear = regexp.MustCompile(`['’](\d{2})\b|\b(?:0?[1-9]|1[0-2])[./-](\d{2})\b|(?i:\b(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+)(\d{2})\b`)
)

// minNumericPairs is how many numeric month/year pairs a CV must print before the model's
// pairs are held to them; a lone "2021-03" in a project line proves nothing.
const minNumericPairs = 2

type dateEvidence struct {
	years map[int]bool
	pairs map[perioddate.PeriodDate]bool
}

func collectDateEvidence(text string) dateEvidence {
	ev := dateEvidence{years: map[int]bool{}, pairs: map[perioddate.PeriodDate]bool{}}

	for _, loc := range yearCandidate.FindAllStringIndex(text, -1) {
		if loc[0] > 0 && isDigitByte(text[loc[0]-1]) || loc[1] < len(text) && isDigitByte(text[loc[1]]) {
			continue // part of a longer number
		}
		y, _ := strconv.Atoi(text[loc[0]:loc[1]])
		ev.years[y] = true
	}
	// A two-digit year supports both centuries: the CV does not say which.
	for _, m := range shortYear.FindAllStringSubmatch(text, -1) {
		for _, g := range m[1:] {
			if yy, err := strconv.Atoi(g); g != "" && err == nil {
				ev.years[1900+yy], ev.years[2000+yy] = true, true
			}
		}
	}
	for _, m := range shortEnd.FindAllStringSubmatch(text, -1) {
		if m[2] != "" || yearFirst.MatchString(m[0]) {
			continue // the digits are a month: "05/2016 - 06/2024", or ISO "2016-08"
		}
		yy, _ := strconv.Atoi(m[1])
		ev.years[1900+yy], ev.years[2000+yy] = true, true
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

// dropUnsupportedYears removes the dates whose year the CV never prints and returns how
// many it removed. Soft problems are left alone.
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

func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// supportedDates counts the dates an answer carries that the CV does not contradict. An
// unsupported-year date is about to be dropped, and a nil date is missing, so neither counts.
func supportedDates(s Structured, cvText string) int {
	ev := collectDateEvidence(cvText)
	n := 0
	count := func(d *perioddate.PeriodDate) {
		if d != nil && !ev.unsupportedYear(d) {
			n++
		}
	}
	for _, e := range s.Experience {
		count(e.Start)
		count(e.End)
	}
	for _, e := range s.Education {
		count(e.Year)
	}

	return n
}

// retryIsBetter reports whether a second answer should replace the first: fewer date
// problems, but only among answers with as many roles and supported dates (an empty
// experience list has no problems and must not win). Hard problems outrank soft ones.
func retryIsBetter(first, again Structured, cvText string) bool {
	if len(again.Experience) < len(first.Experience) ||
		supportedDates(again, cvText) < supportedDates(first, cvText) {
		return false
	}
	hard, soft := dateProblems(first, cvText)
	h, sf := dateProblems(again, cvText)

	return h < hard || (h == hard && sf < soft)
}
