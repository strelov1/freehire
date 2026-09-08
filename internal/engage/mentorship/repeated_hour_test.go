package mentorship

import (
	"testing"
	"time"
)

// On the autumn transition an hour of wall-clock time happens TWICE, and the two
// occurrences are different instants a mentor is genuinely free for. This pins what the
// engine does with them, because it is the one place where "a slot" and "a time on a
// clock" stop being the same thing.
//
// The rule adopted here: no INSTANT is ever offered twice, and no real hour is withheld.
// A consequence is that two distinct slots can carry the same wall-clock label, which is
// a rendering problem rather than a scheduling one — the API must expose each slot's UTC
// offset so a seeker can tell them apart. Withholding one instead would silently delete
// an hour the mentor offered.
func TestTheRepeatedAutumnHourYieldsTwoDistinctInstants(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Sunday, mustTimeOfDay(t, 1, 0), mustTimeOfDay(t, 4, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	got := mustSlots(t, SlotRequest{
		Rules:      []Rule{rule},
		MentorZone: zone,
		Params:     validSession(t, SessionParams{Duration: time.Hour, Horizon: 30 * 24 * time.Hour}),
		Now:        time.Date(2026, time.October, 24, 0, 0, 0, 0, zone),
		From:       time.Date(2026, time.October, 25, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.October, 26, 0, 0, 0, 0, zone),
		ViewerZone: "Europe/Berlin",
	})

	// Four real hours pass between 01:00 and 04:00 local on this date, so four sessions
	// fit — one more than the clock face suggests.
	if len(got.Slots) != 4 {
		t.Fatalf("got %d slots, want 4 — the repeated hour is a real, bookable hour: %v",
			len(got.Slots), slotClocks(got.Slots))
	}

	seen := map[int64]bool{}
	for i, s := range got.Slots {
		if d := s.End.Sub(s.Start); d != time.Hour {
			t.Errorf("slot %d lasts %v of real time, want 1h", i, d)
		}
		if seen[s.Start.Unix()] {
			t.Errorf("slot %d repeats an instant already offered: %v", i, s.Start)
		}
		seen[s.Start.Unix()] = true
		if i > 0 && !got.Slots[i-1].Start.Before(s.Start) {
			t.Errorf("slot %d does not follow slot %d", i, i-1)
		}
	}
}

// The rendering consequence, pinned so nobody "fixes" it by deduplicating: two slots DO
// carry the same wall-clock label, and they are distinguishable only by their offset.
// Whatever renders these must show the offset, or a seeker sees "02:00" twice.
func TestTheRepeatedHourProducesTwoSlotsWithOneLabel(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Sunday, mustTimeOfDay(t, 1, 0), mustTimeOfDay(t, 4, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	got := mustSlots(t, SlotRequest{
		Rules:      []Rule{rule},
		MentorZone: zone,
		Params:     validSession(t, SessionParams{Duration: time.Hour, Horizon: 30 * 24 * time.Hour}),
		Now:        time.Date(2026, time.October, 24, 0, 0, 0, 0, zone),
		From:       time.Date(2026, time.October, 25, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.October, 26, 0, 0, 0, 0, zone),
		ViewerZone: "Europe/Berlin",
	})

	byLabel := map[string]int{}
	for _, s := range got.Slots {
		byLabel[s.Start.Format("15:04")]++
	}
	if byLabel["02:00"] != 2 {
		t.Errorf("02:00 appears %d times, want 2 — the hour repeats and both are bookable (labels: %v)",
			byLabel["02:00"], slotClocks(got.Slots))
	}

	// The offsets are what tell them apart, and the API must carry them.
	offsets := map[int]bool{}
	for _, s := range got.Slots {
		if s.Start.Format("15:04") == "02:00" {
			_, offset := s.Start.Zone()
			offsets[offset] = true
		}
	}
	if len(offsets) != 2 {
		t.Errorf("the two 02:00 slots share %d distinct UTC offsets, want 2 — nothing "+
			"would distinguish them for a seeker", len(offsets))
	}
}

// The spring mirror, at slot level rather than range level: the skipped hour must not
// produce a slot that is not a real hour.
func TestTheSkippedSpringHourProducesNoUnrealSlot(t *testing.T) {
	zone := berlin(t)
	rule, err := NewWeeklyRule(time.Sunday, mustTimeOfDay(t, 1, 0), mustTimeOfDay(t, 4, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}

	got := mustSlots(t, SlotRequest{
		Rules:      []Rule{rule},
		MentorZone: zone,
		Params:     validSession(t, SessionParams{Duration: time.Hour, Horizon: 30 * 24 * time.Hour}),
		Now:        time.Date(2026, time.March, 28, 0, 0, 0, 0, zone),
		From:       time.Date(2026, time.March, 29, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.March, 30, 0, 0, 0, 0, zone),
		ViewerZone: "Europe/Berlin",
	})

	// 01:00 to 04:00 local is only two real hours on this date, so only two sessions fit.
	if len(got.Slots) != 2 {
		t.Fatalf("got %d slots, want 2 — an hour of this window does not exist: %v",
			len(got.Slots), slotClocks(got.Slots))
	}
	for i, s := range got.Slots {
		if d := s.End.Sub(s.Start); d != time.Hour {
			t.Errorf("slot %d lasts %v of real time, want 1h", i, d)
		}
		// Every emitted instant must round-trip: a wall-clock time the zone skips would
		// not survive being formatted and parsed back.
		formatted := s.Start.Format("2006-01-02 15:04:05")
		back, err := time.ParseInLocation("2006-01-02 15:04:05", formatted, zone)
		if err != nil {
			t.Fatalf("ParseInLocation(%q): %v", formatted, err)
		}
		if !back.Equal(s.Start) {
			t.Errorf("slot %d at %s is not a real instant in %s: it re-reads as %s",
				i, formatted, zone, back)
		}
	}
}
