package search

import "testing"

func TestSearchRequestWithoutAVectorAsksForNoHybridRanking(t *testing.T) {
	req := buildSearchRequest(SearchParams{Query: "golang", Sort: []string{"posted_at:desc"}, Limit: 20})
	if req.Vector != nil {
		t.Errorf("Vector = %v, want nil", req.Vector)
	}
	if req.Hybrid != nil {
		t.Errorf("Hybrid = %+v, want nil — a hybrid directive without a vector changes plain keyword ranking", req.Hybrid)
	}
	if req.Limit != 20 {
		t.Errorf("Limit = %d, want 20", req.Limit)
	}
}

// A Vector without an embedder name is a 400 from the engine, so the two must always
// travel together.
func TestSearchRequestWithAVectorAsksForPureSemanticRanking(t *testing.T) {
	v := make([]float32, 8)
	v[0] = 1
	req := buildSearchRequest(SearchParams{Vector: v, Limit: 20})

	if len(req.Vector) != len(v) {
		t.Fatalf("Vector has %d values, want %d", len(req.Vector), len(v))
	}
	if req.Hybrid == nil {
		t.Fatal("Hybrid is nil; a vector without an embedder name is a 400 from the engine")
	}
	if req.Hybrid.Embedder != SkillEmbedder {
		t.Errorf("Hybrid.Embedder = %q, want %q", req.Hybrid.Embedder, SkillEmbedder)
	}
	if req.Hybrid.SemanticRatio != 1 {
		t.Errorf("Hybrid.SemanticRatio = %v, want 1 — anything less blends in a keyword score that is noise for an empty query",
			req.Hybrid.SemanticRatio)
	}
}

func TestSearchRequestCarriesTheFilterAlongsideTheVector(t *testing.T) {
	v := make([]float32, 8)
	v[0] = 1
	req := buildSearchRequest(SearchParams{Vector: v, Filter: `country = "DE"`, Limit: 20})

	if req.Filter == nil {
		t.Error("Filter was dropped when a vector was present; the two must compose in one query")
	}
	if len(req.Vector) == 0 {
		t.Error("Vector was dropped when a filter was present")
	}
}

func TestSearchRequestPassesPaginationThrough(t *testing.T) {
	req := buildSearchRequest(SearchParams{Limit: 25, Offset: 400})
	if req.Limit != 25 || req.Offset != 400 {
		t.Errorf("limit/offset = %d/%d, want 25/400", req.Limit, req.Offset)
	}
}
