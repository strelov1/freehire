package mentorship

import "time"

// expandSchedule turns a mentor's whole availability — recurring rules and dated
// overrides together — into ordered ranges of absolute time covering the window.
//
// An override REPLACES its date rather than adding to it: any weekly range landing on a
// date some override names is dropped before the overrides for that date are laid down.
// This is what makes "this Tuesday, 10:00–12:00 only" mean what it says instead of
// piling a morning on top of the usual evening.
//
// A CLOSURE — a dated rule with an empty span — is unconditional: it removes its date
// entirely, including any other override written for the same date. That is a departure
// from cal.com, where an empty override is merely a zero-length range that gets filtered
// out alongside the rest, and a second override for the date survives it. The asymmetry
// of the mistake decides it: a slot wrongly withheld costs a booking, a slot wrongly
// offered pulls a mentor out of the holiday they declared.
//
// Every date here is a date IN THE MENTOR'S ZONE. A viewer elsewhere may already be on
// the following day, but it is the mentor's own calendar an override closes.
func expandSchedule(rules []Rule, zone *time.Location, w Interval) []Interval {
	if zone == nil || w.IsEmpty() {
		return nil
	}

	overridden, closed := overriddenDates(rules)

	var out []Interval
	for _, iv := range expandWeekly(rules, zone, w) {
		if !overridden[dateOf(iv.Start, zone)] {
			out = append(out, iv)
		}
	}
	for _, r := range rules {
		if !r.IsDated() || closed[r.Date()] {
			continue
		}
		if iv := clip(resolve(r.Date(), r.Start(), r.End(), zone), w); !iv.IsEmpty() {
			out = append(out, iv)
		}
	}

	// Overlapping free ranges are merged, not concatenated. Nothing stops a mentor from
	// writing "Mon–Fri 09:00–17:00" and "Tue 16:00–19:00", or the same row twice, and
	// the schema has no exclusion constraint that would. Left overlapping, the slicer
	// would anchor a grid to each range independently and emit the same hour more than
	// once — cal.com merges for the same reason (mergeOverlappingDateRanges).
	return mergeOverlapping(out)
}

// overriddenDates reports which dates carry any override at all, and which carry a
// closure. The two sets are separate because they answer different questions: the first
// decides whether the weekly schedule still applies to a date, the second whether
// anything does.
func overriddenDates(rules []Rule) (overridden, closed map[Date]bool) {
	overridden, closed = map[Date]bool{}, map[Date]bool{}
	for _, r := range rules {
		if !r.IsDated() {
			continue
		}
		overridden[r.Date()] = true
		if r.IsClosure() {
			closed[r.Date()] = true
		}
	}
	return overridden, closed
}

// dateOf is the calendar date an instant falls on in the given zone. It builds the Date
// directly rather than through NewDate: a date read back out of a time.Time is real by
// construction, so there is nothing for the validating constructor to reject.
func dateOf(at time.Time, zone *time.Location) Date {
	year, month, day := at.In(zone).Date()
	return Date{year: year, month: month, day: day}
}

// compareIntervals orders ranges ascending, breaking a tie on the end so the order is
// total. Determinism is a requirement rather than a nicety: the same inputs must yield
// the same slots, and a cached window must match the one a booking re-derives.
func compareIntervals(a, b Interval) int {
	if c := a.Start.Compare(b.Start); c != 0 {
		return c
	}
	return a.End.Compare(b.End)
}
