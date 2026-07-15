package search

import "testing"

func TestParseSemanticVectorsPage(t *testing.T) {
	// Shape mirrors the live Meili response: _vectors.default.embeddings is a list of
	// embeddings ([[...]]) and the seeded vector is its single element.
	raw := []byte(`{
	  "results": [
	    {"id": 1, "_vectors": {"default": {"embeddings": [[0.1, 0.2, 0.3]], "regenerate": false}}},
	    {"id": 2, "_vectors": {"default": {"embeddings": [[0.4, 0.5, 0.6]], "regenerate": false}}}
	  ],
	  "offset": 0, "limit": 2, "total": 100
	}`)

	got, err := parseSemanticVectorsPage(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d vectors, want 2", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("ids = %d,%d; want 1,2", got[0].ID, got[1].ID)
	}
	want0 := []float32{0.1, 0.2, 0.3}
	if len(got[0].Vector) != 3 || got[0].Vector[0] != want0[0] || got[0].Vector[2] != want0[2] {
		t.Fatalf("vector[0] = %v; want %v", got[0].Vector, want0)
	}
}

func TestParseSemanticVectorsPageSkipsDocsWithoutVector(t *testing.T) {
	// A doc with no _vectors, or an empty embeddings list, has nothing to persist —
	// it must be skipped, not surfaced as a zero-length vector that would overwrite
	// a real one with garbage.
	raw := []byte(`{
	  "results": [
	    {"id": 1, "_vectors": {"default": {"embeddings": [[0.1, 0.2]], "regenerate": false}}},
	    {"id": 2},
	    {"id": 3, "_vectors": {"default": {"embeddings": [], "regenerate": false}}}
	  ],
	  "offset": 0, "limit": 3, "total": 3
	}`)

	got, err := parseSemanticVectorsPage(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d vectors, want 1 (only id 1 carries a vector)", len(got))
	}
	if got[0].ID != 1 {
		t.Fatalf("kept id %d; want 1", got[0].ID)
	}
}

func TestParseSemanticVectorsPageEmpty(t *testing.T) {
	// The end-of-pagination page: no results, no error, empty slice.
	got, err := parseSemanticVectorsPage([]byte(`{"results": [], "offset": 200, "limit": 100, "total": 200}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d vectors, want 0", len(got))
	}
}
