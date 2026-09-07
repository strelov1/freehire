package mentorship

import (
	"testing"
	"time"
)

// at builds an instant on 2026-09-15 UTC. Busy-time arithmetic is zone-independent —
// everything reaching it is already absolute — so these tests stay in UTC and the
// timezone cases live in expand_test.go where they belong.
func at(hour, minute int) time.Time {
	return time.Date(2026, time.September, 15, hour, minute, 0, 0, time.UTC)
}

func span(fromHour, fromMin, toHour, toMin int) Interval {
	return Interval{Start: at(fromHour, fromMin), End: at(toHour, toMin)}
}

func utcHours(ranges []Interval) []string {
	out := make([]string, 0, len(ranges))
	for _, iv := range ranges {
		out = append(out, iv.Start.UTC().Format("15:04")+"–"+iv.End.UTC().Format("15:04"))
	}
	return out
}

func assertSpans(t *testing.T, got []Interval, want ...string) {
	t.Helper()
	gotStrings := utcHours(got)
	if len(gotStrings) != len(want) {
		t.Fatalf("got %v, want %v", gotStrings, want)
	}
	for i := range want {
		if gotStrings[i] != want[i] {
			t.Errorf("range %d = %q, want %q (full: %v)", i, gotStrings[i], want[i], gotStrings)
		}
	}
}

// noBuffers isolates the subtraction itself; the buffer arithmetic is tested separately.
func noBuffers(t *testing.T) SessionParams {
	t.Helper()
	return validSession(t, SessionParams{
		Duration:      time.Hour,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	})
}

func withBuffers(t *testing.T, before, after time.Duration) SessionParams {
	t.Helper()
	return validSession(t, SessionParams{
		Duration:      time.Hour,
		BufferBefore:  before,
		BufferAfter:   after,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	})
}

func TestSubtractBusyCarvesAHoleInTheMiddle(t *testing.T) {
	free := []Interval{span(9, 0, 17, 0)}
	busy := []Interval{span(12, 0, 13, 0)}

	assertSpans(t, subtractBusy(free, busy, noBuffers(t)), "09:00–12:00", "13:00–17:00")
}

func TestSubtractBusyTrimsFromEitherEnd(t *testing.T) {
	free := []Interval{span(9, 0, 17, 0)}

	assertSpans(t, subtractBusy(free, []Interval{span(8, 0, 10, 0)}, noBuffers(t)), "10:00–17:00")
	assertSpans(t, subtractBusy(free, []Interval{span(16, 0, 18, 0)}, noBuffers(t)), "09:00–16:00")
}

func TestSubtractBusySwallowsAFullyCoveredRange(t *testing.T) {
	free := []Interval{span(9, 0, 11, 0), span(14, 0, 16, 0)}
	busy := []Interval{span(8, 0, 12, 0)}

	assertSpans(t, subtractBusy(free, busy, noBuffers(t)), "14:00–16:00")
}

// Half-open bounds again, on the other side of the engine: busy time that merely abuts
// free time takes nothing from it.
func TestSubtractBusyIgnoresAbuttingAndDisjointBusyTime(t *testing.T) {
	free := []Interval{span(18, 0, 20, 0)}

	for _, tc := range []struct {
		name string
		busy Interval
	}{
		{"abutting after", span(20, 0, 21, 0)},
		{"abutting before", span(17, 0, 18, 0)},
		{"disjoint", span(6, 0, 7, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSpans(t, subtractBusy(free, []Interval{tc.busy}, noBuffers(t)), "18:00–20:00")
		})
	}
}

// The buffer rule the spec states: the after-buffer of what is already booked and the
// before-buffer of what might be booked next BOTH apply, so the required gap is their
// sum. 15 after plus 10 before means nothing may start before 19:25.
func TestBuffersOnBothSidesAddUp(t *testing.T) {
	free := []Interval{span(9, 0, 22, 0)}
	busy := []Interval{span(18, 0, 19, 0)}

	got := subtractBusy(free, busy, withBuffers(t, 10*time.Minute, 15*time.Minute))

	assertSpans(t, got, "09:00–17:35", "19:25–22:00")
}

func TestBuffersWidenBusyTimeSymmetrically(t *testing.T) {
	free := []Interval{span(9, 0, 22, 0)}
	busy := []Interval{span(13, 0, 14, 0)}

	// A single 20-minute before-buffer and no after-buffer still widens both sides,
	// because the pair applied at each edge is (existing, prospective) either way round.
	got := subtractBusy(free, busy, withBuffers(t, 20*time.Minute, 0))

	assertSpans(t, got, "09:00–12:40", "14:20–22:00")
}

func TestBuffersCanCloseAGapEntirely(t *testing.T) {
	free := []Interval{span(9, 0, 17, 0)}
	busy := []Interval{span(11, 0, 12, 0), span(13, 0, 14, 0)}

	// 30 + 30 either side leaves 12:00+60 = 13:00 against 13:00−60 = 12:00: the gap is
	// gone, and the two holes merge into one.
	got := subtractBusy(free, busy, withBuffers(t, 30*time.Minute, 30*time.Minute))

	assertSpans(t, got, "09:00–10:00", "15:00–17:00")
}

func TestSubtractBusyHandlesOverlappingBusyRanges(t *testing.T) {
	free := []Interval{span(9, 0, 17, 0)}
	busy := []Interval{span(12, 0, 14, 0), span(13, 0, 15, 0), span(10, 0, 10, 30)}

	assertSpans(t, subtractBusy(free, busy, noBuffers(t)), "09:00–10:00", "10:30–12:00", "15:00–17:00")
}

func TestSubtractBusyWithNothingBusyReturnsFreeUnchanged(t *testing.T) {
	free := []Interval{span(9, 0, 11, 0), span(14, 0, 16, 0)}

	assertSpans(t, subtractBusy(free, nil, withBuffers(t, time.Hour, time.Hour)),
		"09:00–11:00", "14:00–16:00")
}

// A busy range of zero width is a cancelled booking's ghost or a malformed calendar row.
// With buffers it still blocks — the mentor asked for a gap around anything — but with
// none it takes nothing, because it occupies no time.
func TestAZeroWidthBusyRangeTakesNothingWithoutBuffers(t *testing.T) {
	free := []Interval{span(9, 0, 17, 0)}
	busy := []Interval{{Start: at(12, 0), End: at(12, 0)}}

	assertSpans(t, subtractBusy(free, busy, noBuffers(t)), "09:00–17:00")
}

func TestSubtractBusyReturnsRangesInOrder(t *testing.T) {
	free := []Interval{span(14, 0, 16, 0), span(9, 0, 11, 0)}

	got := subtractBusy(free, []Interval{span(10, 0, 10, 30)}, noBuffers(t))

	assertSpans(t, got, "09:00–10:00", "10:30–11:00", "14:00–16:00")
}
