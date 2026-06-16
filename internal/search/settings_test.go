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
