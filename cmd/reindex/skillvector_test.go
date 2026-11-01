package main

import (
	"context"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// A rebuild is the only path that reaches every posting, so it is the one that has to
// carry the skill vectors. A rebuild that quietly dropped them would leave the whole
// catalogue unrankable by match with nothing failing.
func TestSplitJobs_CarriesTheSkillVector(t *testing.T) {
	job := db.Job{
		ID: 1, Title: "Go Engineer", PublicSlug: "go-x", Category: "backend",
		Description: "<p>Build things.</p>", Skills: []string{"go", "erlang"},
	}

	docs, _, err := splitJobs([]db.Job{job}, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("splitJobs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if got := len(docs[0].Vectors.Skills); got != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", got, skillvec.Dimensions)
	}
}

// fakeChunkLister serves fixed chunk rows and records which ids it was asked for.
type fakeChunkLister struct {
	rows  []db.JobSemanticChunk
	asked []int64
}

func (f *fakeChunkLister) ListSemanticChunksForJobs(_ context.Context, ids []int64) ([]db.JobSemanticChunk, error) {
	f.asked = append(f.asked, ids...)
	return f.rows, nil
}

// The rebuild is also the only path that reaches every posting with its description
// vectors. A job with chunks carries each of them; a job without carries none, which the
// document serialises as the clear Meilisearch expects.
func TestStreamOpenDocs_CarriesTheDescriptionVectors(t *testing.T) {
	embedded := db.Job{ID: 1, Title: "A", PublicSlug: "a", Category: "backend", Description: "<p>Build things.</p>"}
	bare := db.Job{ID: 2, Title: "B", PublicSlug: "b", Category: "backend", Description: "<p>Build things.</p>"}
	reader := &fakePageReader{pages: map[int64][]db.Job{0: {embedded, bare}}}
	chunks := &fakeChunkLister{rows: []db.JobSemanticChunk{
		{JobID: 1, ChunkIndex: 0, Embedding: pgvector.NewVector([]float32{1, 0})},
		{JobID: 1, ChunkIndex: 1, Embedding: pgvector.NewVector([]float32{0, 1})},
	}}

	var got []search.JobDocument
	_, _, err := streamOpenDocs(context.Background(), reader, nil, nil, chunks, nil, time.Now(),
		func(_ context.Context, docs []search.JobDocument) error {
			got = append(got, docs...)
			return nil
		})
	if err != nil {
		t.Fatalf("streamOpenDocs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("docs = %d, want 2", len(got))
	}
	if n := len(got[0].Vectors.Desc); n != 2 {
		t.Errorf("job 1 desc vectors = %d, want 2", n)
	}
	if got[1].Vectors.Desc != nil {
		t.Errorf("job 2 desc vectors = %v, want nil", got[1].Vectors.Desc)
	}
	if len(chunks.asked) != 2 {
		t.Errorf("asked for %v, want both page ids in one call", chunks.asked)
	}
}
