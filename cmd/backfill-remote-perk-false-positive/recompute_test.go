package main

import "testing"

func TestRecomputeWorkMode(t *testing.T) {
	tests := []struct {
		name        string
		location    string
		description string
		want        string
	}{
		{
			name:     "the reported false positive: qualified perk phrase, no other signal",
			location: "San Francisco, CA & New York, NY",
			description: `Flexible PTO and the freedom to work from anywhere in the world for up to a month ` +
				`— because life doesn't pause, and neither should you. We run a hybrid culture that brings ` +
				`teams together in person 3 to 4 days a week.`,
			want: "",
		},
		{
			name:        "location marker independently says remote: untouched",
			location:    "Remote - US",
			description: "Flexible PTO and the freedom to work from anywhere in the world for up to a month.",
			want:        "remote",
		},
		{
			name:        "an unqualified remote phrase elsewhere in the description: still remote",
			location:    "San Francisco, CA",
			description: "This is a fully remote position. Also, work from anywhere for up to a month during your PTO.",
			want:        "remote",
		},
		{
			name:        "a genuine denial after the location-silent description phrase: onsite",
			location:    "Austin, TX",
			description: "This position is remote. Correction: this role is not a remote position, it is based in our office.",
			want:        "onsite",
		},
		{
			name:        "no signal at all",
			location:    "Austin, TX",
			description: "We build great software.",
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recomputeWorkMode(tt.location, tt.description)
			if got != tt.want {
				t.Errorf("recomputeWorkMode(%q, ...) = %q, want %q", tt.location, got, tt.want)
			}
		})
	}
}
