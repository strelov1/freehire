package searchintent

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/search"
)

// Two different mistakes wear the same shape, and treating them alike costs the caller
// their whole search. A name no filter has ever had means the model is confused about
// what this product can do, and returning "the rest" would answer a question nobody
// asked. A real filter this surface does not offer is an ordinary miss: drop it, say
// so, and give the caller everything else they asked for.

func TestResolveRefusesNameNoFilterHas(t *testing.T) {
	_, err := intent{Facets: map[string][]string{"vibe": {"chill"}}}.resolve()
	if err == nil {
		t.Fatal("resolve accepted \"vibe\", which is not a filter at all")
	}
}

func TestResolveDropsRealFilterThisSurfaceDoesNotOffer(t *testing.T) {
	// company_slug is a real filter, but naming a company is not something this
	// surface can ground: no dictionary here can tell "Stripe" from "Stripee", and an
	// ungrounded slug is the silent-empty-result failure the package exists to prevent.
	got, err := intent{Facets: map[string][]string{
		"company_slug": {"Stripe"},
		"seniority":    {"senior"},
	}}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got.Facets["company_slug"]) != 0 {
		t.Fatalf("company_slug = %v, want none", got.Facets["company_slug"])
	}
	if !slices.Contains(got.Unresolved, "Stripe") {
		t.Fatalf("unresolved = %v, want it to name the dropped company", got.Unresolved)
	}
	if !slices.Equal(got.Facets["seniority"], []string{"senior"}) {
		t.Fatalf("seniority = %v, want [senior] — one unofferable filter must not cost the rest", got.Facets["seniority"])
	}
}

// The resolver table is this surface's vocabulary, and every name in it must be a
// filter the search actually reads. A typo here would be invisible: the value would
// resolve, reach the filter, and be ignored — an unfiltered result set that looks like
// an answer, which is the exact failure this package exists to prevent.
func TestEveryOfferedFacetIsARealFilter(t *testing.T) {
	for name := range facetResolvers {
		if _, ok := search.StringFacets[name]; !ok {
			t.Errorf("facetResolvers offers %q, which the search filter does not read", name)
		}
	}
}
