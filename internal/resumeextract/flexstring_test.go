package resumeextract

import (
	"encoding/json"
	"testing"
)

// The model is asked to keep years/dates as written, but it frequently emits a bare
// number (e.g. "year": 2019) instead of a string. encoding/json aborts the WHOLE
// unmarshal on the first type mismatch, so one numeric year silently kills the entire
// structured résumé (prod: user 291 — resume_structured never persisted). Number-or-string
// free-form fields must decode either way.
func TestUnmarshal_NumericDateFieldsDecodeAsStrings(t *testing.T) {
	raw := `{
		"full_name": "Ada Lovelace",
		"experience": [{"title": "Engineer", "company": "Acme", "start": 2019, "end": 2021}],
		"education": [{"degree": "BSc", "institution": "MIT", "year": 2015}]
	}`

	var s Structured
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal with numeric year/dates failed: %v", err)
	}

	if len(s.Education) != 1 || s.Education[0].Year != "2015" {
		t.Errorf("Education[0].Year = %q, want %q", educationYear(s), "2015")
	}
	if len(s.Experience) != 1 || s.Experience[0].Start != "2019" || s.Experience[0].End != "2021" {
		t.Errorf("Experience start/end = %q/%q, want %q/%q",
			experienceStart(s), experienceEnd(s), "2019", "2021")
	}
}

// total_years is prompted as an integer, but the model can return it as a string
// ("5") or a phrase ("5+ years"). A string there aborts the whole decode just like a
// numeric year does, so it must coerce to the leading integer.
func TestUnmarshal_TotalYearsFromString(t *testing.T) {
	cases := map[string]int{
		`{"total_years": 5}`:          5,
		`{"total_years": "5"}`:        5,
		`{"total_years": "5+ years"}`: 5,
		`{"total_years": ""}`:         0,
	}
	for raw, want := range cases {
		var s Structured
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Fatalf("unmarshal %s failed: %v", raw, err)
		}
		if s.TotalYears != want {
			t.Errorf("%s: TotalYears = %d, want %d", raw, s.TotalYears, want)
		}
	}
}

func educationYear(s Structured) string {
	if len(s.Education) == 0 {
		return ""
	}
	return s.Education[0].Year
}

func experienceStart(s Structured) string {
	if len(s.Experience) == 0 {
		return ""
	}
	return s.Experience[0].Start
}

func experienceEnd(s Structured) string {
	if len(s.Experience) == 0 {
		return ""
	}
	return s.Experience[0].End
}
