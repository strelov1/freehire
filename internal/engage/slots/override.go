package slots

import "time"

// expandSchedule turns a mentor's whole availability (recurring rules and dated
// overrides) into ordered ranges of absolute time covering the window.
//
// An override REPLACES its date: any weekly range on a date some override names is
// dropped before that date's overrides are laid down.
//
// A CLOSURE (a dated rule with an empty span) is unconditional: it removes its date
// entirely, including other overrides for the same date. This departs from cal.com,
// where a second override survives an empty one. A wrongly withheld slot costs a
// booking; a wrongly offered one pulls a mentor out of a declared holiday.
//
// Every date is in THE MENTOR'S ZONE, whatever day it is for the viewer.
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

	// Merge overlapping ranges: the schema allows e.g. "Mon–Fri 09:00–17:00" plus
	// "Tue 16:00–19:00", and the slicer would otherwise grid each range independently
	// and emit the same hour twice.
	return mergeOverlapping(out)
}

// overriddenDates reports which dates carry any override, and which carry a closure.
// The first set decides whether the weekly schedule still applies to a date, the
// second whether anything does.
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

// dateOf is the calendar date an instant falls on in the given zone. It skips NewDate
// because a date from a time.Time is valid by construction.
func dateOf(at time.Time, zone *time.Location) Date {
	year, month, day := at.In(zone).Date()
	return Date{year: year, month: month, day: day}
}

// compareIntervals orders ranges ascending, breaking ties on the end so the order is
// total: the same inputs must yield the same slots, and a cached window must match the
// one a booking re-derives.
func compareIntervals(a, b Interval) int {
	if c := a.Start.Compare(b.Start); c != 0 {
		return c
	}
	return a.End.Compare(b.End)
}
