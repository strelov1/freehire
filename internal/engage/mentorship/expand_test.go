package mentorship

import (
	"testing"
	"time"
)

// Berlin is the zone every timezone test here uses, because its transitions are the
// ordinary European ones and both pathological cases fall on a date we can name:
// 2026-03-29, when 02:00–03:00 local does not happen, and 2026-10-25, when 02:00–03:00
// local happens twice.
func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation(Europe/Berlin): %v", err)
	}
	return loc
}

func window(t *testing.T, zone *time.Location, fromDay, toDay string) Interval {
	t.Helper()
	parse := func(day string) time.Time {
		at, err := time.ParseInLocation(time.DateOnly, day, zone)
		if err != nil {
			t.Fatalf("ParseInLocation(%q): %v", day, err)
		}
		return at
	}
	return Interval{Start: parse(fromDay), End: parse(toDay)}
}

func TestExpandWeeklyRepeatsAcrossTheWindow(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	// Monday 7 September through Monday 5 October: four Tuesdays.
	got := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-09-07", "2026-10-05"))

	if len(got) != 4 {
		t.Fatalf("got %d ranges, want 4 Tuesdays", len(got))
	}
	for _, iv := range got {
		local := iv.Start.In(zone)
		if local.Weekday() != time.Tuesday {
			t.Errorf("range starts on %v, want Tuesday", local.Weekday())
		}
		if local.Hour() != 18 || local.Minute() != 0 {
			t.Errorf("range starts at %02d:%02d local, want 18:00", local.Hour(), local.Minute())
		}
		if d := iv.End.Sub(iv.Start); d != 2*time.Hour {
			t.Errorf("range lasts %v, want 2h", d)
		}
	}
}

// The claim daylight saving actually makes: the mentor's stated hour does not move, so
// the UTC instant behind it does. A rule expanded once into UTC and repeated would fail
// exactly here, and only for half the year.
func TestExpandWeeklyKeepsTheStatedHourAcrossTheTransition(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	// Two Tuesdays either side of the spring transition on 2026-03-29.
	got := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-03-24", "2026-04-01"))

	if len(got) != 2 {
		t.Fatalf("got %d ranges, want 2 Tuesdays", len(got))
	}
	for _, iv := range got {
		if local := iv.Start.In(zone); local.Hour() != 18 {
			t.Errorf("%s starts at %02d:00 local, want 18:00", local.Format(time.DateOnly), local.Hour())
		}
	}
	beforeUTC, afterUTC := got[0].Start.UTC().Hour(), got[1].Start.UTC().Hour()
	if beforeUTC != 17 {
		t.Errorf("the Tuesday before the transition starts at %02d:00 UTC, want 17:00", beforeUTC)
	}
	if afterUTC != 16 {
		t.Errorf("the Tuesday after the transition starts at %02d:00 UTC, want 16:00", afterUTC)
	}
}

// On the spring transition date an hour of local wall-clock time does not exist, so a
// window stated across it is shorter in real time than the clock suggests. The engine
// must produce a real interval, not a three-hour one that never happened.
func TestExpandWeeklyLosesAnHourOnTheSpringTransition(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Sunday, mustTimeOfDay(t, 1, 0), mustTimeOfDay(t, 4, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	transition := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-03-29", "2026-03-30"))
	ordinary := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-03-22", "2026-03-23"))

	if len(transition) != 1 || len(ordinary) != 1 {
		t.Fatalf("got %d transition and %d ordinary ranges, want 1 each", len(transition), len(ordinary))
	}
	if d := ordinary[0].End.Sub(ordinary[0].Start); d != 3*time.Hour {
		t.Errorf("an ordinary Sunday 01:00–04:00 lasts %v, want 3h", d)
	}
	if d := transition[0].End.Sub(transition[0].Start); d != 2*time.Hour {
		t.Errorf("the spring-transition Sunday 01:00–04:00 lasts %v, want 2h — an hour does not exist", d)
	}
}

// The autumn mirror: an hour happens twice, so the same stated window is longer.
func TestExpandWeeklyGainsAnHourOnTheAutumnTransition(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Sunday, mustTimeOfDay(t, 1, 0), mustTimeOfDay(t, 4, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	got := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-10-25", "2026-10-26"))

	if len(got) != 1 {
		t.Fatalf("got %d ranges, want 1", len(got))
	}
	if d := got[0].End.Sub(got[0].Start); d != 4*time.Hour {
		t.Errorf("the autumn-transition Sunday 01:00–04:00 lasts %v, want 4h — an hour repeats", d)
	}
}

// A rule reaching 24:00 ends at midnight and does not spill onto the next date, which
// is the whole reason TimeOfDay admits 24:00 as an end and nothing past it.
func TestExpandWeeklyEndsAtMidnightWithoutSpilling(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 22, 0), mustTimeOfDay(t, 24, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	got := expandWeekly([]Rule{rule}, zone, window(t, zone, "2026-09-08", "2026-09-09"))

	if len(got) != 1 {
		t.Fatalf("got %d ranges, want 1", len(got))
	}
	end := got[0].End.In(zone)
	if end.Hour() != 0 || end.Minute() != 0 {
		t.Errorf("range ends at %02d:%02d local, want midnight", end.Hour(), end.Minute())
	}
	if end.Day() != 9 {
		t.Errorf("range ends on day %d, want the 9th — midnight belongs to the next date", end.Day())
	}
}

func TestExpandWeeklyClipsToTheWindow(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	// A window opening mid-session: 19:00 on the Tuesday to the next morning.
	from := time.Date(2026, time.September, 8, 19, 0, 0, 0, zone)
	to := time.Date(2026, time.September, 9, 9, 0, 0, 0, zone)

	got := expandWeekly([]Rule{rule}, zone, Interval{Start: from, End: to})

	if len(got) != 1 {
		t.Fatalf("got %d ranges, want 1", len(got))
	}
	if !got[0].Start.Equal(from) {
		t.Errorf("range starts at %v, want the window's own start %v", got[0].Start, from)
	}
	if d := got[0].End.Sub(got[0].Start); d != time.Hour {
		t.Errorf("clipped range lasts %v, want 1h", d)
	}
}

func TestExpandWeeklyIgnoresDatedRulesAndEmptyWindows(t *testing.T) {
	zone := berlin(t)
	weekly, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	dated, err := NewDatedRule(mustDate(t, 2026, time.September, 8), mustTimeOfDay(t, 10, 0), mustTimeOfDay(t, 12, 0))
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}

	// The dated rule belongs to the override pass, not this one.
	got := expandWeekly([]Rule{weekly, dated}, zone, window(t, zone, "2026-09-08", "2026-09-09"))
	if len(got) != 1 {
		t.Fatalf("got %d ranges, want only the weekly one", len(got))
	}

	empty := window(t, zone, "2026-09-08", "2026-09-08")
	if got := expandWeekly([]Rule{weekly}, zone, empty); len(got) != 0 {
		t.Errorf("an empty window yielded %d ranges, want none", len(got))
	}
}

func TestExpandWeeklyReturnsRangesInOrder(t *testing.T) {
	zone := berlin(t)
	evening, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	morning, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 9, 0), mustTimeOfDay(t, 11, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	// Deliberately supplied evening-first, to prove the engine orders its output.
	got := expandWeekly([]Rule{evening, morning}, zone, window(t, zone, "2026-09-08", "2026-09-09"))

	if len(got) != 2 {
		t.Fatalf("got %d ranges, want 2", len(got))
	}
	if !got[0].Start.Before(got[1].Start) {
		t.Errorf("ranges are not ascending: %v then %v", got[0].Start, got[1].Start)
	}
}
