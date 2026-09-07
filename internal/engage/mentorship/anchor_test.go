package mentorship

import (
	"slices"
	"testing"
	"time"
)

// eveningMentor is a mentor free 18:00–22:00 every Tuesday in Berlin, hour-long sessions
// and no notice period, so that `now` is the only thing these tests vary.
func eveningMentor(t *testing.T, now time.Time) SlotRequest {
	t.Helper()
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 22, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	return SlotRequest{
		Rules:      []Rule{rule},
		MentorZone: zone,
		Params:     validSession(t, SessionParams{Duration: time.Hour, Horizon: 30 * 24 * time.Hour}),
		Now:        now,
		From:       time.Date(2026, time.September, 8, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.September, 9, 0, 0, 0, 0, zone),
		ViewerZone: "Europe/Berlin",
	}
}

func slotClocks(slots []Interval) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Start.Format("15:04"))
	}
	return out
}

// The grid belongs to the MENTOR'S schedule, not to the moment somebody happens to look.
// A visitor arriving at 18:33 must be offered 19:00, not 18:33 — three things break
// otherwise: the mentor's stated hours are not what is offered; the slot cache is keyed
// on (mentor, window, zone) with no `now` in it, so a cached grid would answer for a
// different minute; and the booking re-check runs with a later `now`, so the slot the
// seeker clicked would no longer exist when they submit it.
func TestTheSlotGridDoesNotMoveWithNow(t *testing.T) {
	zone := berlin(t)
	on := func(hour, minute int) time.Time {
		return time.Date(2026, time.September, 8, hour, minute, 0, 0, zone)
	}

	for _, tc := range []struct {
		name string
		now  time.Time
		want []string
	}{
		{"well before the mentor opens", on(9, 0), []string{"18:00", "19:00", "20:00", "21:00"}},
		{"one minute before", on(17, 59), []string{"18:00", "19:00", "20:00", "21:00"}},
		{"mid-session, on the half hour", on(18, 30), []string{"19:00", "20:00", "21:00"}},
		{"mid-session, on an awkward minute", on(18, 33), []string{"19:00", "20:00", "21:00"}},
		{"a different awkward minute", on(18, 47), []string{"19:00", "20:00", "21:00"}},
		{"late in the evening", on(20, 15), []string{"21:00"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mustSlots(t, eveningMentor(t, tc.now))

			if clocks := slotClocks(got.Slots); !slices.Equal(clocks, tc.want) {
				t.Errorf("at %s the mentor offers %v, want %v", tc.now.Format("15:04"), clocks, tc.want)
			}
		})
	}
}

// Stated as the property rather than as a table: every slot begins on the grid the
// mentor's own hours define, whatever minute the request arrives on.
func TestEverySlotStartsOnTheMentorsGridWhateverTheMinute(t *testing.T) {
	zone := berlin(t)

	for minute := 0; minute < 60; minute++ {
		now := time.Date(2026, time.September, 8, 18, minute, 0, 0, zone)
		got := mustSlots(t, eveningMentor(t, now))

		for _, s := range got.Slots {
			if s.Start.Minute() != 0 {
				t.Fatalf("at 18:%02d a slot starts at %s, off the mentor's hourly grid",
					minute, s.Start.Format("15:04"))
			}
		}
	}
}

// Nothing stops a mentor writing overlapping availability — the schema has no exclusion
// constraint on it, and "Mon–Fri 09:00–17:00 plus Tue 16:00–19:00" is an ordinary thing
// to type. Left unmerged, each range anchors its own grid and the same hour is offered
// more than once.
func TestOverlappingWeeklyRulesDoNotDuplicateSlots(t *testing.T) {
	zone := berlin(t)
	evening, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	overlapping, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 19, 0), mustTimeOfDay(t, 21, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	req := eveningMentor(t, time.Date(2026, time.September, 8, 9, 0, 0, 0, zone))
	req.Rules = []Rule{evening, overlapping}

	got := mustSlots(t, req)

	if clocks := slotClocks(got.Slots); !slices.Equal(clocks, []string{"18:00", "19:00", "20:00"}) {
		t.Errorf("overlapping rules offered %v, want 18:00 19:00 20:00", clocks)
	}
}

