package search

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/search/facetidx"
)

// facetd must index exactly what Meilisearch can filter on: an attribute added to one list
// and not the other answers "unknown facet" (or silently counts nothing) on the second engine.
func TestFacetidxIndexesEveryFilterableAttribute(t *testing.T) {
	var want []string
	for _, a := range facetSettings().FilterableAttributes {
		switch a {
		case "id", "is_closed", "is_duplicate": // facetidx's id map and closed/duplicate bitmaps
		default:
			want = append(want, a)
		}
	}
	got := facetidx.Attributes()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("facetidx attributes differ from the jobs index filterable attributes:\n meili:   %v\n facetidx: %v", want, got)
	}
}
