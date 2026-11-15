package slots

// sliceSlots cuts free ranges into consecutive sessions, emitting a slot only where the
// WHOLE session fits; a short tail is dropped.
//
// The grid is anchored to each free range's OWN start, so the minimum notice filters
// slots afterwards instead of clamping the window (which would offer a mentor who
// stated 18:00 a slot at 14:37). Per-range anchoring also keeps two ranges on one day
// independent.
//
// Slots abut on half-open bounds: 18:00–19:00 and 19:00–20:00 are two slots. The step
// is the session duration; there is no separate slot interval (cal.com's slotInterval).
// shortcut: add one here if mentors need slots offered more often than they last.
func sliceSlots(free []Interval, params SessionParams) []Interval {
	var out []Interval
	for _, f := range free {
		// Real elapsed time, not wall clock: a range spanning a DST transition holds an
		// hour more or less than its clock face suggests.
		for start := f.Start; !start.Add(params.Duration).After(f.End); start = start.Add(params.Duration) {
			out = append(out, Interval{Start: start, End: start.Add(params.Duration)})
		}
	}
	return out
}