func TestDuplicateWeeklyRulesDoNotDuplicateSlots(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	req := eveningMentor(t, time.Date(2026, time.September, 8, 9, 0, 0, 0, zone))
	req.Rules = []Rule{rule, rule}

	got := mustSlots(t, req)

	if clocks := slotClocks(got.Slots); !slices.Equal(clocks, []string{"18:00", "19:00"}) {
		t.Errorf("a rule written twice offered %v, want 18:00 19:00", clocks)
	}
}

func TestOverlappingDatedOverridesDoNotDuplicateSlots(t *testing.T) {
	zone := berlin(t)
	req := eveningMentor(t, time.Date(2026, time.September, 8, 0, 0, 0, 0, zone))
	req.Rules = []Rule{datedRule(t, 8, 10, 12), datedRule(t, 8, 11, 13)}

	got := mustSlots(t, req)

	if clocks := slotClocks(got.Slots); !slices.Equal(clocks, []string{"10:00", "11:00", "12:00"}) {
		t.Errorf("overlapping overrides offered %v, want 10:00 11:00 12:00", clocks)
	}
}

// Slots are ascending. Asserted on a fixture that actually has out-of-order inputs —
// the existing determinism fixture has none, which is why it never caught this.
func TestSlotsAreAscendingEvenWhenRulesAreNot(t *testing.T) {
	zone := berlin(t)
	late, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 20, 0), mustTimeOfDay(t, 22, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	early, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 9, 0), mustTimeOfDay(t, 11, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	req := eveningMentor(t, time.Date(2026, time.September, 8, 0, 0, 0, 0, zone))
	req.Rules = []Rule{late, datedRule(t, 8, 14, 16), early}

	got := mustSlots(t, req)

	if len(got.Slots) == 0 {
		t.Fatal("no slots")
	}
	for i := 1; i < len(got.Slots); i++ {
		if !got.Slots[i-1].Start.Before(got.Slots[i].Start) {
			t.Errorf("slots %d and %d are out of order: %v", i-1, i, slotClocks(got.Slots))
		}
	}
}

// The viewer's zone is not a free-text field the server will act on. "Local" resolves in
// Go and yields the SERVER's clock — the same undetectable failure as falling back to the
// mentor's zone, by a different door.
func TestAViewerZoneOfLocalFallsBackToUTC(t *testing.T) {
	zone := berlin(t)
	req := eveningMentor(t, time.Date(2026, time.September, 8, 9, 0, 0, 0, zone))

	for _, name := range []string{"Local", "EST", "Factory", "posixrules"} {
		t.Run(name, func(t *testing.T) {
			req.ViewerZone = name
			got := mustSlots(t, req)

			if got.Zone != "UTC" {
				t.Errorf("Zone = %q, want UTC", got.Zone)
			}
			if len(got.Slots) > 0 && got.Slots[0].Start.Location() != time.UTC {
				t.Errorf("slot location = %v, want UTC", got.Slots[0].Start.Location())
			}
		})
	}
}

// The horizon's own boundary, the twin of TestASlotExactlyAtTheNoticeBoundaryIsOffered.
// The spec bounds when a slot may BEGIN, so one starting exactly on the horizon is
// offerable — which is what the session-length slack on the window's far end is for.
func TestASlotExactlyAtTheHorizonIsOfferedAndOneBeyondIsNot(t *testing.T) {
	zone := berlin(t)
	req := eveningMentor(t, time.Date(2026, time.September, 8, 9, 0, 0, 0, zone))
	req.To = req.From.AddDate(0, 0, 30)

	// The horizon lands exactly on the 21:00 slot; the 20:00 one is comfortably inside.
	req.Params.Horizon = time.Date(2026, time.September, 8, 21, 0, 0, 0, zone).Sub(req.Now)
	got := mustSlots(t, req)
	if clocks := slotClocks(got.Slots); !slices.Contains(clocks, "21:00") {
		t.Errorf("a slot beginning exactly on the horizon was withheld: %v", clocks)
	}

	// One minute earlier and the 21:00 slot is past the horizon.
	req.Params.Horizon -= time.Minute
	got = mustSlots(t, req)
	if clocks := slotClocks(got.Slots); slices.Contains(clocks, "21:00") {
		t.Errorf("a slot beginning past the horizon was offered: %v", clocks)
	}
}
