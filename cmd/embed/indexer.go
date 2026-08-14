package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to embed.Indexer: compute each open job's
// pgvector-backed chunk embeddings. It works directly off the raw db.Job rows — no
// search.JobDocument is built, no search index is written or read here. (An earlier
// revision of this adapter also computed a legacy single-vector embedding via
// search.Client.EmbedJobs, persisted to jobs.semantic_embedding; that write was
// removed once nothing was left reading the column — job_semantic_chunks is the
// queryable representation now. See openspec/changes/drop-hybrid-search-pgvector-similar.)
type searchIndexer struct {
	client *search.Client
}

func (ix searchIndexer) IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]embed.ChunkEmbedding, error) {
	chunkVecs, err := ix.client.EmbedJobChunks(ctx, jobs)
	if err != nil {
		return nil, fmt.Errorf("embed chunks: %w", err)
	}
	chunks := make(map[int64][]embed.ChunkEmbedding, len(chunkVecs))
	for jobID, cs := range chunkVecs {
		out := make([]embed.ChunkEmbedding, len(cs))
		for i, c := range cs {
			out[i] = embed.ChunkEmbedding{ChunkIndex: c.ChunkIndex, Vector: c.Vector}
		}
		chunks[jobID] = out
	}
	return chunks, nil
}
