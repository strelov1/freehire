package search

import (
	"testing"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/dict/skillvec"
)

// The facet index carries exactly one embedder, and it must stay model-free: the
// vectors are arithmetic over a finite dictionary, so a rebuild still contacts no
// embedding service. A model-backed embedder here would turn every rebuild into a
// full-catalogue inference run.
func TestFacetSettingsDeclaresOneModelFreeEmbedder(t *testing.T) {
	s := facetSettings()
	if len(s.Embedders) != 1 {
		t.Fatalf("facetSettings() declares %d embedders, want exactly 1", len(s.Embedders))
	}
	e, ok := s.Embedders[SkillEmbedder]
	if !ok {
		t.Fatalf("facetSettings() declares no %q embedder; it has %v", SkillEmbedder, s.Embedders)
	}
	if e.Source != meilisearch.UserProvidedEmbedderSource {
		t.Errorf("embedder source = %q, want %q — no model may be called at index time",
			e.Source, meilisearch.UserProvidedEmbedderSource)
	}
	if e.Dimensions != skillvec.Dimensions {
		t.Errorf("embedder dimensions = %d, want %d; a mismatch makes the index reject every document",
			e.Dimensions, skillvec.Dimensions)
	}
	if e.BinaryQuantized {
		t.Error("binary quantization must stay off: on vectors this sparse it measured recall@20 of 10% against 95% unquantized, and lost the rare-skill signal entirely")
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

func TestRolesIsFilterable(t *testing.T) {
	// The role facet filters on a bare top-level `roles` attribute (derived at
	// index time), so it must be declared filterable for `role=` to take effect.
	s := facetSettings()
	if !contains(s.FilterableAttributes, "roles") {
		t.Errorf("roles must be filterable for the role facet, got %v", s.FilterableAttributes)
	}
}

func TestAIArchetypeIsFilterable(t *testing.T) {
	// The ai_archetype facet filters on a bare top-level `ai_archetype` attribute
	// (derived at index time by aiarchetype), so it must be declared filterable
	// for `ai_archetype=` to take effect.
	s := facetSettings()
	if !contains(s.FilterableAttributes, "ai_archetype") {
		t.Errorf("ai_archetype must be filterable for the ai_archetype facet, got %v", s.FilterableAttributes)
	}
	if StringFacets["ai_archetype"] != "ai_archetype" {
		t.Errorf("StringFacets[ai_archetype] = %q, want the bare attribute \"ai_archetype\"", StringFacets["ai_archetype"])
	}
}

func TestIsTechIsFilterableFacet(t *testing.T) {
	// is_tech is served top-level (jobview) and filtered on the bare attribute, so it
	// must be declared filterable for `is_tech=` to take effect and its facet counts.
	s := facetSettings()
	if !contains(s.FilterableAttributes, "is_tech") {
		t.Errorf("is_tech must be filterable for the tech/non-tech facet, got %v", s.FilterableAttributes)
	}
	if StringFacets["is_tech"] != "is_tech" {
		t.Errorf("StringFacets[is_tech] = %q, want the bare attribute \"is_tech\"", StringFacets["is_tech"])
	}
}

func TestIDIsFilterable(t *testing.T) {
	// The swipe deck excludes the caller's already-judged jobs via an
	// `id NOT IN [...]` filter, which requires id to be a filterable attribute.
	s := facetSettings()
	if !contains(s.FilterableAttributes, "id") {
		t.Errorf("id must be filterable for the swipe-deck exclusion, got %v", s.FilterableAttributes)
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

func TestFacetSettingsSkipsExpensiveProximityIndexing(t *testing.T) {
	// A local benchmark against a real ~317k-document sample of prod job data
	// showed Meilisearch's "merging word proximity" indexing phase costing up to
	// ~10s per 200-document batch (search-drain's push size), backed by a
	// wordPairProximityDocids structure larger than the documents themselves.
	// byAttribute skips building that structure entirely; job descriptions are
	// long-form prose, not short phrases where word adjacency is load-bearing for
	// ranking, so the relevancy trade-off is negligible.
	//
	// prefixSearch is deliberately NOT disabled despite being part of the same
	// benchmark's savings: HeaderSearch.svelte and the /jobs list's filters.ts both
	// debounce a query-as-you-type search through this index and rely on
	// Meilisearch's default last-word prefix matching to return results mid-word.
	s := facetSettings()
	if s.ProximityPrecision != meilisearch.ByAttribute {
		t.Errorf("facetSettings().ProximityPrecision = %q, want %q", s.ProximityPrecision, meilisearch.ByAttribute)
	}
	if s.PrefixSearch != nil {
		t.Errorf("facetSettings().PrefixSearch = %v, want nil (prefix search stays enabled for live query-as-you-type)", *s.PrefixSearch)
	}
}
