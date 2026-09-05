package experience

import (
	"sort"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// periodSortKey orders a *perioddate.PeriodDate reverse-chronologically: nil (unknown) sorts
// lowest, and a year-only date sorts as if it were January of that year — the same
// floor the free-text parser this replaced used, so "2024" still ranks below "Feb 2024"
// and above "2023" (whatever month).
func periodSortKey(d *perioddate.PeriodDate) int {
	if d == nil {
		return 0
	}
	month := d.Month
	if month == 0 {
		month = 1
	}
	return d.Year*100 + month
}

// employmentSortKey prefers a concrete end date, treats an ongoing role as "now", and
// falls back to the start date — unlike the free-text era, there is no parsing left to
// do here: Start/End are already perioddate.PeriodDate.
func employmentSortKey(e Employment) int {
	const periodKeyCurrent = 999999
	if e.Current {
		return periodKeyCurrent
	}
	if k := periodSortKey(e.End); k != 0 {
		return k
	}
	return periodSortKey(e.Start)
}

// sortEmploymentsChronological orders current roles first, then reverse-chronological by
// derived sort key, then by id for stability. Display values are never rewritten.
func sortEmploymentsChronological(list []Employment) {
	if len(list) < 2 {
		return
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Current != b.Current {
			return a.Current // current first
		}
		ak, bk := employmentSortKey(a), employmentSortKey(b)
		if ak != bk {
			return ak > bk // more recent first; unknown (0) last among non-current
		}
		return a.ID.String() < b.ID.String()
	})
}
