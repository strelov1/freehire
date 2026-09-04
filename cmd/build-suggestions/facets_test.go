package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/search/search"
)

// The first production build failed here, and nothing in the unit suite could have
// caught it: the builder asks a live Meilisearch for a facet distribution, and the
// engine refused with
//
//	Attribute `role` is not filterable
//
// because the filter PARAMETER was `role` while the index ATTRIBUTE was `roles`. Two
// names for one concept, and the builder had the wrong one written out as a literal.
//
// That facet is retired and both names the builder now asks for happen to match their
// attribute, so the example is gone but the hazard is not — most of this table maps a
// bare param onto an `enrichment.` path. This does not need an engine. It only has to
// assert that the names the builder asks for come from the table the filter builder
// already uses, so a renamed attribute breaks the build rather than the 06:45 timer.
func TestFacetNamesComeFromTheFilterTable(t *testing.T) {
	for _, param := range []string{"skills", "category"} {
		attr, ok := search.StringFacets[param]
		if !ok {
			t.Fatalf("%q is not a known facet — the builder asks for a distribution nothing serves", param)
		}
		if attr == "" {
			t.Errorf("%q maps to an empty attribute", param)
		}
	}

	// The retired facet must not come back by being re-added to the table: the builder
	// no longer has a kind for it, so its distribution would be fetched and thrown away.
	if _, ok := search.StringFacets["role"]; ok {
		t.Error("role is served again — the suggestion builder has no kind for it")
	}
}
