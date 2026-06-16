package search

import (
	"net/url"
	"reflect"
	"sort"
	"testing"
)

// normalizeGroups makes a Filter result order-insensitive for comparison: the
// fragments within a group and the groups themselves are sorted. FilterFromValues
// iterates a map, so group order is not deterministic, but the AND/OR semantics
// do not depend on order.
func normalizeGroups(t *testing.T, got any) [][]string {
	t.Helper()
	if got == nil {
		return nil
	}
	groups, ok := got.([][]string)
	if !ok {
		t.Fatalf("filter type = %T, want [][]string", got)
	}
	out := make([][]string, len(groups))
	for i, g := range groups {
		cp := append([]string(nil), g...)
		sort.Strings(cp)
		out[i] = cp
	}
	sort.Slice(out, func(i, j int) bool {
		return joinKey(out[i]) < joinKey(out[j])
	})
	return out
}

func joinKey(s []string) string {
	b := ""
	for _, x := range s {
		b += x + "|"
	}
	return b
}

func vals(q string) url.Values {
	v, _ := url.ParseQuery(q)
	return v
}

func TestFilterFromValues_Empty(t *testing.T) {
	if got := FilterFromValues(url.Values{}); got != nil {
		t.Errorf("FilterFromValues(empty) = %v, want nil", got)
	}
	// Params we do not filter on (free-text query, sort) produce no filter.
	if got := FilterFromValues(vals("q=go&sort=posted_at")); got != nil {
		t.Errorf("FilterFromValues(non-facet) = %v, want nil", got)
	}
}

func TestFilterFromValues_SingleFacet(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("seniority=senior")))
	want := [][]string{{`enrichment.seniority = "senior"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_RepeatedFacetIsORed(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("skills=go&skills=rust")))
	want := [][]string{{`skills = "go"`, `skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_AndMode(t *testing.T) {
	// skills_mode=and → each value its own AND group (a job must have both).
	got := normalizeGroups(t, FilterFromValues(vals("skills=go&skills=rust&skills_mode=and")))
	want := [][]string{{`skills = "go"`}, {`skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_Exclude(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("regions_exclude=cis")))
	want := [][]string{{`regions != "cis"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_VisaBoolAndNumeric(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("visa_sponsorship=true&salary_min=100000&salary_max=200000&experience_years_min=3")))
	want := [][]string{
		{`enrichment.experience_years_min >= 3`},
		{`enrichment.salary_max <= 200000`},
		{`enrichment.salary_min >= 100000`},
		{`enrichment.visa_sponsorship = true`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_NonNumericSalaryIgnored(t *testing.T) {
	// A non-numeric value must not emit a bogus `>= 0` fragment.
	if got := FilterFromValues(vals("salary_min=abc")); got != nil {
		t.Errorf("FilterFromValues(bad salary) = %v, want nil", got)
	}
}

func TestFilterFromValues_EmptyValueIgnored(t *testing.T) {
	if got := FilterFromValues(vals("seniority=")); got != nil {
		t.Errorf("FilterFromValues(empty facet value) = %v, want nil", got)
	}
}
