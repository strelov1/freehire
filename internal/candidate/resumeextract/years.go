package resumeextract

import (
	"sort"
	"time"
)

// datedYears is the whole years of work the dated roles span, overlaps counted once.
//
// The stored TotalYears was written by the model at parse time, goes stale, and can be
// misjudged, so it can contradict the dates shown beside it. A role with no start year is
// skipped; an ongoing role (current, or no end) runs to now. A year-only date counts as
// the least it could mean, so the figure never claims more than the CV shows.
func datedYears(experience []Experience, now time.Time) int {
	nowMonth := now.Year()*12 + int(now.Month()) - 1
	type span struct{ from, to int }
	spans := make([]span, 0, len(experience))
	for _, e := range experience {
		if e.Start == nil || e.Start.Year <= 0 {
			continue
		}
		// Year-only dates take the shortest span they allow (start in December, end in
		// January): "2024-2025" may be one month, and counting a year would mark a
		// vacancy's minimum as met on no evidence.
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

// EffectiveYears is the figure to show and compare against a vacancy: the larger of the
// parse-time TotalYears and the dated roles. A candidate's own figure is separate; see
// resume.Owned.ApplyTotalYears.
func (s Structured) EffectiveYears(now time.Time) int {
	return max(s.TotalYears, datedYears(s.Experience, now))
}
