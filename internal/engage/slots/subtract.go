package slots

import (
	"slices"
	"time"
)

// subtractBusy removes the mentor's busy time (bookings, calendar free/busy) from
// their free ranges, widening each busy range by the buffers first.
//
// The AFTER-buffer of an existing booking and the BEFORE-buffer of a prospective one
// apply at the same edge, so the required gap is their sum and the widening is
// symmetric. That holds only because a mentor has ONE session shape; with
// per-session-type parameters this needs the existing booking's own buffers.
func subtractBusy(free, busy []Interval, params SessionParams) []Interval {
	if len(free) == 0 {
		return nil
	}

	blocked := mergeOverlapping(widen(busy, params.BufferBefore+params.BufferAfter))

	out := make([]Interval, 0, len(free))
	for _, f := range free {
		out = append(out, subtractOne(f, blocked)...)
	}

	slices.SortFunc(out, compareIntervals)
	return out
}

// widen grows each busy range by the same margin at both ends. A zero-width busy range
// survives with a margin and vanishes without one.
func widen(busy []Interval, margin time.Duration) []Interval {
	if margin == 0 {
		return busy
	}
	out := make([]Interval, 0, len(busy))
	for _, b := range busy {
		out = append(out, Interval{Start: b.Start.Add(-margin), End: b.End.Add(margin)})
	}
	return out
}

// mergeOverlapping collapses ranges that overlap or touch into single ranges, sorted
// ascending. Callers: the busy set (so subtraction walks a disjoint list and can stop
// early) and the FREE set in expandSchedule (overlaps would anchor separate slot grids
// and offer the same hour twice).
//
// Empty ranges are dropped first, after widening: a zero-width busy range with buffers
// is a real block and stays; without them it must go, or it would split a free range
// at an instant so a session spanning it fits in neither half.
func mergeOverlapping(ranges []Interval) []Interval {
	sorted := make([]Interval, 0, len(ranges))
	for _, r := range ranges {
		if !r.IsEmpty() {
			sorted = append(sorted, r)
		}
	}
	if len(sorted) == 0 {
		return nil
	}
	slices.SortFunc(sorted, compareIntervals)

	merged := []Interval{sorted[0]}
	for _, r := range sorted[1:] {
		last := &merged[len(merged)-1]
		if r.Start.After(last.End) {
			merged = append(merged, r)
			continue
		}
		if r.End.After(last.End) {
			last.End = r.End
		}
	}
	return merged
}

// subtractOne removes a sorted, disjoint set of blocked ranges from one free range,
// returning what is left. No remnant is ever empty (a wholly covered range yields
// nothing); downstream steps rely on this and TestSubtractBusyNeverEmitsAnEmptyRange
// holds it.
func subtractOne(free Interval, blocked []Interval) []Interval {
	var out []Interval
	cursor := free.Start

	keep := func(iv Interval) {
		if !iv.IsEmpty() {
			out = append(out, iv)
		}
	}

	for _, b := range blocked {
		if !b.End.After(cursor) {
			continue
		}
		if !b.Start.Before(free.End) {
			break
		}
		keep(Interval{Start: cursor, End: b.Start})
		if b.End.After(cursor) {
			cursor = b.End
		}
	}

	keep(Interval{Start: cursor, End: free.End})
	return out
}
