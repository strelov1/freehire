package ats

import "testing"

func TestAshbyEmploymentType(t *testing.T) {
	cases := map[string]string{
		"FullTime": "full_time", "PartTime": "part_time",
		"Contract": "contract", "Temporary": "contract",
		"Intern": "internship", "Internship": "internship",
		"": "", "Volunteer": "",
	}
	for in, want := range cases {
		if got := ashbyEmploymentType(in); got != want {
			t.Errorf("ashbyEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLeverEmploymentType(t *testing.T) {
	// Lever's commitment is per-company free text, so the mapping matches keywords.
	cases := map[string]string{
		"Regular Full-Time":     "full_time",
		"Full-Time Maintenance": "full_time",
		"Full Time":             "full_time",
		"Part-Time":             "part_time",
		"Intern":                "internship",
		"Temporary":             "contract",
		"Contractor":            "contract",
		"Variable Hour":         "", // ambiguous → abstain
		"":                      "",
	}
	for in, want := range cases {
		if got := leverEmploymentType(in); got != want {
			t.Errorf("leverEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
