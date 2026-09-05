package resumeextract

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// The model is asked for structured {"year":N,"month":N} dates, but it frequently emits
// a bare number instead (prod: user 291 — resume_structured never persisted because
// encoding/json aborts the WHOLE unmarshal on the first type mismatch, and one numeric
// year silently killed the entire structured résumé). perioddate.PeriodDate.UnmarshalJSON's
// defensive bare-number tolerance must still catch this with the new schema shape.
func TestUnmarshal_NumericDateFieldsDecodeAsYears(t *testing.T) {
	raw := `{
		"full_name": "Ada Lovelace",
		"experience": [{"title": "Engineer", "company": "Acme", "start": 2019, "end": 2021}],
		"education": [{"degree": "BSc", "institution": "MIT", "year": 2015}]
	}`

	var s Structured
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal with numeric year/dates failed: %v", err)
	}

	if len(s.Education) != 1 || *s.Education[0].Year != (perioddate.PeriodDate{Year: 2015}) {
		t.Errorf("Education[0].Year = %+v, want {2015 0}", educationYear(s))
	}
	if len(s.Experience) != 1 ||
		*s.Experience[0].Start != (perioddate.PeriodDate{Year: 2019}) ||
		*s.Experience[0].End != (perioddate.PeriodDate{Year: 2021}) {
		t.Errorf("Experience start/end = %+v/%+v, want {2019 0}/{2021 0}",
			experienceStart(s), experienceEnd(s))
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

func educationYear(s Structured) *perioddate.PeriodDate {
	if len(s.Education) == 0 {
		return nil
	}
	return s.Education[0].Year
}

func experienceStart(s Structured) *perioddate.PeriodDate {
	if len(s.Experience) == 0 {
		return nil
	}
	return s.Experience[0].Start
}

func experienceEnd(s Structured) *perioddate.PeriodDate {
	if len(s.Experience) == 0 {
		return nil
	}
	return s.Experience[0].End
}
