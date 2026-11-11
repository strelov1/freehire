//go:build integration

package search

import (
	"context"
	"reflect"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// The retired `desc` embedder (#4417) left its chunk vectors inside every document that
// was not rewritten afterwards. The edit-by-function engine caps ONE document's total
// array elements at 10,000 and counts those leftovers, so a document with 14+ chunks of
// 768 floats made the whole task fail with "Size of array/BLOB too large" — and the wave
// holding it failed again on every lease-expiry retry.
func TestIntegration_MarkersSurviveLegacyDescriptionVectors(t *testing.T) {
	for _, status := range []struct {
		marker string
		mark   func(*Client, context.Context, []int64) error
	}{
		{marker: "is_closed", mark: (*Client).CloseJobs},
		{marker: "is_duplicate", mark: (*Client).MarkDuplicateJobs},
	} {
		t.Run(status.marker, func(t *testing.T) {
			ctx := context.Background()
			c := startMeili(t)
			if _, err := c.manager.ExperimentalFeatures().SetEditDocumentsByFunction(true).UpdateWithContext(ctx); err != nil {
				t.Fatal(err)
			}
			if err := c.EnsureIndex(ctx); err != nil {
				t.Fatal(err)
			}
			// Recreate the pre-#4417 state: declare `desc`, store chunk vectors under it,
			// then retire the embedder exactly as production did (EnsureIndex resets it).
			awaitOK(t, ctx, c, func() (*meilisearch.TaskInfo, error) {
				return c.facet.UpdateEmbeddersWithContext(ctx, map[string]meilisearch.Embedder{
					SkillEmbedder: {Source: meilisearch.UserProvidedEmbedderSource, Dimensions: 1280},
					"desc":        {Source: meilisearch.UserProvidedEmbedderSource, Dimensions: 768},
				})
			})
			skills := make([]float32, 1280)
			skills[0] = 1
			doc := func(id int, chunks int) map[string]any {
				emb := make([][]float32, chunks)
				for i := range emb {
					emb[i] = make([]float32, 768)
				}
				return map[string]any{
					"id": id, "title": "Golang engineer",
					"_vectors": map[string]any{
						SkillEmbedder: skills,
						"desc":        map[string]any{"embeddings": emb, "regenerate": false},
					},
				}
			}
			// 13 chunks (9,984 floats) is the largest document the engine still edits;
			// 14 (10,752) is the first it refuses.
			awaitOK(t, ctx, c, func() (*meilisearch.TaskInfo, error) {
				return c.facet.AddDocumentsWithContext(ctx, []map[string]any{doc(1, 13), doc(2, 14), doc(3, 40)}, nil)
			})
			if err := c.EnsureIndex(ctx); err != nil {
				t.Fatal(err)
			}

			var before map[string]any
			if err := c.facet.GetDocumentWithContext(ctx, "2", &meilisearch.DocumentQuery{RetrieveVectors: true}, &before); err != nil {
				t.Fatal(err)
			}
			// One wave: poison documents, a healthy one, and an id that was never indexed.
			if err := status.mark(c, ctx, []int64{1, 2, 3, 999}); err != nil {
				t.Fatalf("one oversized legacy document must not fail the wave: %v", err)
			}
			for _, id := range []string{"1", "2", "3"} {
				var raw map[string]any
				if err := c.facet.GetDocumentWithContext(ctx, id, &meilisearch.DocumentQuery{RetrieveVectors: true}, &raw); err != nil {
					t.Fatal(err)
				}
				if raw[status.marker] != true || raw["title"] != "Golang engineer" {
					t.Fatalf("doc %s not marked with its content kept: %v", id, raw)
				}
				vecs, _ := raw["_vectors"].(map[string]any)
				if !reflect.DeepEqual(vecs[SkillEmbedder], before["_vectors"].(map[string]any)[SkillEmbedder]) {
					t.Fatalf("doc %s lost or changed its live skill vector", id)
				}
				if _, kept := vecs["desc"]; kept {
					t.Fatalf("doc %s kept the retired desc vectors", id)
				}
			}
			stats, err := c.facet.GetStatsWithContext(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if stats.NumberOfDocuments != 3 {
				t.Fatalf("documents=%d, want 3: a missing id must not create a skeleton", stats.NumberOfDocuments)
			}
			// Replaying the same wave (lease expiry after a lost acknowledgement) is harmless.
			if err := status.mark(c, ctx, []int64{1, 2, 3, 999}); err != nil {
				t.Fatalf("replay: %v", err)
			}
		})
	}
}

func awaitOK(t *testing.T, ctx context.Context, c *Client, submit func() (*meilisearch.TaskInfo, error)) {
	t.Helper()
	task, err := submit()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.awaitTask(ctx, c.facet, task.TaskUID); err != nil {
		t.Fatal(err)
	}
}
