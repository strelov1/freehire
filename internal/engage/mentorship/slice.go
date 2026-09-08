package mentorship

// sliceSlots cuts free ranges into consecutive sessions, emitting a slot only where the
// WHOLE session fits. A tail too short for another session is not a short session, so it
// is dropped.
//
// The grid is anchored to each free range's OWN start, which is why the minimum notice
// filters slots afterwards rather than clamping the window beforehand: clamping would
// make the anchor "now plus two hours" and offer a mentor who stated 18:00 a slot at
// 14:37. Anchoring per range also keeps two ranges on the same day independent — a
// morning starting at 09:15 does not push the evening's grid off 18:00.
//
// Slots abut rather than overlap, on the same half-open bounds the rest of the engine
// and the database EXCLUDE constraint use: 18:00–19:00 and 19:00–20:00 are two slots.
//
// The step is the session duration; there is no separate slot interval. cal.com has one
// (slotInterval) so an event can be offered every 15 minutes while lasting an hour, and
// that is a knob a mentor has not asked for. It would enter here and nowhere else.
func sliceSlots(free []Interval, params SessionParams) []Interval {
	var out []Interval
	for _, f := range free {
		// Real elapsed time, not wall clock: a range spanning a daylight-saving
		// transition holds an hour more or less than its clock face suggests, and a
		// session must fit into the time that actually passes.
		for start := f.Start; !start.Add(params.Duration).After(f.End); start = start.Add(params.Duration) {
			out = append(out, Interval{Start: start, End: start.Add(params.Duration)})
		}
	}
	return out
}
