//go:build integration

// Integration tests for the skill embedder's LIFECYCLE against a real engine: that it
// survives the very calls that apply index settings.
//
// The unit tests assert facetSettings() declares the embedder. That is not the same
// claim: settings are applied and then adjusted, so an embedder can be declared and
// still be absent from the live index a moment later. This file tests the index, not
// the struct.
package search

import (
	"context"
	"testing"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestIntegration_EnsureIndexLeavesTheSkillEmbedderInPlace is the regression that
// matters: EnsureIndex applies the settings and then used to reset every embedder,
// which would have declared the skill embedder and deleted it in the same call. The
// match sort would have returned a 400 with nothing in the code looking wrong.
func TestIntegration_EnsureIndexLeavesTheSkillEmbedderInPlace(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	got, err := c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	e, ok := got[SkillEmbedder]
	if !ok {
		t.Fatalf("the live index carries no %q embedder after EnsureIndex; it has %v", SkillEmbedder, got)
	}
	if e.Dimensions != skillvec.Dimensions {
		t.Errorf("live embedder dimensions = %d, want %d", e.Dimensions, skillvec.Dimensions)
	}
}

// A rebuild is how the embedder reaches production at all, so the same claim has to
// hold for the index a rebuild prepares — and it is the path that was broken.
func TestIntegration_RebuildPreparesTheSkillEmbedder(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	r := c.NewFacetRebuild()
	if err := r.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	got, err := r.rebuild.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	if _, ok := got[SkillEmbedder]; !ok {
		t.Fatalf("the rebuild index carries no %q embedder; a rebuild would ship without the match sort", SkillEmbedder)
	}
}

// End to end against the engine: a document carrying a vector is accepted, and a
// vector search over it ranks. This is what a 400 from a missing embedder would break.
func TestIntegration_VectorSearchRanksByTheSkillVector(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{ID: 1, Title: "Go Engineer", Company: "Acme", Location: "Berlin", PublicSlug: "go-acme-a",
			Category: "backend", Description: "Build things.", Skills: []string{"go", "docker"}},
		{ID: 2, Title: "Erlang Engineer", Company: "Beta", Location: "Berlin", PublicSlug: "erlang-beta-b",
			Category: "backend", Description: "Build other things.", Skills: []string{"erlang"}},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, nil, "")
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	// A candidate who knows Go and Docker: the Go posting must come first.
	res, err := c.Search(ctx, SearchParams{Vector: skillvec.ProfileVector([]string{"go", "docker"}), Limit: 10})
	if err != nil {
		t.Fatalf("vector Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("vector search returned nothing")
	}
	if res.Hits[0].ID != 1 {
		t.Errorf("top hit id = %d, want 1 (the Go posting) — ranking is not following the vector", res.Hits[0].ID)
	}
}

// The clearing semantics, against the engine that motivated them: pushing a job that
// has lost its skills must remove the stored vector, not merge around it.
func TestIntegration_LosingSkillsClearsTheStoredVector(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	job := db.Job{ID: 1, Title: "Go Engineer", Company: "Acme", Location: "Berlin",
		PublicSlug: "go-acme-a", Category: "backend", Description: "Build things.",
		Skills: []string{"go", "docker"}}

	withSkills, err := FromJob(job, nil, "")
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if err := c.IndexJobs(ctx, []JobDocument{withSkills}); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
	if res, err := c.Search(ctx, SearchParams{Vector: skillvec.ProfileVector([]string{"go"}), Limit: 10}); err != nil {
		t.Fatalf("vector Search: %v", err)
	} else if len(res.Hits) == 0 {
		t.Fatal("the job is not rankable by vector before losing its skills")
	}

	// The same job, re-indexed with its skills gone.
	job.Skills = nil
	cleared, err := FromJob(job, nil, "")
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if err := c.IndexJobs(ctx, []JobDocument{cleared}); err != nil {
		t.Fatalf("IndexJobs (cleared): %v", err)
	}

	// Read the STORED document rather than inferring from a search. A vector search
	// over a one-document index returns that document whether or not it has a vector,
	// so "is it still in the results" cannot answer this; "what does the index hold"
	// can.
	var stored struct {
		Vectors map[string]struct {
			Embeddings [][]float32 `json:"embeddings"`
		} `json:"_vectors"`
	}
	if err := c.facet.GetDocumentWithContext(ctx, "1", &meilisearch.DocumentQuery{RetrieveVectors: true}, &stored); err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if n := len(stored.Vectors[SkillEmbedder].Embeddings); n != 0 {
		t.Errorf("the index still holds %d embedding(s) for a job with no skills — the clear merged instead of replacing", n)
	}

	// And the job is still there: clearing a vector is not a deletion.
	res, err := c.Search(ctx, SearchParams{Query: "Go Engineer", Limit: 10})
	if err != nil {
		t.Fatalf("keyword Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Error("clearing the vector removed the job from keyword search")
	}
}

// storedVectorCounts reads a document back with its vectors and reports how many
// embeddings the index holds under each embedder.
func storedVectorCounts(t *testing.T, c *Client, id string) map[string]int {
	t.Helper()
	var stored struct {
		Vectors map[string]struct {
			Embeddings [][]float32 `json:"embeddings"`
		} `json:"_vectors"`
	}
	if err := c.facet.GetDocumentWithContext(context.Background(), id, &meilisearch.DocumentQuery{RetrieveVectors: true}, &stored); err != nil {
		t.Fatalf("GetDocument %s: %v", id, err)
	}
	out := map[string]int{}
	for name, v := range stored.Vectors {
		out[name] = len(v.Embeddings)
	}
	return out
}

// unitDesc builds a descDimensions-wide vector that is 1 in one dimension.
func unitDesc(dim int) []float32 {
	v := make([]float32, descDimensions)
	v[dim] = 1
	return v
}

// The description embedder has to survive the same two settings paths the skill one
// does — and a document has to carry every chunk, not just the first.
func TestIntegration_DescEmbedderSurvivesSettingsAndHoldsEveryChunk(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	live, err := c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	if e, ok := live[DescEmbedder]; !ok || e.Dimensions != descDimensions {
		t.Fatalf("live index desc embedder = %+v (present %v), want %d dimensions", e, ok, descDimensions)
	}

	r := c.NewFacetRebuild()
	if err := r.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	rebuilt, err := r.rebuild.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders (rebuild): %v", err)
	}
	if _, ok := rebuilt[DescEmbedder]; !ok {
		t.Fatalf("the rebuild index carries no %q embedder", DescEmbedder)
	}

	job := db.Job{ID: 1, Title: "Go Engineer", Company: "Acme", Location: "Berlin",
		PublicSlug: "go-acme-a", Category: "backend", Description: "Build things.",
		Skills: []string{"go"}}
	doc, err := FromJob(job, [][]float32{unitDesc(0), unitDesc(1), unitDesc(2)}, "")
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if err := c.IndexJobs(ctx, []JobDocument{doc}); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
	if got := storedVectorCounts(t, c, "1"); got[DescEmbedder] != 3 || got[SkillEmbedder] != 1 {
		t.Errorf("stored embeddings = %v, want 3 desc and 1 skills", got)
	}
}

// The drain re-pushes a document whenever anything about the posting changes. That push
// is an add-or-update, and both embedders must come out of it intact: a re-push that
// changed only the skills must not cost the description vectors, and a job whose chunks
// were deleted must lose them.
func TestIntegration_RepushKeepsBothEmbeddersInStep(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	job := db.Job{ID: 1, Title: "Go Engineer", Company: "Acme", Location: "Berlin",
		PublicSlug: "go-acme-a", Category: "backend", Description: "Build things.",
		Skills: []string{"go"}}
	desc := [][]float32{unitDesc(0), unitDesc(1)}
	push := func(j db.Job, d [][]float32) {
		t.Helper()
		doc, err := FromJob(j, d, "")
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		if err := c.IndexJobs(ctx, []JobDocument{doc}); err != nil {
			t.Fatalf("IndexJobs: %v", err)
		}
	}

	push(job, desc)
	job.Skills = []string{"go", "docker"}
	push(job, desc)
	if got := storedVectorCounts(t, c, "1"); got[DescEmbedder] != 2 || got[SkillEmbedder] != 1 {
		t.Errorf("after a skills-only change, stored embeddings = %v, want 2 desc and 1 skills", got)
	}

	push(job, nil)
	if got := storedVectorCounts(t, c, "1"); got[DescEmbedder] != 0 {
		t.Errorf("after the chunks were deleted, stored desc embeddings = %d, want 0", got[DescEmbedder])
	}
}

// End to end: a hybrid query finds a posting whose words do not match the query but
// whose description vector does, keeps the caller's filter, and the threshold keeps a
// posting that matches neither off the page.
func TestIntegration_HybridSearchFindsByMeaning(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []struct {
		job  db.Job
		desc [][]float32
	}{
		{db.Job{ID: 1, Title: "Platform Engineer", Company: "Acme", Location: "Berlin", Countries: []string{"DE"},
			PublicSlug: "platform-acme-a", Category: "backend", Description: "Ledger services."}, [][]float32{unitDesc(5), unitDesc(6)}},
		{db.Job{ID: 2, Title: "Platform Engineer", Company: "Beta", Location: "Paris", Countries: []string{"FR"},
			PublicSlug: "platform-beta-b", Category: "backend", Description: "Ledger services."}, [][]float32{unitDesc(6)}},
		{db.Job{ID: 3, Title: "Pastry Chef", Company: "Gamma", Location: "Berlin", Countries: []string{"DE"},
			PublicSlug: "pastry-gamma-c", Category: "backend", Description: "Croissants."}, [][]float32{unitDesc(100)}},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j.job, j.desc, "")
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	// "payments" appears in no posting; the query vector points at chunk 6, which jobs 1
	// and 2 both carry — so only the country filter can keep job 2 out.
	res, err := c.Search(ctx, SearchParams{
		Query:          "payments",
		SemanticVector: unitDesc(6),
		Filter:         FilterFromValues(map[string][]string{"countries": {"DE"}}),
		ScoreThreshold: 0.6,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("hybrid Search: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].ID != 1 {
		ids := make([]int64, len(res.Hits))
		for i, h := range res.Hits {
			ids[i] = h.ID
		}
		t.Errorf("hits = %v, want only job 1 (meaning match, inside the DE filter, pastry chef below threshold)", ids)
	}

	// A query with OR alternatives goes out as one federated multi-search; every
	// alternative carries the vector, the filter and the threshold.
	fed, err := c.Search(ctx, SearchParams{
		Query:          "payments OR billing",
		SemanticVector: unitDesc(6),
		Filter:         FilterFromValues(map[string][]string{"countries": {"DE"}}),
		ScoreThreshold: 0.6,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("federated hybrid Search: %v", err)
	}
	if len(fed.Hits) != 1 || fed.Hits[0].ID != 1 {
		t.Errorf("federated hits = %d (first %v), want only job 1", len(fed.Hits), fed.Hits)
	}
}
