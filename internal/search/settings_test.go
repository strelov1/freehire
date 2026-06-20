package search

import (
	"reflect"
	"testing"
)

// The reindex split keeps two index configurations: a facet/keyword index with NO
// embedder (the fast, always-fresh production search) and a semantic index that
// adds the embedder (built by a separate, optional pass). Both must share the
// facet/keyword settings so keyword search and faceting behave identically.

func TestFacetSettingsHasNoEmbedder(t *testing.T) {
	if facetSettings().Embedders != nil {
		t.Error("facetSettings() must not configure an embedder (keeps the facet reindex fast)")
	}
}

func TestSemanticSettingsHasEmbedder(t *testing.T) {
	s := semanticSettings()
	if s.Embedders == nil {
		t.Fatal("semanticSettings() must configure the embedder")
	}
	if _, ok := s.Embedders[embedderName]; !ok {
		t.Errorf("semanticSettings() missing the %q embedder", embedderName)
	}
}

func TestPostedTSIsFilterableNotSortable(t *testing.T) {
	// posted_ts backs the "posted within N days" range filter, so it must be a
	// filterable attribute. Sorting still uses the string posted_at, so posted_ts
	// is deliberately NOT added to the sortable attributes.
	s := facetSettings()
	if !contains(s.FilterableAttributes, "posted_ts") {
		t.Errorf("posted_ts must be filterable, got %v", s.FilterableAttributes)
	}
	if contains(s.SortableAttributes, "posted_ts") {
		t.Errorf("posted_ts must not be sortable (sort uses posted_at), got %v", s.SortableAttributes)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestFacetAndSemanticShareKeywordSettings(t *testing.T) {
	f, s := facetSettings(), semanticSettings()
	if !reflect.DeepEqual(f.FilterableAttributes, s.FilterableAttributes) {
		t.Error("facet and semantic settings must share FilterableAttributes")
	}
	if !reflect.DeepEqual(f.SearchableAttributes, s.SearchableAttributes) {
		t.Error("facet and semantic settings must share SearchableAttributes")
	}
	if !reflect.DeepEqual(f.SortableAttributes, s.SortableAttributes) {
		t.Error("facet and semantic settings must share SortableAttributes")
	}
}
