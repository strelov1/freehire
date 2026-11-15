package slots

import (
	"slices"
	"time"
)

// expandWeekly turns a mentor's recurring rules into concrete ranges of absolute time
// covering the window, clipped to it and ordered ascending. Dated rules are skipped:
// the override pass replaces whole days.
//
// The walk steps CALENDAR DATES in the mentor's zone, not instants. A date has no
// transitions, so stepping it cannot fail; an instant cursor must be built at some
// wall-clock time, and midnight does not exist once a year in America/Santiago,
// America/Havana and Atlantic/Azores (Go normalises it back to 23:00 of the previous
// date and the walk stops advancing). A zone enters only when a rule is placed on a
// date, where time.Date picks the instant (see resolve). Likewise never build 18:00 as
// midnight plus 1080 minutes: transition days are 23 or 25 hours long.
func expandWeekly(rules []Rule, zone *time.Location, w Interval) []Interval {
	if zone == nil || w.IsEmpty() {
		return nil
	}

	var out []Interval
	last := dateOf(w.End, zone)
	for day := dateOf(w.Start, zone); !day.after(last); day = day.next() {
		weekday := day.Weekday()
		for _, r := range rules {
			if r.IsDated() || r.Weekday() != weekday {
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

// resolve places a rule's two wall-clock times on one calendar date in one zone.
//
// This is the one place a zone turns a stated hour into an instant, via time.Date: a
// wall-clock time the zone SKIPS in spring normalises forward, and one it REPEATS in
// autumn resolves to a single occurrence. An end of 24:00 normalises to the next
// midnight, so a rule reaches the end of its day without spilling onto the next date.
func resolve(day Date, start, end TimeOfDay, zone *time.Location) Interval {
	year, month, date := day.Parts()
	at := func(t TimeOfDay) time.Time {
		return time.Date(year, month, date, t.Minutes()/60, t.Minutes()%60, 0, 0, zone)
	}
	return Interval{Start: at(start), End: at(end)}
}

// startOfDayIn is the beginning of the calendar date an instant falls on, in that zone.
// It clamps a window's near end to a DAY boundary rather than `now`, keeping the slot
// grid anchored to the mentor's stated hours.
//
// In the zones that move their clocks at midnight this lands at 23:00 of the previous
// date. Harmless: the bound widens by an hour and the notice period still filters.
func startOfDayIn(at time.Time, zone *time.Location) time.Time {
	return resolve(dateOf(at, zone), TimeOfDay{}, TimeOfDay{}, zone).Start
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
