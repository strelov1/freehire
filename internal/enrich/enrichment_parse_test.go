package enrich

import (
	"encoding/json"
	"testing"
)

// The model occasionally emits a semantically correct value in the wrong JSON
// shape: a scalar where the contract wants an array, an array where it wants a
// scalar, or a float where it wants an int. The data is good; only the wrapper
// is wrong. Enrichment.UnmarshalJSON coerces these to the canonical shape so a
// stray wrapper no longer fails the whole payload.

func TestUnmarshalCoercesScalarToArray(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"regions":"eu"}`), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Regions) != 1 || e.Regions[0] != "eu" {
		t.Errorf("regions = %v, want [eu]", e.Regions)
	}
}

func TestUnmarshalCoercesArrayToScalar(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"seniority":["senior"]}`), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Seniority != "senior" {
		t.Errorf("seniority = %q, want senior", e.Seniority)
	}
}

func TestUnmarshalEmptyArrayToScalarStaysEmpty(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"category":[]}`), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Category != "" {
		t.Errorf("category = %q, want empty", e.Category)
	}
}

func TestUnmarshalRoundsFloatToInt(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"salary_min":17.5,"salary_max":209587.5,"experience_years_min":0.5}`), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.SalaryMin == nil || *e.SalaryMin != 18 {
		t.Errorf("salary_min = %v, want 18", e.SalaryMin)
	}
	if e.SalaryMax == nil || *e.SalaryMax != 209588 {
		t.Errorf("salary_max = %v, want 209588", e.SalaryMax)
	}
	if e.ExperienceYearsMin == nil || *e.ExperienceYearsMin != 1 {
		t.Errorf("experience_years_min = %v, want 1", e.ExperienceYearsMin)
	}
}

func TestUnmarshalCanonicalShapesUnchanged(t *testing.T) {
	raw := `{"regions":["eu","us"],"seniority":"senior","salary_min":100000,"skills":["go","postgresql"]}`
	var e Enrichment
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Regions) != 2 || e.Regions[0] != "eu" || e.Regions[1] != "us" {
		t.Errorf("regions = %v, want [eu us]", e.Regions)
	}
	if e.Seniority != "senior" {
		t.Errorf("seniority = %q, want senior", e.Seniority)
	}
	if e.SalaryMin == nil || *e.SalaryMin != 100000 {
		t.Errorf("salary_min = %v, want 100000", e.SalaryMin)
	}
	if len(e.Skills) != 2 {
		t.Errorf("skills = %v, want 2 entries", e.Skills)
	}
}

func TestUnmarshalStillRejectsBrokenJSON(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"regions":`), &e); err == nil {
		t.Error("expected error for truncated JSON, got nil")
	}
}
