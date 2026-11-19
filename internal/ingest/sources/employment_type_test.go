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
