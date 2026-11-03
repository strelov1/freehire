package resumeextract

import (
	"sort"
	"time"
)

// datedYears is the whole years of work the dated roles span, overlaps counted once.
//
// The stored TotalYears is a figure the model wrote when the CV was parsed: it goes stale
// the day after (a role still running keeps the length it had at parse time) and the
// model can simply misjudge it, so a card showing "2+ years" above a role that began
// three years ago contradicts itself. The dates are on the card too, so the figure must
// agree with them. A role with no start year says nothing and is skipped; an ongoing
// role (marked current, or with no end) runs to now. A year-only date counts as the
// least it could mean (see below), so the figure never claims more than the CV shows.
func datedYears(experience []Experience, now time.Time) int {
	nowMonth := now.Year()*12 + int(now.Month()) - 1
	type span struct{ from, to int }
	spans := make([]span, 0, len(experience))
	for _, e := range experience {
		if e.Start == nil || e.Start.Year <= 0 {
			continue
		}
		// A CV that gives only a year does not say which month, and this figure is
		// compared against a vacancy's minimum, so it takes the shortest span the dates
		// allow: a role begun in a year-only 2024 starts in December, one ended in a
		// year-only 2025 ends in January. "2024-2025" may be a single month, and counting
		// it as a year would mark a one-year requirement as met on no evidence.
		startMonth := e.Start.Month
		if startMonth == 0 {
			startMonth = 12
		}
		from := e.Start.Year*12 + startMonth - 1
		to := nowMonth
		if !e.Current && e.End != nil && e.End.Year > 0 {
			endMonth := max(e.End.Month, 1)
			to = min(e.End.Year*12+endMonth-1, nowMonth)
		}
		if to > from {
			spans = append(spans, span{from, to})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	months, covered := 0, 0
	for i, s := range spans {
		if i == 0 || s.from > covered {
			months += s.to - s.from
			covered = s.to
		} else if s.to > covered {
			months += s.to - covered
			covered = s.to
		}
	}
	return months / 12
}

// EffectiveYears is the figure to show and to compare against a vacancy: the larger of
// what the model wrote at parse time and what the dated roles add up to now. A
// candidate's own figure is not this one — see resume.Owned.ApplyTotalYears.
func (s Structured) EffectiveYears(now time.Time) int {
	return max(s.TotalYears, datedYears(s.Experience, now))
}
