package search

import (
	"encoding/json"
	"testing"
)

// decodeEmbedding must pull one vector out of every shape Meilisearch's
// _vectors.<name> payload takes across versions: the object form with an
// array-of-vectors, the object form with a single vector, and a bare array — so the
// CV read-back does not break on a Meili upgrade.
func TestDecodeEmbedding(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []float64
		wantErr bool
	}{
		{"object array-of-vectors", `{"embeddings": [[1, 2, 3]], "regenerate": true}`, []float64{1, 2, 3}, false},
		{"object single vector", `{"embeddings": [1, 2, 3]}`, []float64{1, 2, 3}, false},
		{"bare array-of-vectors", `[[0.5, -0.5]]`, []float64{0.5, -0.5}, false},
		{"object empty embeddings", `{"embeddings": []}`, nil, true},
		{"empty object", `{}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeEmbedding(json.RawMessage(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
