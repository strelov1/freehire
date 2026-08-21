package searchintent

import (
	"encoding/json"
	"slices"
	"testing"
)

// Observed against the live gateway: asked for "seniority": ["senior"] under a strict
// schema declaring an array, the model answered "seniority": "senior". encoding/json
// aborts the WHOLE unmarshal on the first type mismatch, so one scalar in one field
// discarded the entire interpretation and the caller saw a 500 — for an answer that was
// perfectly usable.

func TestProposalAcceptsABareStringWhereAListWasAsked(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"seniority":"senior","skills":["go"]}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal([]string(p.Seniority), []string{"senior"}) {
		t.Fatalf("seniority = %v, want [senior]", p.Seniority)
	}
	if !slices.Equal([]string(p.Skills), []string{"go"}) {
		t.Fatalf("skills = %v, want [go]", p.Skills)
	}
}

// One malformed field must not cost the rest of the answer.
func TestProposalSurvivesAScalarInsideAnExclusion(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"exclude":{"countries":"United Kingdom"},"work_mode":["remote"]}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal([]string(p.Exclude.Countries), []string{"United Kingdom"}) {
		t.Fatalf("excluded countries = %v", p.Exclude.Countries)
	}
	if !slices.Equal([]string(p.WorkMode), []string{"remote"}) {
		t.Fatalf("work_mode = %v", p.WorkMode)
	}
}

// A list with one number in it is the same miss as a bare scalar, one level down — and
// it cost more, because encoding/json abandoned the whole field rather than the one
// element. The point of this type is that a single odd value never costs the rest.
func TestProposalReadsAMixedList(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"skills":["go",5,null,"react"]}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal([]string(p.Skills), []string{"go", "5", "react"}) {
		t.Fatalf("skills = %v, want [go 5 react] — one odd element must not cost the list", p.Skills)
	}
}

func TestProposalReadsNullAndEmptyAsNoValues(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"seniority":null,"skills":"","cities":[]}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for name, got := range map[string]flexStrings{"seniority": p.Seniority, "skills": p.Skills, "cities": p.Cities} {
		if len(got) != 0 {
			t.Errorf("%s = %v, want none", name, got)
		}
	}
}

// A number where a list of strings was asked for is the same class of miss, and the
// values that matter here (a year count, a salary) are exactly what a model writes bare.
func TestProposalAcceptsABareNumberWhereAListWasAsked(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"company_size":11}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal([]string(p.CompanySize), []string{"11"}) {
		t.Fatalf("company_size = %v, want [11]", p.CompanySize)
	}
}

// The bounds are ints, and a model that quotes one must not cost the interpretation
// either — same failure, same reason to guard against it.
func TestProposalAcceptsAQuotedNumberForABound(t *testing.T) {
	var p proposal
	if err := json.Unmarshal([]byte(`{"salary_min":"100000","posted_within_days":7}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := p.SalaryMin.plain(); got == nil || *got != 100000 {
		t.Fatalf("salary_min = %v, want 100000", got)
	}
	if got := p.PostedWithinDays.plain(); got == nil || *got != 7 {
		t.Fatalf("posted_within_days = %v, want 7", got)
	}
}
