package main

import (
	"slices"
	"strings"
	"testing"
)

// The fold moved out of the SQL and into Go for the planner's sake, so what it
// produces must still be byte-identical to what `replace(company_slug, '-', ”)`
// produced — the query compares the array against that same expression over the
// column, and any divergence silently stops matching rows rather than erroring.
func TestFoldCompanySlugs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"strips every hyphen, not just the first", []string{"cfo-insights-gmbh"}, []string{"cfoinsightsgmbh"}},
		{"leaves an unhyphenated slug alone", []string{"cfoinsights"}, []string{"cfoinsights"}},
		// The collision is the whole point of folding: one source spells an employer
		// "CFO Insights", another "Cfoinsights", and the two slugs must agree.
		{"two spellings fold together", []string{"cfo-insights", "cfoinsights"}, []string{"cfoinsights", "cfoinsights"}},
		{"keeps duplicates rather than collapsing them", []string{"a-b", "ab"}, []string{"ab", "ab"}},
		{"empty input yields empty output", []string{}, []string{}},
		{"a slug that is only hyphens folds to empty", []string{"---"}, []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foldCompanySlugs(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("foldCompanySlugs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Order and length must survive the fold: the array is compared positionally by
// nothing, but a caller batching by companyBatchSize relies on one folded entry per
// input company — a shorter result would silently drop companies from the batch.
func TestFoldCompanySlugsPreservesLength(t *testing.T) {
	in := make([]string, 500)
	for i := range in {
		in[i] = "company-" + strings.Repeat("x", i%7) + "-slug"
	}
	if got := foldCompanySlugs(in); len(got) != len(in) {
		t.Fatalf("folded %d companies into %d entries", len(in), len(got))
	}
}

// A nil batch must not panic: forCompanyBatches never emits one today, but the
// helper is a plain function and the query would simply match nothing.
func TestFoldCompanySlugsNil(t *testing.T) {
	if got := foldCompanySlugs(nil); len(got) != 0 {
		t.Fatalf("foldCompanySlugs(nil) = %q, want empty", got)
	}
}
