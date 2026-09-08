package mentorship

import (
	"testing"
	"time"
)

// tuesdayEvenings is the schedule most of these tests vary: a plain recurring evening,
// so that what an override does to it is the only thing under test.
func tuesdayEvenings(t *testing.T) Rule {
	t.Helper()
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	return rule
}

func datedRule(t *testing.T, day int, fromHour, toHour int) Rule {
	t.Helper()
	rule, err := NewDatedRule(
		mustDate(t, 2026, time.September, day),
		mustTimeOfDay(t, fromHour, 0),
		mustTimeOfDay(t, toHour, 0),
	)
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}
	return rule
}

// localHours renders each range as "MM-DD HH:MM–HH:MM" in the given zone, so a failure
// reads as a schedule instead of as a list of instants.
func localHours(zone *time.Location, ranges []Interval) []string {
	out := make([]string, 0, len(ranges))
	for _, iv := range ranges {
		start, end := iv.Start.In(zone), iv.End.In(zone)
		out = append(out, start.Format("01-02 15:04")+"–"+end.Format("15:04"))
	}
	return out
}

func assertRanges(t *testing.T, zone *time.Location, got []Interval, want ...string) {
	t.Helper()
	gotStrings := localHours(zone, got)
	if len(gotStrings) != len(want) {
		t.Fatalf("got %v, want %v", gotStrings, want)
	}
	for i := range want {
		if gotStrings[i] != want[i] {
			t.Errorf("range %d = %q, want %q (full: %v)", i, gotStrings[i], want[i], gotStrings)
		}
	}
}

// The override replaces its day rather than adding to it: the recurring evening is gone
// from that date, and present on every other.
func TestOverrideReplacesItsWholeDay(t *testing.T) {
	zone := berlin(t)
	rules := []Rule{tuesdayEvenings(t), datedRule(t, 15, 10, 12)}

	// Two Tuesdays: the 8th (ordinary) and the 15th (overridden).
	got := expandSchedule(rules, zone, window(t, zone, "2026-09-07", "2026-09-16"))

	assertRanges(t, zone, got, "09-08 18:00–20:00", "09-15 10:00–12:00")
}

func TestAnEmptyOverrideClosesItsDayAndLeavesOthersAlone(t *testing.T) {
	zone := berlin(t)
	closure, err := NewDatedRule(mustDate(t, 2026, time.September, 15), mustTimeOfDay(t, 0, 0), mustTimeOfDay(t, 0, 0))
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}

	got := expandSchedule([]Rule{tuesdayEvenings(t), closure}, zone, window(t, zone, "2026-09-07", "2026-09-23"))

	assertRanges(t, zone, got, "09-08 18:00–20:00", "09-22 18:00–20:00")
}

// The decision this package makes and cal.com does not: a closure is an unconditional
// statement about its date, not one vote among several. A mentor who says "I am away on
// the 15th" and forgets to delete an older override for the same date is away.
func TestAClosureBeatsEveryOtherOverrideOnItsDate(t *testing.T) {
	zone := berlin(t)
	closure, err := NewDatedRule(mustDate(t, 2026, time.September, 15), mustTimeOfDay(t, 9, 0), mustTimeOfDay(t, 9, 0))
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}
	// Deliberately ordered with the closure last, and again first, to prove the outcome
	// does not depend on which row the mentor happened to write second.
	forwards := []Rule{tuesdayEvenings(t), datedRule(t, 15, 10, 12), closure}
	backwards := []Rule{tuesdayEvenings(t), closure, datedRule(t, 15, 10, 12)}

	w := window(t, zone, "2026-09-15", "2026-09-16")
	if got := expandSchedule(forwards, zone, w); len(got) != 0 {
		t.Errorf("closure written last: got %v, want nothing", localHours(zone, got))
	}
	if got := expandSchedule(backwards, zone, w); len(got) != 0 {
		t.Errorf("closure written first: got %v, want nothing", localHours(zone, got))
	}
}

// Two non-empty overrides on one date are a mentor describing a split day, not a
// conflict; both stand.
func TestTwoOverridesOnOneDateBothStand(t *testing.T) {
	zone := berlin(t)
	rules := []Rule{tuesdayEvenings(t), datedRule(t, 15, 14, 16), datedRule(t, 15, 10, 12)}

	got := expandSchedule(rules, zone, window(t, zone, "2026-09-15", "2026-09-16"))

	assertRanges(t, zone, got, "09-15 10:00–12:00", "09-15 14:00–16:00")
}

// An override on a date the weekly schedule never covered adds availability rather than
// replacing anything — the mentor opening a one-off Saturday.
func TestAnOverrideOnAnOtherwiseEmptyDayAddsAvailability(t *testing.T) {
	zone := berlin(t)
	// The 12th is a Saturday; the weekly rule only covers Tuesdays.
	rules := []Rule{tuesdayEvenings(t), datedRule(t, 12, 9, 11)}

	got := expandSchedule(rules, zone, window(t, zone, "2026-09-07", "2026-09-16"))

	assertRanges(t, zone, got, "09-08 18:00–20:00", "09-12 09:00–11:00", "09-15 18:00–20:00")
}

func TestAnOverrideOutsideTheWindowChangesNothing(t *testing.T) {
	zone := berlin(t)
	rules := []Rule{tuesdayEvenings(t), datedRule(t, 29, 10, 12)}

	got := expandSchedule(rules, zone, window(t, zone, "2026-09-07", "2026-09-16"))

	assertRanges(t, zone, got, "09-08 18:00–20:00", "09-15 18:00–20:00")
}

func TestAnOverrideIsClippedToTheWindowLikeAnyRange(t *testing.T) {
	zone := berlin(t)
	rules := []Rule{datedRule(t, 15, 10, 12)}
	from := time.Date(2026, time.September, 15, 11, 0, 0, 0, zone)
	to := time.Date(2026, time.September, 16, 0, 0, 0, 0, zone)

	got := expandSchedule(rules, zone, Interval{Start: from, End: to})

	assertRanges(t, zone, got, "09-15 11:00–12:00")
}

// A date is a date in the MENTOR'S zone. A viewer elsewhere may be on the 16th while the
// mentor is still on the 15th, and it is the mentor's calendar the override closes.
func TestOverrideDatesAreResolvedInTheMentorsZone(t *testing.T) {
	zone := berlin(t)
	closure, err := NewDatedRule(mustDate(t, 2026, time.September, 15), mustTimeOfDay(t, 0, 0), mustTimeOfDay(t, 0, 0))
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}
	late, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 23, 0), mustTimeOfDay(t, 24, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	// 23:00 Berlin on the 15th is already the 16th in Tokyo; the closure still applies.
	got := expandSchedule([]Rule{late, closure}, zone, window(t, zone, "2026-09-15", "2026-09-16"))

	if len(got) != 0 {
		t.Errorf("got %v, want nothing — the closure covers the mentor's own date", localHours(zone, got))
	}
}

func TestExpandScheduleWithNoRulesOrNoZoneYieldsNothing(t *testing.T) {
	zone := berlin(t)
	w := window(t, zone, "2026-09-07", "2026-09-16")

	if got := expandSchedule(nil, zone, w); len(got) != 0 {
		t.Errorf("no rules yielded %v, want nothing", localHours(zone, got))
	}
	if got := expandSchedule([]Rule{tuesdayEvenings(t)}, nil, w); len(got) != 0 {
		t.Errorf("no zone yielded %d ranges, want nothing", len(got))
	}
}
