package mentorship

import (
	"testing"
	"time"
)

// calendarRequest is a Berlin mentor available every weekday evening, asked about one
// week starting the Monday of mentorRequest's window — most tests vary one field of it.
func calendarRequest(t *testing.T) CalendarRequest {
	t.Helper()
	zone := berlin(t)

	var rules []Rule
	for _, day := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		rule, err := NewWeeklyRule(day, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0))
		if err != nil {
			t.Fatalf("NewWeeklyRule: %v", err)
		}
		rules = append(rules, rule)
	}

	params := validSession(t, SessionParams{
		Duration:      time.Hour,
		MinimumNotice: 2 * time.Hour,
		Horizon:       14 * 24 * time.Hour,
	})

	return CalendarRequest{
		Rules:      rules,
		MentorZone: zone,
		Params:     params,
		Now:        time.Date(2026, time.September, 7, 9, 0, 0, 0, zone), // Monday morning
		From:       time.Date(2026, time.September, 7, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.September, 14, 0, 0, 0, 0, zone),
	}
}

func mustCalendar(t *testing.T, req CalendarRequest) CalendarResult {
	t.Helper()
	got, err := Calendar(req)
	if err != nil {
		t.Fatalf("Calendar: %v", err)
	}
	return got
}

// statusAt finds the interval covering an instant, or fails — every instant in the
// requested window must be covered by exactly one interval.
func statusAt(t *testing.T, result CalendarResult, at time.Time) CalendarStatus {
	t.Helper()
	for _, iv := range result.Intervals {
		if !at.Before(iv.Start) && at.Before(iv.End) {
			return iv.Status
		}
	}
	t.Fatalf("no interval covers %v", at)
	return ""
}

func TestCalendarPartitionCoversTheWholeWindowWithNoGapsOrOverlaps(t *testing.T) {
	result := mustCalendar(t, calendarRequest(t))

	if len(result.Intervals) == 0 {
		t.Fatal("no intervals at all")
	}
	req := calendarRequest(t)
	if !result.Intervals[0].Start.Equal(req.From) {
		t.Errorf("first interval starts at %v, want %v", result.Intervals[0].Start, req.From)
	}
	if !result.Intervals[len(result.Intervals)-1].End.Equal(req.To) {
		t.Errorf("last interval ends at %v, want %v", result.Intervals[len(result.Intervals)-1].End, req.To)
	}
	for i := 1; i < len(result.Intervals); i++ {
		if !result.Intervals[i-1].End.Equal(result.Intervals[i].Start) {
			t.Errorf("gap or overlap between interval %d (ends %v) and %d (starts %v)",
				i-1, result.Intervals[i-1].End, i, result.Intervals[i].Start)
		}
	}
}

// A day with no configured availability at all is entirely closed.
func TestCalendarDayWithNoAvailabilityIsEntirelyClosed(t *testing.T) {
	req := calendarRequest(t)
	req.Rules = nil // no weekly rules, no overrides

	result := mustCalendar(t, req)

	if len(result.Intervals) != 1 {
		t.Fatalf("got %d intervals, want 1 (the whole window closed)", len(result.Intervals))
	}
	if result.Intervals[0].Status != StatusClosed {
		t.Errorf("Status = %v, want closed", result.Intervals[0].Status)
	}
}

// A booked hour is labeled booked, not busy or free — even when a synced busy interval
// happens to be requested for the exact same span.
func TestCalendarBookedHourIsLabeledBookedNotBusyOrFree(t *testing.T) {
	req := calendarRequest(t)
	zone := req.MentorZone
	bookingStart := time.Date(2026, time.September, 7, 18, 0, 0, 0, zone)
	req.Booked = []Interval{{Start: bookingStart, End: bookingStart.Add(time.Hour)}}

	result := mustCalendar(t, req)

	if got := statusAt(t, result, bookingStart.Add(30*time.Minute)); got != StatusBooked {
		t.Errorf("Status = %v, want booked", got)
	}
}

// A synced busy interval is labeled busy and the interval carries no extra detail — the
// domain type has no field for one, so this is really an API-shape guarantee, checked
// here by construction.
func TestCalendarSyncedBusyIntervalIsLabeledBusy(t *testing.T) {
	req := calendarRequest(t)
	zone := req.MentorZone
	busyStart := time.Date(2026, time.September, 7, 18, 0, 0, 0, zone)
	req.Busy = []Interval{{Start: busyStart, End: busyStart.Add(time.Hour)}}

	result := mustCalendar(t, req)

	if got := statusAt(t, result, busyStart.Add(30*time.Minute)); got != StatusBusy {
		t.Errorf("Status = %v, want busy", got)
	}
}

