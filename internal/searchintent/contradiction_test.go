package searchintent

import (
	"encoding/json"
	"slices"
	"testing"
)

// All three of these were found by running the feature against the live gateway. None
// could have been found from the contract alone: they are things a real model does that
// the schema permits.

func decode(t *testing.T, body string, into *proposal) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), into); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
}

// Observed on a refinement: asked to change "onsite in Berlin" to "remote in Europe",
// the model returned regions=[eu] AND exclude.regions=[eu]. A filter that excludes what
// it includes matches nothing, and reads to the user as "we carry no such jobs".
func TestResolveDropsAnExclusionThatContradictsAnInclusion(t *testing.T) {
	got, err := intent{
		Facets:  map[string][]string{"regions": {"eu"}, "skills": {"go"}},
		Exclude: map[string][]string{"regions": {"eu"}, "skills": {"php"}},
	}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Facets["regions"], []string{"eu"}) {
		t.Fatalf("regions = %v, want [eu] — the inclusion is what they asked for", got.Facets["regions"])
	}
	if len(got.Exclude["regions"]) != 0 {
		t.Fatalf("excluded regions = %v, want none — it cancels the inclusion", got.Exclude["regions"])
	}
	// An exclusion that contradicts nothing is untouched.
	if !slices.Equal(got.Exclude["skills"], []string{"php"}) {
		t.Fatalf("excluded skills = %v, want [php]", got.Exclude["skills"])
	}
}

// Observed everywhere: the model writes 0 for a bound it does not mean to set, rather
// than null. For a floor or a freshness window that is harmless — both are already out
// of range — but reporting it as something we "did not recognise" puts a line in front
// of the user about a value the model never meant to send.
func TestResolveTreatsAZeroFloorAsUnsetRatherThanUnrecognised(t *testing.T) {
	got, err := intent{SalaryMin: ptr(0), PostedWithinDays: ptr(0)}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Scalars.SalaryMin != nil || got.Scalars.PostedWithinDays != nil {
		t.Fatal("a zero bound was applied")
	}
	if len(got.Unresolved) != 0 {
		t.Fatalf("unresolved = %v, want none — a zero is the model saying nothing", got.Unresolved)
	}
}

// A real out-of-range value is still reported: that IS something it tried to say.
func TestResolveStillReportsAnAbsurdBound(t *testing.T) {
	got, err := intent{PostedWithinDays: ptr(100000)}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got.Unresolved) == 0 {
		t.Fatal("unresolved is empty — an out-of-range bound must still be reported")
	}
}

// The experience ceiling is the dangerous one. Zero is a REAL filter there — it selects
// postings that ask for no prior experience — so the model's habit of writing 0 for
// "unset" inverts the search: asked for senior roles, it returned the entry-level
// filter. The field is therefore asked for as TEXT, where "" is unset and "0" is the
// entry-level filter, and the two can no longer be confused.
func TestExperienceCeilingDistinguishesUnsetFromEntryLevel(t *testing.T) {
	var unset, entry proposal
	decode(t, `{"experience_years_max":""}`, &unset)
	decode(t, `{"experience_years_max":"0"}`, &entry)

	if got := unset.intent().ExperienceYearsMax; got != nil {
		t.Fatalf("empty text = %v, want unset", *got)
	}
	if got := entry.intent().ExperienceYearsMax; got == nil || *got != 0 {
		t.Fatalf(`"0" = %v, want the entry-level bound`, got)
	}
}

func TestExperienceCeilingReadsAWrittenNumber(t *testing.T) {
	var p proposal
	decode(t, `{"experience_years_max":"3"}`, &p)
	if got := p.intent().ExperienceYearsMax; got == nil || *got != 3 {
		t.Fatalf("experience ceiling = %v, want 3", got)
	}
}
