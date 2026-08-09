package search

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// SemanticChunksFromArray must treat a row still holding the OLD single-vector shape
// (one dimension) as a single-chunk list — byte-for-byte the pre-chunking behavior —
// so a job not yet re-embedded under the new scheme stays fully readable everywhere
// db.Job.SemanticEmbedding is scanned (not just the embed pipeline), rather than
// erroring for the whole re-embed drain window (design.md Decision 3).
func TestSemanticChunksFromArray_OneDimensionIsSingleChunk(t *testing.T) {
	a := pgtype.Array[float32]{
		Elements: []float32{0.1, 0.2, 0.3},
		Dims:     []pgtype.ArrayDimension{{Length: 3, LowerBound: 1}},
		Valid:    true,
	}
	want := [][]float32{{0.1, 0.2, 0.3}}
	if got := SemanticChunksFromArray(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("SemanticChunksFromArray(1D) = %v, want %v", got, want)
	}
}

// A row already re-embedded under the new scheme carries a real 2-dimension array —
// this reshapes it back into its per-chunk rows, in order.
func TestSemanticChunksFromArray_TwoDimensionsReshapesToRows(t *testing.T) {
	a := pgtype.Array[float32]{
		Elements: []float32{1, 2, 3, 4, 5, 6},
		Dims:     []pgtype.ArrayDimension{{Length: 3, LowerBound: 1}, {Length: 2, LowerBound: 1}},
		Valid:    true,
	}
	want := [][]float32{{1, 2}, {3, 4}, {5, 6}}
	if got := SemanticChunksFromArray(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("SemanticChunksFromArray(2D) = %v, want %v", got, want)
	}
}

// A NULL column (never embedded) or an explicitly empty array must reshape to no
// vectors at all, not a slice containing an empty chunk.
func TestSemanticChunksFromArray_InvalidOrEmptyIsNil(t *testing.T) {
	cases := []struct {
		name string
		a    pgtype.Array[float32]
	}{
		{"NULL column", pgtype.Array[float32]{Valid: false}},
		{"zero dimensions", pgtype.Array[float32]{Valid: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SemanticChunksFromArray(c.a); got != nil {
				t.Fatalf("SemanticChunksFromArray(%s) = %v, want nil", c.name, got)
			}
		})
	}
}

// SemanticChunksToArray is the write-side inverse: chunk vectors flatten into one
// Elements slice described by a 2-dimension Dims, ready for SetSemanticEmbedding's
// parameter — the exact shape Postgres accepts directly into the real[]-declared
// column (verified empirically against a disposable Postgres instance, design.md
// Decision 3).
func TestSemanticChunksToArray_RoundTripsWithFromArray(t *testing.T) {
	vecs := [][]float32{{1, 2}, {3, 4}, {5, 6}}
	arr := SemanticChunksToArray(vecs)
	if !arr.Valid {
		t.Fatalf("SemanticChunksToArray(%v).Valid = false, want true", vecs)
	}
	if got := SemanticChunksFromArray(arr); !reflect.DeepEqual(got, vecs) {
		t.Fatalf("round trip = %v, want %v", got, vecs)
	}
}

// An empty chunk list (nothing to persist) must produce an invalid array — the
// parameter that writes SQL NULL — not a zero-length, zero-dimension array value.
func TestSemanticChunksToArray_EmptyIsInvalid(t *testing.T) {
	if arr := SemanticChunksToArray(nil); arr.Valid {
		t.Fatalf("SemanticChunksToArray(nil).Valid = true, want false (NULL)")
	}
}
