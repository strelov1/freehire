package mentorship

import (
	"testing"
	"time"
)

// mentorRequest is the shape most of these tests vary one field of: a Berlin mentor
// available every weekday evening, an hour a session, asked about a fortnight.
func mentorRequest(t *testing.T) SlotRequest {
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

	return SlotRequest{
		Rules:      rules,
		MentorZone: zone,
		Params:     params,
		Now:        time.Date(2026, time.September, 7, 9, 0, 0, 0, zone), // Monday morning
		From:       time.Date(2026, time.September, 7, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.September, 21, 0, 0, 0, 0, zone),
		ViewerZone: "Europe/Berlin",
	}
}

func mustSlots(t *testing.T, req SlotRequest) SlotResult {
	t.Helper()
	got, err := Slots(req)
	if err != nil {
		t.Fatalf("Slots: %v", err)
	}
	return got
}

func TestSlotsOffersTheMentorsEveningsInTheViewersZone(t *testing.T) {
	got := mustSlots(t, mentorRequest(t))

	if got.Zone != "Europe/Berlin" {
		t.Errorf("Zone = %q, want Europe/Berlin", got.Zone)
	}
	if len(got.Slots) == 0 {
		t.Fatal("no slots at all")
	}
	for _, s := range got.Slots {
		if h := s.Start.Hour(); h != 18 && h != 19 {
			t.Errorf("slot starts at %02d:00, want 18:00 or 19:00 in the viewer's zone", h)
		}
	}
}

// The viewer's zone changes how a slot READS and never which instant it is.
func TestSlotsInAnotherZoneAreTheSameInstants(t *testing.T) {
	req := mentorRequest(t)
	inBerlin := mustSlots(t, req)

	req.ViewerZone = "Asia/Tokyo"
	inTokyo := mustSlots(t, req)

	if inTokyo.Zone != "Asia/Tokyo" {
		t.Errorf("Zone = %q, want Asia/Tokyo", inTokyo.Zone)
	}
	if len(inTokyo.Slots) != len(inBerlin.Slots) {
		t.Fatalf("got %d slots in Tokyo and %d in Berlin, want the same count", len(inTokyo.Slots), len(inBerlin.Slots))
	}
	for i := range inBerlin.Slots {
		if !inTokyo.Slots[i].Start.Equal(inBerlin.Slots[i].Start) {
			t.Errorf("slot %d is a different instant: %v vs %v", i, inTokyo.Slots[i].Start, inBerlin.Slots[i].Start)
		}
	}
	// 18:00 Berlin in summer is 01:00 the next day in Tokyo.
	if h := inTokyo.Slots[0].Start.Hour(); h != 1 && h != 2 {
		t.Errorf("the first Tokyo slot reads %02d:00, want the small hours", h)
	}
}

// A zone we cannot resolve must not silently become the mentor's — that would show a
// visitor times that are not theirs while looking entirely plausible.
func TestSlotsFallBackToUTCAndSayWhichZoneTheyUsed(t *testing.T) {
	for _, name := range []string{"", "Mars/Olympus_Mons", "not a zone"} {
		t.Run("zone "+name, func(t *testing.T) {
			req := mentorRequest(t)
			req.ViewerZone = name

			got := mustSlots(t, req)

			if got.Zone != "UTC" {
				t.Errorf("Zone = %q, want UTC", got.Zone)
			}
			if len(got.Slots) == 0 {
				t.Fatal("the fallback returned no slots")
			}
			if loc := got.Slots[0].Start.Location(); loc != time.UTC {
				t.Errorf("slot location = %v, want UTC", loc)
			}
		})
	}
}

func TestSlotsWithheldInsideTheNoticePeriod(t *testing.T) {
	req := mentorRequest(t)
	// Monday 17:30, half an hour before the mentor opens: with two hours' notice, nothing
	// that evening is offerable, and Tuesday is the first day with slots.
	req.Now = time.Date(2026, time.September, 7, 17, 30, 0, 0, req.MentorZone)

	got := mustSlots(t, req)

	if len(got.Slots) == 0 {
		t.Fatal("no slots at all")
	}
	if day := got.Slots[0].Start.Day(); day != 8 {
		t.Errorf("the first slot is on the %dth, want the 8th — Monday is inside the notice period", day)
	}
}

// The boundary belongs to the mentor: a slot starting exactly when the notice period
// elapses is offerable, because the notice has in fact elapsed.
func TestASlotExactlyAtTheNoticeBoundaryIsOffered(t *testing.T) {
	req := mentorRequest(t)
	req.Now = time.Date(2026, time.September, 7, 16, 0, 0, 0, req.MentorZone) // 18:00 minus two hours

	got := mustSlots(t, req)

	if len(got.Slots) == 0 {
		t.Fatal("no slots at all")
	}
	first := got.Slots[0]
	if first.Start.Day() != 7 || first.Start.Hour() != 18 {
		t.Errorf("first slot = %v, want Monday 18:00", first.Start)
	}
}

func TestSlotsAreClampedToTheHorizonRatherThanRefused(t *testing.T) {
	req := mentorRequest(t)
	req.Params.Horizon = 3 * 24 * time.Hour
	req.To = req.From.AddDate(0, 0, 90) // far past the horizon

	got := mustSlots(t, req)

	if len(got.Slots) == 0 {
		t.Fatal("a window wider than the horizon returned nothing, want it clamped")
	}
	limit := req.Now.Add(req.Params.Horizon)
	for _, s := range got.Slots {
		if s.Start.After(limit) {
			t.Errorf("slot at %v starts past the horizon %v", s.Start, limit)
		}
	}
}

func TestSlotsInThePastAreNeverOffered(t *testing.T) {
	req := mentorRequest(t)
	req.From = req.From.AddDate(0, 0, -30)

	got := mustSlots(t, req)

	for _, s := range got.Slots {
		if s.Start.Before(req.Now) {
			t.Errorf("slot at %v is in the past", s.Start)
		}
	}
}

func TestSlotsSubtractBusyTime(t *testing.T) {
	req := mentorRequest(t)
	// Book the whole of Tuesday evening.
	req.Busy = []Interval{{
		Start: time.Date(2026, time.September, 8, 18, 0, 0, 0, req.MentorZone),
		End:   time.Date(2026, time.September, 8, 20, 0, 0, 0, req.MentorZone),
	}}

	got := mustSlots(t, req)

	for _, s := range got.Slots {
		if s.Start.Month() == time.September && s.Start.Day() == 8 {
			t.Errorf("slot at %v survived a booking covering that whole evening", s.Start)
		}
	}
}

func TestSlotsRefuseParametersThatCannotYieldASlot(t *testing.T) {
	req := mentorRequest(t)
	req.Params.Duration = 0

	if _, err := Slots(req); err == nil {
		t.Error("Slots accepted a zero duration")
	}
}

func TestSlotsWithoutAMentorZoneAreRefused(t *testing.T) {
	req := mentorRequest(t)
	req.MentorZone = nil

	if _, err := Slots(req); err == nil {
		t.Error("Slots accepted a request with no mentor zone")
	}
}

func TestSlotsForAnEmptyOrBackwardsWindowAreEmptyNotAnError(t *testing.T) {
	req := mentorRequest(t)
	req.To = req.From

	got := mustSlots(t, req)
	if len(got.Slots) != 0 {
		t.Errorf("an empty window yielded %d slots", len(got.Slots))
	}
	if got.Zone != "Europe/Berlin" {
		t.Errorf("Zone = %q, want the viewer's zone even with no slots", got.Zone)
	}
}
