package search

import "testing"

func TestSemanticDocsFromPG(t *testing.T) {
	docs := []JobDocument{
		{ID: 1, semanticVector: []float32{0.1, 0.2}},
		{ID: 2}, // no persisted vector — must be skipped
		{ID: 3, semanticVector: []float32{0.3, 0.4}},
	}

	got := semanticDocsFromPG(docs)

	if len(got) != 2 {
		t.Fatalf("got %d semantic docs, want 2 (id 2 has no vector)", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("ids = %d,%d; want 1,3", got[0].ID, got[1].ID)
	}
	// The persisted vector must ride _vectors under the embedder name, unchanged.
	v := got[0].Vectors[embedderName]
	if len(v) != 2 || v[0] != 0.1 || v[1] != 0.2 {
		t.Fatalf("vector for id 1 = %v; want [0.1 0.2]", v)
	}
}

func TestSemanticDocsFromPGAllEmpty(t *testing.T) {
	got := semanticDocsFromPG([]JobDocument{{ID: 1}, {ID: 2}})
	if len(got) != 0 {
		t.Fatalf("got %d docs, want 0 (none carry a vector)", len(got))
	}
}
