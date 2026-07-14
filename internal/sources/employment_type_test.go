package sources

import "testing"

// The per-adapter employment-type mappings translate each platform's own vocabulary
// onto freehire's controlled values, mapping any unknown/ambiguous value to "" so the
// pipeline's description parser decides — structured signal only, never a guess.

func TestUKGEmploymentType(t *testing.T) {
	tru, fls := true, false
	cases := []struct {
		in   *bool
		want string
	}{
		{&tru, "full_time"},
		{&fls, "part_time"},
		{nil, ""}, // absent flag → let the description parser decide
	}
	for _, c := range cases {
		if got := ukgEmploymentType(c.in); got != c.want {
			t.Errorf("ukgEmploymentType(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOracleEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full time": "full_time", "full time": "full_time",
		"Part time": "part_time",
		"":          "", "On Call": "",
	}
	for in, want := range cases {
		if got := oracleEmploymentType(in); got != want {
			t.Errorf("oracleEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

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

func TestWorkableEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full-time": "full_time", "Part-time": "part_time",
		"Contract": "contract", "Temporary": "contract",
		"Internship": "internship",
		"Other":      "", "": "",
	}
	for in, want := range cases {
		if got := workableEmploymentType(in); got != want {
			t.Errorf("workableEmploymentType(%q) = %q, want %q", in, got, want)
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
