package search

import "testing"

func TestSemanticDocsFromPG(t *testing.T) {
	docs := []JobDocument{
		{ID: 1, semanticVectors: [][]float32{{0.1, 0.2}}}, // legacy single-chunk shape
		{ID: 2}, // no persisted vectors — must be skipped
		{ID: 3, semanticVectors: [][]float32{{0.3, 0.4}, {0.5, 0.6}}}, // real multi-chunk shape
	}

	got := semanticDocsFromPG(docs)

	if len(got) != 2 {
		t.Fatalf("got %d semantic docs, want 2 (id 2 has no vectors)", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("ids = %d,%d; want 1,3", got[0].ID, got[1].ID)
	}
	// The persisted vectors must ride _vectors under the embedder name, unchanged.
	v1 := got[0].Vectors[embedderName]
	if len(v1) != 1 || v1[0][0] != 0.1 || v1[0][1] != 0.2 {
		t.Fatalf("vectors for id 1 = %v; want [[0.1 0.2]]", v1)
	}
	v3 := got[1].Vectors[embedderName]
	if len(v3) != 2 || v3[1][0] != 0.5 || v3[1][1] != 0.6 {
		t.Fatalf("vectors for id 3 = %v; want [[0.3 0.4] [0.5 0.6]]", v3)
	}
}

func TestSemanticDocsFromPGAllEmpty(t *testing.T) {
	got := semanticDocsFromPG([]JobDocument{{ID: 1}, {ID: 2}})
	if len(got) != 0 {
		t.Fatalf("got %d docs, want 0 (none carry a vector)", len(got))
	}
}
