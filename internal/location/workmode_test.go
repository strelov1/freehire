package location

import "testing"

func TestWorkModeFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		// Positives — clear, anchored phrases.
		{"fully remote", "This is a fully remote position open to anyone.", "remote"},
		{"remote-first", "We are a remote-first company.", "remote"},
		{"100 percent remote", "The role is 100% remote.", "remote"},
		{"work from anywhere", "You can work from anywhere in the EU.", "remote"},
		{"remote position", "Remote position with occasional travel.", "remote"},
		{"hybrid role", "This is a hybrid role based in Berlin.", "hybrid"},
		{"hybrid working", "We offer hybrid working arrangements.", "hybrid"},
		{"days in the office", "You will spend 3 days in the office each week.", "hybrid"},
		{"on-site only", "This job is on-site only.", "onsite"},
		{"must be on-site", "Candidates must be on-site in our HQ.", "onsite"},
		{"in-office position", "An in-office position in Munich.", "onsite"},

		// Priority — hybrid beats remote when both appear.
		{"hybrid beats remote", "A hybrid role with some remote days.", "hybrid"},

		// Trap negatives — incidental tokens that must NOT trigger a match.
		{"distributed systems", "Experience building distributed systems at scale.", ""},
		{"hybrid cloud", "You will manage our hybrid cloud infrastructure.", ""},
		{"remote server", "Debug issues on a remote server over SSH.", ""},
		{"remote team", "Collaborate with a remote team across time zones.", ""},
		{"bare in office", "Free snacks in office and a great culture.", ""},
		{"incidental from our office", "Enjoy free lunch from our office cafeteria.", ""},
		{"no arrangement phrase", "We build payments infrastructure in Go.", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkModeFromDescription(tt.desc); got != tt.want {
				t.Errorf("WorkModeFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}
