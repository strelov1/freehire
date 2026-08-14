package search

import (
	"reflect"
	"testing"
)

func TestEq_QuotesAndEscapes(t *testing.T) {
	if got := Eq("seniority", "senior"); got != `seniority = "senior"` {
		t.Errorf("Eq = %q", got)
	}
	// A value carrying a quote must be escaped so it cannot break out of the
	// string literal and inject filter logic.
	if got := Eq("title", `a"b`); got != `title = "a\"b"` {
		t.Errorf("Eq escape = %q", got)
	}
}

func TestEqBool(t *testing.T) {
	if got := EqBool("remote", true); got != "remote = true" {
		t.Errorf("EqBool true = %q", got)
	}
	if got := EqBool("visa_sponsorship", false); got != "visa_sponsorship = false" {
		t.Errorf("EqBool false = %q", got)
	}
}

func TestGteLte(t *testing.T) {
	if got := Gte("salary_min", 100000); got != "salary_min >= 100000" {
		t.Errorf("Gte = %q", got)
	}
	if got := Lte("salary_max", 200000); got != "salary_max <= 200000" {
		t.Errorf("Lte = %q", got)
	}
}

func TestNotIn(t *testing.T) {
	if got := NotIn("id", []int64{3, 1, 2}); got != "id NOT IN [3, 1, 2]" {
		t.Errorf("NotIn = %q, want %q", got, "id NOT IN [3, 1, 2]")
	}
	if got := NotIn("id", []int64{7}); got != "id NOT IN [7]" {
		t.Errorf("NotIn single = %q", got)
	}
	// An empty exclusion set yields no fragment, so the caller adds no filter.
	if got := NotIn("id", nil); got != "" {
		t.Errorf("NotIn(nil) = %q, want empty", got)
	}
	if got := NotIn("id", []int64{}); got != "" {
		t.Errorf("NotIn(empty) = %q, want empty", got)
	}
}

func TestInStrings(t *testing.T) {
	if got := InStrings("company_slug", []string{"acme", "globex"}); got != `company_slug IN ["acme", "globex"]` {
		t.Errorf("InStrings = %q", got)
	}
	if got := InStrings("company_slug", []string{`a"b`}); got != `company_slug IN ["a\"b"]` {
		t.Errorf("InStrings escape = %q", got)
	}
	// An empty set yields no fragment, so the caller adds no filter — mirrors NotIn(nil/empty).
	if got := InStrings("company_slug", nil); got != "" {
		t.Errorf("InStrings(nil) = %q, want empty", got)
	}
	if got := InStrings("company_slug", []string{}); got != "" {
		t.Errorf("InStrings(empty) = %q, want empty", got)
	}
}

func TestNotInStrings(t *testing.T) {
	if got := NotInStrings("source", []string{"himalayas", "echojobs"}); got != `source NOT IN ["himalayas", "echojobs"]` {
		t.Errorf("NotInStrings = %q", got)
	}
	if got := NotInStrings("source", nil); got != "" {
		t.Errorf("NotInStrings(nil) = %q, want empty", got)
	}
}

func TestFilter_NilWhenEmpty(t *testing.T) {
	if got := Filter(); got != nil {
		t.Errorf("Filter() = %v, want nil", got)
	}
	if got := Filter([]string{}, nil); got != nil {
		t.Errorf("Filter(empty groups) = %v, want nil", got)
	}
}

func TestFilter_NestsAndOfOr(t *testing.T) {
	got := Filter(
		[]string{Eq("category", "backend"), Eq("category", "frontend")}, // OR within
		[]string{EqBool("remote", true)},
	)
	want := [][]string{
		{`category = "backend"`, `category = "frontend"`},
		{"remote = true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Filter = %#v, want %#v", got, want)
	}
}

func TestFilter_SkipsEmptyGroups(t *testing.T) {
	got := Filter(
		[]string{Eq("seniority", "senior")},
		[]string{}, // dropped
		nil,        // dropped
	)
	want := [][]string{{`seniority = "senior"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Filter = %#v, want %#v", got, want)
	}
}
