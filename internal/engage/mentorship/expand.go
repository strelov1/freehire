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
// The walk is over CALENDAR DATES in the mentor's zone, and the cursor is a Date rather
// than an instant. That distinction is the whole correctness argument here:
//
//   - A date has no zone and no transitions, so stepping it cannot fail. An instant
//     cursor has to be built at some wall-clock time, and every choice of one is wrong
//     somewhere: midnight does not exist once a year in America/Santiago,
//     America/Havana and Atlantic/Azores, where Go normalises it backwards to 23:00 of
//     the previous date and the walk stops advancing entirely.
//
//   - Only when a rule is placed ON a date does a zone enter, and then it is time.Date
//     that decides which instant a wall-clock time is. That is what handles the hour
//     spring skips and the hour autumn repeats — see resolve.
//
// Building 18:00 as midnight plus 1080 minutes would break for the same family of
// reasons: a transition day is 23 or 25 hours long, so offsets from its start do not
// land where the clock says.
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
// This is the one place a zone turns a stated hour into an instant, and time.Date is
// what decides the two cases where that is not a plain mapping: a wall-clock time the
// zone SKIPS in spring normalises forward to a real instant, and one it REPEATS in
// autumn resolves to a single occurrence. An end of 24:00 normalises to the following
// midnight, which is how a rule reaches the end of its day without spilling a minute
// onto the next date.
func resolve(day Date, start, end TimeOfDay, zone *time.Location) Interval {
	year, month, date := day.Parts()
	at := func(t TimeOfDay) time.Time {
		return time.Date(year, month, date, t.Minutes()/60, t.Minutes()%60, 0, 0, zone)
	}
	return Interval{Start: at(start), End: at(end)}
}

// startOfDayIn is the beginning of the calendar date an instant falls on, in that zone.
// Used to clamp a window's near end to a DAY boundary rather than to `now`, so the slot
// grid stays anchored to the mentor's stated hours.
//
// In the three zones that move their clocks at midnight this lands an hour earlier — on
// 23:00 of the previous date, since the requested midnight does not exist and Go
// normalises backwards. That is harmless and arguably right: the bound only widens, by
// an hour, and every slot it lets through is still filtered by the notice period.
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
