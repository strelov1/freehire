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
// because the filter PARAMETER is `role` while the index ATTRIBUTE is `roles`. Two
// names for one concept, and the builder had the wrong one written out as a literal.
//
// This does not need an engine. It only has to assert that the three names the
// builder asks for come from the table the filter builder already uses — so a
// renamed attribute breaks the build rather than the 06:46 timer.
func TestFacetNamesComeFromTheFilterTable(t *testing.T) {
	for _, param := range []string{"role", "skills", "category"} {
		attr, ok := search.StringFacets[param]
		if !ok {
			t.Fatalf("%q is not a known facet — the builder asks for a distribution nothing serves", param)
		}
		if attr == "" {
			t.Errorf("%q maps to an empty attribute", param)
		}
	}

	// The one that actually bit, named explicitly: it is the whole reason this test
	// exists, and a reader deserves to see the difference rather than infer it.
	if got := search.StringFacets["role"]; got == "role" {
		t.Errorf("role maps to %q — if the attribute ever equals the param, this test has stopped proving anything", got)
	}
}
