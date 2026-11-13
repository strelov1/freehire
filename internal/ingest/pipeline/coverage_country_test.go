package pipeline

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// TestRunScopesCoverageToTheBoardsMarket proves the gate asks CoverageLookup about a national
// aggregator's own countries, and about none for a global one. Asked country-blind, a
// Slovenian employer on mojedelo was discarded because a foreign employer of the same name has
// a crawled board (issue #4412).
func TestRunScopesCoverageToTheBoardsMarket(t *testing.T) {
	for _, tc := range []struct {
		provider, board string
		want            []string
	}{
		{"mojedelo", "", []string{"si"}},
		{"mojposao", "mojposao.ba", []string{"ba"}},
		{"himalayas", "", nil},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			src := fakeSource{provider: tc.provider, jobs: []sources.Job{
				{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
			}}
			coverage := &fakeCoverage{}
			r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage}
			if _, err := r.Run(context.Background(), []sources.CompanyEntry{
				{Company: "Acme", Provider: tc.provider, Board: tc.board},
			}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(coverage.countryArgs) != 1 {
				t.Fatalf("coverage asked %d times, want 1", len(coverage.countryArgs))
			}
			if got := coverage.countryArgs[0]; !slices.Equal(got, tc.want) {
				t.Errorf("countries = %v, want %v", got, tc.want)
			}
		})
	}
}
