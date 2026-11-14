package main

import (
	"sort"
	"time"
)

// flow is one UTC day's movement in the listed catalogue.
type flow struct {
	Day            time.Time
	Added, Removed int64
}

// firstID is the smallest job id created on Day.
type firstID struct {
	Day time.Time
	ID  int64
}

// pruned is one hard-deleted posting: its id and the day it was deleted.
type pruned struct {
	ID  int64
	Day time.Time
}

// point is the catalogue's size at the end of Day.
type point struct {
	Day  time.Time
	Open int64
}

// series turns daily flows into the catalogue's size at the end of each day from the first
// flow to through, inclusive. A day with no movement repeats the previous size.
//
// Pruned postings left jobs without a trace in the flows, so they are added back: each one
// counts from the day its id was allocated (dated through firsts, which must be sorted by
// day) until the day it was deleted. prunedShare scales them, because the prune also took
// closed postings and duplicates, which were never part of the listed catalogue, and the
// archive does not say which ones those were.
func series(flows []flow, firsts []firstID, prunes []pruned, prunedShare float64, through time.Time) []point {
	delta := map[time.Time]float64{}
	for _, f := range flows {
		delta[f.Day] += float64(f.Added - f.Removed)
	}
	for _, p := range prunes {
		born, ok := dayOfID(firsts, p.ID)
		if !ok || !born.Before(p.Day) {
			continue // created and deleted the same day, or older than any dated row: never counted at a day's end
		}
		delta[born] += prunedShare
		delta[p.Day] -= prunedShare
	}
	// The walk starts at the earliest day anything moved — a pruned posting can be born
	// before the first listed one, and skipping its birth would still subtract its deletion.
	start := through
	for d := range delta {
		if d.Before(start) {
			start = d
		}
	}

	var out []point
	open := 0.0
	for d := start; !d.After(through); d = d.AddDate(0, 0, 1) {
		open += delta[d]
		out = append(out, point{Day: d, Open: int64(open + 0.5)})
	}
	return out
}

// dayOfID is the day the id was allocated on: the last day whose first id is at or below it.
func dayOfID(firsts []firstID, id int64) (time.Time, bool) {
	i := sort.Search(len(firsts), func(i int) bool { return firsts[i].ID > id })
	if i == 0 {
		return time.Time{}, false
	}
	return firsts[i-1].Day, true
}