// A buffer-widened sliver around a booking is closed, not free — it is walled off, not
// actually occupied, and not offerable either.
func TestCalendarBufferWithheldIntervalIsClosedNotFree(t *testing.T) {
	req := calendarRequest(t)
	req.Params.BufferAfter = 30 * time.Minute
	zone := req.MentorZone
	bookingStart := time.Date(2026, time.September, 7, 18, 0, 0, 0, zone)
	req.Booked = []Interval{{Start: bookingStart, End: bookingStart.Add(time.Hour)}}

	result := mustCalendar(t, req)

	// 19:00-19:30 is inside the mentor's stated evening, immediately after the booking,
	// and withheld only by the after-buffer.
	widened := bookingStart.Add(time.Hour).Add(15 * time.Minute)
	if got := statusAt(t, result, widened); got != StatusClosed {
		t.Errorf("Status = %v, want closed (buffer-withheld)", got)
	}
}

// A past interval is never labeled free, regardless of the mentor's availability rules.
func TestCalendarPastIntervalIsNeverFree(t *testing.T) {
	req := calendarRequest(t)
	zone := req.MentorZone
	// Now is Monday 09:00; Sunday's whole day (before the window's availability even
	// starts) and Monday morning both precede it.
	past := time.Date(2026, time.September, 7, 6, 0, 0, 0, zone)

	result := mustCalendar(t, req)

	if got := statusAt(t, result, past); got == StatusFree {
		t.Errorf("Status = %v at %v, want anything but free", got, past)
	}
}

// TestCalendarPastIntervalIsNeverFree exercises a time that is BOTH before now and
// outside the mentor's stated hours, so it is closed either way and does not, on its
// own, prove the notice cutoff is what withholds it. This test isolates that: `now`
// sits INSIDE Monday's stated evening, so the 2-hour minimum notice — not the
// availability window — is what pushes the rest of Monday past the earliest offerable
// instant, while a later evening well past the notice period is unaffected.
func TestCalendarWithinTheNoticeWindowIsClosedNotFree(t *testing.T) {
	req := calendarRequest(t)
	zone := req.MentorZone
	req.Now = time.Date(2026, time.September, 7, 18, 30, 0, 0, zone) // mid-Monday-evening

	result := mustCalendar(t, req)

	withinNotice := time.Date(2026, time.September, 7, 19, 0, 0, 0, zone)
	if got := statusAt(t, result, withinNotice); got != StatusClosed {
		t.Errorf("Status = %v at %v (inside stated hours, inside the 2-hour notice window), want closed",
			got, withinNotice)
	}

	pastNotice := time.Date(2026, time.September, 8, 18, 0, 0, 0, zone)
	if got := statusAt(t, result, pastNotice); got != StatusFree {
		t.Errorf("Status = %v at %v (well past the notice window), want free", got, pastNotice)
	}
}

// The breakdown's free ranges are exactly what the public slot engine would offer for
// the same inputs — the whole point of reusing its pipeline.
func TestCalendarFreeRangesMatchWhatSlotsOffers(t *testing.T) {
	cReq := calendarRequest(t)
	cReq.Booked = []Interval{{
		Start: time.Date(2026, time.September, 8, 18, 0, 0, 0, cReq.MentorZone),
		End:   time.Date(2026, time.September, 8, 19, 0, 0, 0, cReq.MentorZone),
	}}

	sReq := SlotRequest{
		Rules:      cReq.Rules,
		MentorZone: cReq.MentorZone,
		Params:     cReq.Params,
		Busy:       cReq.Booked,
		From:       cReq.From,
		To:         cReq.To,
		Now:        cReq.Now,
		ViewerZone: "Europe/Berlin",
	}

	calResult := mustCalendar(t, cReq)
	slotResult := mustSlots(t, sReq)

	var freeFromCalendar []Interval
	for _, iv := range calResult.Intervals {
		if iv.Status == StatusFree {
			freeFromCalendar = append(freeFromCalendar, iv.Interval)
		}
	}

	if len(freeFromCalendar) != len(slotResult.Slots) {
		t.Fatalf("got %d free ranges from Calendar, %d slots from Slots, want equal",
			len(freeFromCalendar), len(slotResult.Slots))
	}
	for i := range slotResult.Slots {
		if !freeFromCalendar[i].Start.Equal(slotResult.Slots[i].Start) ||
			!freeFromCalendar[i].End.Equal(slotResult.Slots[i].End) {
			t.Errorf("free range %d = %+v, want %+v", i, freeFromCalendar[i], slotResult.Slots[i])
		}
	}
}

func TestCalendarRejectsAMentorWithNoZone(t *testing.T) {
	req := calendarRequest(t)
	req.MentorZone = nil

	_, err := Calendar(req)
	if err == nil {
		t.Fatal("want an error, got nil")
	}
}
