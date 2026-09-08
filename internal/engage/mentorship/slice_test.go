package mentorship

import (
	"testing"
	"time"
)

func session(t *testing.T, d time.Duration) SessionParams {
	t.Helper()
	return validSession(t, SessionParams{
		Duration:      d,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	})
}

func TestSliceCutsAFreeRangeIntoWholeSessions(t *testing.T) {
	free := []Interval{span(18, 0, 20, 0)}

	assertSpans(t, sliceSlots(free, session(t, time.Hour)), "18:00–19:00", "19:00–20:00")
}

// The half-open rule seen from the slicing side: two sessions that meet at an instant
// are two slots, not one overlapping pair, and the database constraint agrees.
func TestSliceEmitsBackToBackSlotsThatDoNotOverlap(t *testing.T) {
	got := sliceSlots([]Interval{span(18, 0, 20, 0)}, session(t, time.Hour))

	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2", len(got))
	}
	if !got[0].End.Equal(got[1].Start) {
		t.Errorf("slots do not meet: %v ends, %v starts", got[0].End, got[1].Start)
	}
	if got[0].Overlaps(got[1]) {
		t.Error("back-to-back slots report themselves as overlapping")
	}
}

// The rule that matters most to a mentor: a session is offered only where the WHOLE of
// it fits. A tail too short for another session is not a short session.
func TestSliceDropsATailTooShortForASession(t *testing.T) {
	free := []Interval{span(18, 0, 19, 45)}

	assertSpans(t, sliceSlots(free, session(t, time.Hour)), "18:00–19:00")
}

func TestSliceYieldsNothingWhenTheSessionCannotFit(t *testing.T) {
	for _, tc := range []struct {
		name string
		free Interval
	}{
		{"a gap shorter than the session", span(18, 0, 18, 45)},
		{"a gap one minute short", span(18, 0, 18, 59)},
		{"an empty range", span(18, 0, 18, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sliceSlots([]Interval{tc.free}, session(t, time.Hour)); len(got) != 0 {
				t.Errorf("got %v, want nothing", utcHours(got))
			}
		})
	}
}

func TestSliceFitsASessionExactlyFillingItsRange(t *testing.T) {
	free := []Interval{span(18, 0, 19, 0)}

	assertSpans(t, sliceSlots(free, session(t, time.Hour)), "18:00–19:00")
}

// The grid is anchored to each free range's own start, so a mentor stating 18:00 gets
// slots at 18:00 — not at whatever minute the previous range happened to end on.
func TestSliceAnchorsEachRangeToItsOwnStart(t *testing.T) {
	free := []Interval{span(9, 15, 11, 15), span(18, 0, 20, 0)}

	assertSpans(t, sliceSlots(free, session(t, time.Hour)),
		"09:15–10:15", "10:15–11:15", "18:00–19:00", "19:00–20:00")
}

func TestSliceHandlesASessionShorterThanAnHour(t *testing.T) {
	free := []Interval{span(18, 0, 19, 30)}

	assertSpans(t, sliceSlots(free, session(t, 30*time.Minute)),
		"18:00–18:30", "18:30–19:00", "19:00–19:30")
}

func TestSliceReturnsNothingForNoRanges(t *testing.T) {
	if got := sliceSlots(nil, session(t, time.Hour)); len(got) != 0 {
		t.Errorf("got %v, want nothing", utcHours(got))
	}
}

// A range spanning a daylight-saving transition holds one hour fewer (or more) of real
// time than its wall clock suggests, and the slicing counts real time — a 90-minute
// session must not be offered in what is really 60 minutes.
func TestSliceCountsRealTimeAcrossADaylightSavingTransition(t *testing.T) {
	zone := berlin(t)
	// 01:00–03:00 local on the spring transition date is one real hour.
	from := time.Date(2026, time.March, 29, 1, 0, 0, 0, zone)
	to := time.Date(2026, time.March, 29, 3, 0, 0, 0, zone)
	free := []Interval{{Start: from, End: to}}

	if got := sliceSlots(free, session(t, 90*time.Minute)); len(got) != 0 {
		t.Errorf("a 90-minute session fitted into one real hour: %v", utcHours(got))
	}
	if got := sliceSlots(free, session(t, time.Hour)); len(got) != 1 {
		t.Errorf("got %d one-hour slots in one real hour, want 1", len(got))
	}
}
