package telegram

import (
	"strings"
	"testing"
)

func validJob() ExtractedJob {
	return ExtractedJob{
		Title:       "Senior Go Engineer",
		Company:     "Acme",
		Location:    "London",
		Remote:      true,
		Description: "Building the thing. $150k. Apply via @acme_hr.",
	}
}

func TestExtractionValidate(t *testing.T) {
	t.Run("zero jobs is valid (post was not a vacancy)", func(t *testing.T) {
		e := Extraction{}
		if err := e.Validate(); err != nil {
			t.Errorf("empty extraction: %v, want nil", err)
		}
	})

	t.Run("a well-formed job passes", func(t *testing.T) {
		e := Extraction{Jobs: []ExtractedJob{validJob()}}
		if err := e.Validate(); err != nil {
			t.Errorf("valid: %v, want nil", err)
		}
		if len(e.Jobs) != 1 {
			t.Errorf("Jobs = %v, want the one well-formed job kept", e.Jobs)
		}
	})

	t.Run("company and location may be empty", func(t *testing.T) {
		j := validJob()
		j.Company, j.Location = "", ""
		e := Extraction{Jobs: []ExtractedJob{j}}
		if err := e.Validate(); err != nil {
			t.Errorf("optional fields empty: %v, want nil", err)
		}
	})

	t.Run("empty title is rejected", func(t *testing.T) {
		j := validJob()
		j.Title = "  "
		e := Extraction{Jobs: []ExtractedJob{j}}
		err := e.Validate()
		if err == nil || !strings.Contains(err.Error(), "title") {
			t.Errorf("err = %v, want a title error", err)
		}
	})

	t.Run("empty description is rejected", func(t *testing.T) {
		j := validJob()
		j.Description = ""
		e := Extraction{Jobs: []ExtractedJob{j}}
		err := e.Validate()
		if err == nil || !strings.Contains(err.Error(), "description") {
			t.Errorf("err = %v, want a description error", err)
		}
	})

	t.Run("a malformed job is dropped, not the whole extraction", func(t *testing.T) {
		bad := validJob()
		bad.Title = ""
		good1, good2 := validJob(), validJob()
		good2.Title = "Backend Engineer"
		e := Extraction{Jobs: []ExtractedJob{good1, bad, good2}}
		if err := e.Validate(); err != nil {
			t.Fatalf("err = %v, want nil (some jobs remain valid)", err)
		}
		if len(e.Jobs) != 2 {
			t.Fatalf("Jobs = %v, want the 2 well-formed jobs kept", e.Jobs)
		}
		if e.Jobs[0].Title != good1.Title || e.Jobs[1].Title != good2.Title {
			t.Errorf("Jobs = %v, want [%q, %q]", e.Jobs, good1.Title, good2.Title)
		}
	})

	t.Run("all jobs malformed is rejected", func(t *testing.T) {
		bad1, bad2 := validJob(), validJob()
		bad1.Title, bad2.Description = "", ""
		e := Extraction{Jobs: []ExtractedJob{bad1, bad2}}
		if err := e.Validate(); err == nil {
			t.Error("err = nil, want an error when nothing survives validation")
		}
		if len(e.Jobs) != 0 {
			t.Errorf("Jobs = %v, want none kept", e.Jobs)
		}
	})
}
