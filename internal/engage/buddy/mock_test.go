package buddy

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

func TestValidateMockSettings(t *testing.T) {
	got, err := validateMockSettings(MockSettings{
		Timezone:   "Europe/Berlin",
		MeetingURL: "  https://meet.example.test/abc  ",
		Topics:     []string{TopicBehavioral, TopicCoding, TopicCoding},
	})
	if err != nil {
		t.Fatalf("valid settings refused: %v", err)
	}
	if got.MeetingURL != "https://meet.example.test/abc" {
		t.Errorf("link = %q, want it trimmed", got.MeetingURL)
	}
	if want := []string{TopicCoding, TopicBehavioral}; !slices.Equal(got.Topics, want) {
		t.Errorf("topics = %v, want %v (vocabulary order, no duplicates)", got.Topics, want)
	}

	link := "https://meet.example.test/abc"
	for name, in := range map[string]MockSettings{
		"unknown zone":      {Timezone: "Mars/Olympus", MeetingURL: link, Topics: []string{TopicCoding}},
		"server zone":       {Timezone: "Local", MeetingURL: link, Topics: []string{TopicCoding}},
		"abbreviation":      {Timezone: "EST", MeetingURL: link, Topics: []string{TopicCoding}},
		"no zone":           {Timezone: "", MeetingURL: link, Topics: []string{TopicCoding}},
		"plain http link":   {Timezone: "UTC", MeetingURL: "http://meet.example.test/abc", Topics: []string{TopicCoding}},
		"no host":           {Timezone: "UTC", MeetingURL: "https://", Topics: []string{TopicCoding}},
		"user and password": {Timezone: "UTC", MeetingURL: "https://user:pass@meet.example.test/abc", Topics: []string{TopicCoding}},
		"user only":         {Timezone: "UTC", MeetingURL: "https://user@meet.example.test/abc", Topics: []string{TopicCoding}},
		"too long":          {Timezone: "UTC", MeetingURL: "https://meet.example.test/" + strings.Repeat("a", 500), Topics: []string{TopicCoding}},
		"no topics":         {Timezone: "UTC", MeetingURL: link},
		"unknown topic":     {Timezone: "UTC", MeetingURL: link, Topics: []string{TopicCoding, "karaoke"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateMockSettings(in); !errors.Is(err, ErrInvalidMockSettings) {
				t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
			}
		})
	}
}

func weeklyRule(t *testing.T, day time.Weekday, fromHour, toHour int) slots.Rule {
	t.Helper()
	start, err := slots.NewTimeOfDay(fromHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	end, err := slots.NewTimeOfDay(toHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := slots.NewWeeklyRule(day, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

// Tuesdays 19:00-21:00 in Berlin, listed by a viewer in New York from 15 October to
// 5 November 2026: Berlin leaves summer time on 25 October, New York on 1 November, and
// every slot must still begin at 19:00 or 20:00 on the HOST's wall clock.
func TestMockSlotsKeepTheHostsWallClockAcrossDST(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)

	res, err := mockSlots([]slots.Rule{weeklyRule(t, time.Tuesday, 19, 21)}, berlin, nil, now, to, now, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "America/New_York" {
		t.Errorf("zone = %q, want the viewer's", res.Zone)
	}
	var days []string
	for _, s := range res.Slots {
		local := s.Start.In(berlin)
		if h := local.Hour(); (h != 19 && h != 20) || local.Minute() != 0 {
			t.Errorf("slot %v starts at %02d:%02d in Berlin", s.Start, h, local.Minute())
		}
		if s.End.Sub(s.Start) != MockDuration {
			t.Errorf("slot %v lasts %v", s.Start, s.End.Sub(s.Start))
		}
		days = append(days, local.Format(time.DateOnly))
	}
	want := []string{"2026-10-20", "2026-10-20", "2026-10-27", "2026-10-27", "2026-11-03", "2026-11-03"}
	if !slices.Equal(days, want) {
		t.Errorf("slot days = %v, want %v", days, want)
	}
}

func TestMockSlotsOfferNothingOnAClosedDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	day, err := slots.NewDate(2026, time.October, 27)
	if err != nil {
		t.Fatal(err)
	}
	midnight, err := slots.NewTimeOfDay(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := slots.NewDatedRule(day, midnight, midnight)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)

	res, err := mockSlots([]slots.Rule{weeklyRule(t, time.Tuesday, 19, 21), closure}, berlin, nil, now, to, now, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.In(berlin).Format(time.DateOnly) == "2026-10-27" {
			t.Errorf("slot %v offered on the closed date", s.Start)
		}
	}
	if len(res.Slots) != 4 {
		t.Errorf("slots = %d, want 4 (two Tuesdays, two hours each)", len(res.Slots))
	}
}

func TestMockEntrySettle(t *testing.T) {
	start := time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC)
	base := MockEntry{StartsAt: start, EndsAt: start.Add(MockDuration), Status: MockConfirmed, MeetingURL: "https://meet.example.test/room"}

	before := base
	before.settle(start.Add(-time.Minute))
	if !before.CanCancel || before.CanReview || before.MeetingURL == "" {
		t.Errorf("before the start: %+v", before)
	}
	after := base
	after.settle(start.Add(2 * time.Hour))
	if after.CanCancel || !after.CanReview {
		t.Errorf("after the end: %+v", after)
	}
	reviewed := base
	reviewed.MyReview = &MockReview{Rating: 4}
	reviewed.settle(start.Add(2 * time.Hour))
	if reviewed.CanReview {
		t.Error("a reviewed mock still offers a review")
	}
	closed := base
	closed.settle(start.Add(MockDuration + ReviewWindow))
	if closed.CanReview {
		t.Error("a mock past its review window still offers a review")
	}
	cancelled := base
	cancelled.Status = MockCancelled
	cancelled.settle(start.Add(-time.Hour))
	if cancelled.CanCancel || cancelled.MeetingURL != "" {
		t.Errorf("a cancelled mock: %+v", cancelled)
	}
}
