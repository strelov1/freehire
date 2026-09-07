package mentorship

import (
	"slices"
	"time"
)

// expandWeekly turns a mentor's recurring rules into concrete ranges of absolute time
// covering the window, clipped to it and ordered ascending. Dated rules are skipped:
// they belong to the override pass, which replaces whole days rather than adding to
// them.
//
// The walk is over DATES IN THE MENTOR'S ZONE, and every wall-clock time is built in
// that zone before it becomes an instant. Both facts are load-bearing across a daylight
// -saving transition, and both are easy to lose:
//
//   - Stepping a day with Add(24*time.Hour) is wrong. A transition day is 23 or 25 hours
//     long, so adding 24 lands an hour off midnight and the walk drifts from there on.
//
//   - Building 18:00 as midnight plus 1080 minutes is wrong for the same reason. Only
//     naming the hour to time.Date lets the zone decide which instant it is — and lets
//     it resolve the two wall-clock times that are not one instant at all: the hour
//     spring skips, and the hour autumn repeats.
func expandWeekly(rules []Rule, zone *time.Location, w Interval) []Interval {
	if zone == nil || w.IsEmpty() {
		return nil
	}

	var out []Interval
	last := w.End.In(zone)
	for day := startOfDay(w.Start.In(zone), zone); day.Before(last); day = nextDay(day, zone) {
		for _, r := range rules {
			if r.IsDated() || r.Weekday() != day.Weekday() {
				continue
			}
			if clipped := clip(resolve(day, r.Start(), r.End(), zone), w); !clipped.IsEmpty() {
				out = append(out, clipped)
			}
		}
	}

	slices.SortFunc(out, compareIntervals)
	return out
}

// resolve places a rule's two wall-clock times on one date in one zone. An end of 24:00
// normalises to the following midnight, which is how a rule may reach the end of its day
// without spilling a minute onto the next date.
func resolve(day time.Time, start, end TimeOfDay, zone *time.Location) Interval {
	year, month, date := day.Date()
	at := func(t TimeOfDay) time.Time {
		return time.Date(year, month, date, t.Minutes()/60, t.Minutes()%60, 0, 0, zone)
	}
	return Interval{Start: at(start), End: at(end)}
}

// startOfDay is midnight on the date the instant falls on, in the given zone.
func startOfDay(at time.Time, zone *time.Location) time.Time {
	year, month, day := at.In(zone).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, zone)
}

// nextDay is midnight on the following date. It counts in DATES rather than hours, so a
// 23- or 25-hour transition day advances by exactly one day like any other.
func nextDay(day time.Time, zone *time.Location) time.Time {
	year, month, date := day.Date()
	return time.Date(year, month, date+1, 0, 0, 0, 0, zone)
}

// clip narrows an interval to a window, returning an empty interval when the two do not
// meet.
func clip(iv, w Interval) Interval {
	if iv.Start.Before(w.Start) {
		iv.Start = w.Start
	}
	if iv.End.After(w.End) {
		iv.End = w.End
	}
	return iv
}
