package mentorship

import (
	"slices"
	"time"
)

// subtractBusy removes the mentor's busy time — their confirmed bookings and, once the
// calendar sync lands, the free/busy intervals read from it — from their free ranges,
// widening each busy range by the buffers first.
//
// The buffer rule, from cal.com and stated in the spec: the AFTER-buffer of what is
// already booked and the BEFORE-buffer of what might be booked next both apply at the
// same edge, so the gap required between two sessions is their sum. Reading the same
// pair from the other side gives the same sum at the leading edge, which is why the
// widening here is symmetric.
//
// It is symmetric only because a mentor has ONE session shape, so "the buffers of the
// existing booking" and "the buffers of the prospective one" are the same two numbers.
// When per-session-type parameters arrive, the two edges stop being equal and this
// function needs the existing booking's own buffers as an argument — that, and not the
// arithmetic, is what changes.
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
// survives with a margin and vanishes without one: with buffers the mentor asked for a
// gap around anything at all, and without them a range occupying no time takes none.
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
// ascending. It serves two callers with the same need for different reasons: the busy
// set, so the subtraction below walks a disjoint list and can stop early; and the FREE
// set in expandSchedule, where overlapping ranges would each anchor their own slot grid
// and offer the same hour twice.
//
// Touching ranges merge as well as overlapping ones. Not because keeping them apart
// would emit a zero-width gap — subtractOne already guards that — but because two blocks
// that meet at an instant are one block, and saying so once is cheaper than deciding it
// again at every use.
//
// Empty ranges are dropped first, and dropping them AFTER widening is what makes both
// cases right: a zero-width busy range with buffers has become a real block and must
// stay, and without them occupies no time and must go. Left in, it would split a free
// range in two at an instant — no time lost, and yet a session spanning that instant
// then fits in neither half.
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
// returning what is left. No remnant it emits is ever empty — a free range wholly
// covered yields nothing at all — which is the invariant every downstream step relies on
// and which TestSubtractBusyNeverEmitsAnEmptyRange holds.
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
