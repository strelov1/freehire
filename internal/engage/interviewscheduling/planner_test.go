package interviewscheduling

import (
	"testing"
	"time"
)

func TestSelectedTimeMustBeSupportedByRecruiterEvidence(t *testing.T) {
	first := time.Date(2026, 12, 4, 11, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	offers := []time.Time{first, second}
	cases := []struct {
		body     string
		decision Decision
		want     time.Time
	}{
		{"The second option works for me.", Decision{Action: Accept, Choice: 2, Evidence: "second option"}, second},
		{"The second option does not work for me. The first option works.", Decision{Action: Accept, Choice: 2, Evidence: "second option"}, time.Time{}},
		{"The first option works for me.", Decision{Action: Accept, Choice: 2, Evidence: "first option"}, time.Time{}},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: first, Evidence: "Dec 4 at 11:00 UTC",
		}, first},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: second, Evidence: "Dec 4 at 11:00 UTC",
		}, time.Time{}},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: first, Evidence: "Dec 11 at 11:00 UTC",
		}, time.Time{}},
	}
	for _, tc := range cases {
		if got := selectedTime(tc.body, tc.decision, offers, time.UTC); !got.Equal(tc.want) {
			t.Errorf("%q: selected %v, want %v", tc.body, got, tc.want)
		}
	}
}
