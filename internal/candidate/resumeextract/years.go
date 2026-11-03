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
// role (marked current, or with no end) runs to now.
func datedYears(experience []Experience, now time.Time) int {
	nowMonth := now.Year()*12 + int(now.Month()) - 1
	type span struct{ from, to int }
	spans := make([]span, 0, len(experience))
	for _, e := range experience {
		if e.Start == nil || e.Start.Year <= 0 {
			continue
		}
		from := e.Start.Year*12 + max(e.Start.Month, 1) - 1
		to := nowMonth
		if !e.Current && e.End != nil && e.End.Year > 0 {
			endMonth := e.End.Month
			if endMonth == 0 {
				endMonth = 12
			}
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
