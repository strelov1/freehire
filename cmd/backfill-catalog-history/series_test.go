package main

import (
	"testing"
	"time"
)

func day(n int) time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n) }

func TestSeries(t *testing.T) {
	flows := []flow{
		{Day: day(0), Added: 10},
		{Day: day(2), Added: 5, Removed: 3}, // day 1 has no movement and repeats day 0
	}
	firsts := []firstID{{day(0), 100}, {day(1), 200}, {day(2), 300}}
	prunes := []pruned{
		{ID: 150, Day: day(2)}, // born day 0, deleted day 2: listed at the end of days 0 and 1
		{ID: 250, Day: day(1)}, // born and deleted on day 1: never at a day's end
		{ID: 50, Day: day(2)},  // older than any dated row: undatable, skipped
		{ID: 160, Day: day(2)},
	}

	got := series(flows, firsts, prunes, 0.5, day(3))
	want := []int64{11, 11, 12, 12} // 10+1, 11, 11+2-1, unchanged
	if len(got) != len(want) {
		t.Fatalf("got %d days, want %d: %+v", len(got), len(want), got)
	}
	for i, p := range got {
		if !p.Day.Equal(day(i)) || p.Open != want[i] {
			t.Errorf("day %d = %s/%d, want %s/%d", i, p.Day.Format(time.DateOnly), p.Open, day(i).Format(time.DateOnly), want[i])
		}
	}
}

func TestSeriesWithoutPrunedShare(t *testing.T) {
	got := series([]flow{{Day: day(0), Added: 3}}, []firstID{{day(0), 1}}, []pruned{{ID: 1, Day: day(1)}}, 0, day(1))
	if got[0].Open != 3 || got[1].Open != 3 {
		t.Fatalf("share 0 must ignore pruned postings: %+v", got)
	}
}

func TestSeriesStartsAtAPrunedBirthBeforeTheFirstFlow(t *testing.T) {
	got := series([]flow{{Day: day(2), Added: 4}}, []firstID{{day(0), 1}}, []pruned{{ID: 1, Day: day(3)}}, 1, day(3))
	want := []int64{1, 1, 5, 4}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, p := range got {
		if p.Open != want[i] {
			t.Errorf("day %d = %d, want %d", i, p.Open, want[i])
		}
	}
}
