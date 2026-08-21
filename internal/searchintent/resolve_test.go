package searchintent

import (
	"slices"
	"testing"
)

// The grounding rule under test: nothing the model writes becomes a filter until a
// dictionary has confirmed it, and everything a dictionary refuses is reported. A
// dropped value that is not reported is, to the caller, indistinguishable from one
// that was applied — which is the failure this package exists to prevent.

func TestResolveRefusesUnknownFacetName(t *testing.T) {
	_, err := intent{Facets: map[string][]string{"vibe": {"chill"}}}.resolve()
	if err == nil {
		t.Fatal("resolve accepted the facet \"vibe\", which the search filter does not read")
	}
}

func TestResolveKeepsClosedVocabularyValue(t *testing.T) {
	got, err := intent{Facets: map[string][]string{"seniority": {"senior"}}}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Facets["seniority"], []string{"senior"}) {
		t.Fatalf("seniority = %v, want [senior]", got.Facets["seniority"])
	}
}

func TestResolveDropsValueOutsideClosedVocabulary(t *testing.T) {
	got, err := intent{Facets: map[string][]string{"seniority": {"rockstar"}}}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got.Facets["seniority"]) != 0 {
		t.Fatalf("seniority = %v, want none — \"rockstar\" is not a seniority", got.Facets["seniority"])
	}
	if !slices.Contains(got.Unresolved, "rockstar") {
		t.Fatalf("unresolved = %v, want it to name \"rockstar\"", got.Unresolved)
	}
}

func TestResolveNormalizesClosedVocabularyCasing(t *testing.T) {
	got, err := intent{Facets: map[string][]string{"work_mode": {"Remote"}}}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Facets["work_mode"], []string{"remote"}) {
		t.Fatalf("work_mode = %v, want [remote]", got.Facets["work_mode"])
	}
}
