package search

import (
	"reflect"
	"testing"
)

func TestFilterFromValues_ExperienceYearsMax(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_max=3")))
	want := [][]string{{`enrichment.experience_years_min <= 3`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_ExperienceYearsClosedRange(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_min=2&experience_years_max=5")))
	want := [][]string{
		{`enrichment.experience_years_min <= 5`},
		{`enrichment.experience_years_min >= 2`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// experience_include_unspecified widens each experience bound to also admit the
// postings that state no requirement at all — most of the catalogue, which a
// bound otherwise drops. It is an OR inside the bound's own group, so it never
// widens anything else in the query.
func TestFilterFromValues_ExperienceIncludeUnspecified(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_min=2&experience_years_max=5&experience_include_unspecified=true")))
	want := [][]string{
		{`enrichment.experience_years_min <= 5`, `enrichment.experience_years_min NOT EXISTS`},
		{`enrichment.experience_years_min >= 2`, `enrichment.experience_years_min NOT EXISTS`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// On its own the modifier bounds nothing, so it must not invent a filter: a query
// carrying only it is the whole catalogue, and Narrows has to say so.
func TestFilterFromValues_ExperienceIncludeUnspecifiedAloneIsNoFilter(t *testing.T) {
	if got := FilterFromValues(vals("experience_include_unspecified=true")); got != nil {
		t.Errorf("got %v, want no filter", got)
	}
}

// experience_only_seniority_unspecified confines each experience bound to the postings
// that carry no level: a labelled one passes the bound whatever years it states, so it
// is answered by the seniority facet alone. Paired with seniority_include_unspecified it
// reads "one of these levels, or no level and within the bound" — the entry-level
// profile alert, which an independent AND of the two cannot express (freehire#3655).
func TestFilterFromValues_ExperienceOnlySeniorityUnspecified(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("seniority=intern&seniority_include_unspecified=true&experience_years_max=1&experience_only_seniority_unspecified=true")))
	want := [][]string{
		{`enrichment.experience_years_min <= 1`, `enrichment.seniority EXISTS`},
		{`enrichment.seniority = "intern"`, `enrichment.seniority NOT EXISTS`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Both experience modifiers OR into the same bound group, so together a bound admits
// a labelled posting, an unstated one, or one within the bound.
func TestFilterFromValues_ExperienceModifiersCombine(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_min=2&experience_include_unspecified=true&experience_only_seniority_unspecified=true")))
	want := [][]string{
		{`enrichment.experience_years_min >= 2`, `enrichment.experience_years_min NOT EXISTS`, `enrichment.seniority EXISTS`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Like the other modifier, alone it bounds nothing.
func TestFilterFromValues_ExperienceOnlySeniorityUnspecifiedAloneIsNoFilter(t *testing.T) {
	if got := FilterFromValues(vals("experience_only_seniority_unspecified=true")); got != nil {
		t.Errorf("got %v, want no filter", got)
	}
}

// Zero is the entry-level selector, not an absent value: `experience_years_max=0`
// asks for the postings that state no prior experience is required. A guard that
// treats the bound as falsy would silently drop the one filter juniors need.
func TestFilterFromValues_ExperienceYearsMaxZero(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_max=0")))
	want := [][]string{{`enrichment.experience_years_min <= 0`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_ExperienceYearsMaxNegative(t *testing.T) {
	for _, raw := range []string{"-1", "-10"} {
		got := normalizeGroups(t, FilterFromValues(vals("experience_years_max="+raw)))
		if len(got) != 0 {
			t.Errorf("experience_years_max=%q: got %v, want no filter group", raw, got)
		}
	}
}

func TestFilterFromValues_ExperienceYearsMaxUnparseable(t *testing.T) {
	for _, raw := range []string{"", "abc", "3.5"} {
		got := normalizeGroups(t, FilterFromValues(vals("experience_years_max="+raw)))
		if len(got) != 0 {
			t.Errorf("experience_years_max=%q: got %v, want no filter group", raw, got)
		}
	}
}
