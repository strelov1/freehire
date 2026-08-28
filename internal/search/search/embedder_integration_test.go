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
		d, err := FromJob(j)
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

	withSkills, err := FromJob(job)
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
	cleared, err := FromJob(job)
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
