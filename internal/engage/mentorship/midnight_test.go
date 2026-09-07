package mentorship

import (
	"testing"
	"time"
)

// Three live zones move their clocks AT MIDNIGHT, so on one date a year 00:00 local does
// not exist. Go's time.Date normalises such a time BACKWARDS — to 23:00 on the previous
// date — which means a day cursor built at midnight can fail to advance at all.
//
// Each of these recurs annually, and the slot endpoint is unauthenticated on a host whose
// traffic is mostly crawlers, so one mentor in Santiago is a permanently pinned core
// rather than a slow page.
var midnightTransitions = []struct {
	zone string
	date string // the local date whose midnight does not exist
}{
	{"America/Santiago", "2026-09-05"},
	{"America/Havana", "2026-03-07"},
	{"Atlantic/Azores", "2026-03-28"},
}

// The bug, stated directly: the day cursor must always advance. It is a Date rather than
// an instant precisely so that this cannot depend on a zone at all — but the walk starts
// from a date derived from one, so the transition dates are still the cases to walk.
func TestTheDayCursorAlwaysAdvances(t *testing.T) {
	for _, tc := range midnightTransitions {
		t.Run(tc.zone, func(t *testing.T) {
			zone, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("zone %s not in this host's tzdata", tc.zone)
			}
			start, err := time.ParseInLocation(time.DateOnly, tc.date, zone)
			if err != nil {
				t.Fatalf("ParseInLocation: %v", err)
			}

			// Walk a week across the transition. Each step must land on the next
			// calendar date, and each date must sort after the one before it.
			day := dateOf(start.AddDate(0, 0, -2), zone)
			for i := range 7 {
				next := day.next()
				if !next.after(day) {
					t.Fatalf("step %d: %s.next() = %s, which does not advance", i, day, next)
				}
				want := time.Date(day.year, day.month, day.day+1, 0, 0, 0, 0, time.UTC)
				if next.String() != want.Format(time.DateOnly) {
					t.Errorf("step %d: %s.next() = %s, want %s", i, day, next, want.Format(time.DateOnly))
				}
				day = next
			}
		})
	}
}

// A Date is a plain calendar date, so stepping one must not consult a zone at all. Both
// month and year rollovers, and a leap day, in one pass.
func TestDateNextRollsOverMonthsAndYears(t *testing.T) {
	for _, tc := range []struct{ from, want string }{
		{"2026-09-15", "2026-09-16"},
		{"2026-09-30", "2026-10-01"},
		{"2026-12-31", "2027-01-01"},
		{"2028-02-28", "2028-02-29"},
		{"2028-02-29", "2028-03-01"},
		{"2026-02-28", "2026-03-01"},
	} {
		t.Run(tc.from, func(t *testing.T) {
			at, err := time.Parse(time.DateOnly, tc.from)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			day, err := NewDate(at.Year(), at.Month(), at.Day())
			if err != nil {
				t.Fatalf("NewDate: %v", err)
			}
			if got := day.next(); got.String() != tc.want {
				t.Errorf("%s.next() = %s, want %s", day, got, tc.want)
			}
		})
	}
}

// The same defect seen from the override side: a dated rule must resolve onto the date it
// names, not the one before it.
func TestADatedOverrideLandsOnItsOwnDateEvenWhenMidnightIsMissing(t *testing.T) {
	for _, tc := range midnightTransitions {
		t.Run(tc.zone, func(t *testing.T) {
			zone, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("zone %s not in this host's tzdata", tc.zone)
			}
			named, err := time.ParseInLocation(time.DateOnly, tc.date, zone)
			if err != nil {
				t.Fatalf("ParseInLocation: %v", err)
			}
			date, err := NewDate(named.Year(), named.Month(), named.Day())
			if err != nil {
				t.Fatalf("NewDate: %v", err)
			}
			rule, err := NewDatedRule(date, mustTimeOfDay(t, 10, 0), mustTimeOfDay(t, 12, 0))
			if err != nil {
				t.Fatalf("NewDatedRule: %v", err)
			}

			w := Interval{Start: named.AddDate(0, 0, -1), End: named.AddDate(0, 0, 2)}
			got := expandSchedule([]Rule{rule}, zone, w)

			if len(got) != 1 {
				t.Fatalf("got %d ranges, want 1", len(got))
			}
			if local := got[0].Start.In(zone); local.Format(time.DateOnly) != tc.date {
				t.Errorf("an override for %s resolved onto %s", tc.date, local.Format(time.DateOnly))
			}
		})
	}
}

// The end-to-end symptom: a whole request must terminate. Guarded by the test binary's
// own -timeout, which turns the hang into a failure rather than a hung suite.
func TestSlotsTerminateInAZoneWhoseMidnightIsMissing(t *testing.T) {
	for _, tc := range midnightTransitions {
		t.Run(tc.zone, func(t *testing.T) {
			zone, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("zone %s not in this host's tzdata", tc.zone)
			}
			transition, err := time.ParseInLocation(time.DateOnly, tc.date, zone)
			if err != nil {
				t.Fatalf("ParseInLocation: %v", err)
			}
			rule, err := NewWeeklyRule(transition.Weekday(), mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
			if err != nil {
				t.Fatalf("NewWeeklyRule: %v", err)
			}

			now := transition.AddDate(0, 0, -2)
			done := make(chan int, 1)
			go func() {
				got, err := Slots(SlotRequest{
					Rules:      []Rule{rule},
					MentorZone: zone,
					Params:     validSession(t, SessionParams{Duration: time.Hour, Horizon: 30 * 24 * time.Hour}),
					From:       now,
					To:         now.AddDate(0, 0, 7),
					Now:        now,
					ViewerZone: tc.zone,
				})
				if err != nil {
					t.Errorf("Slots: %v", err)
				}
				done <- len(got.Slots)
			}()

			select {
			case n := <-done:
				if n == 0 {
					t.Error("the week across the transition offered no slots at all")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Slots did not return within 5s — the day cursor is not advancing")
			}
		})
	}
}
