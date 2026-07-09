package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to embed.Indexer: embed+upsert a batch of open
// jobs' vectors in place (no swap), or remove a batch of closed jobs' documents. It
// builds each document from the persisted row (search.FromJob) so a re-embedded job
// keeps its enrichment facets — the same path the incremental ingest indexer uses.
type searchIndexer struct {
	client *search.Client
}

func (ix searchIndexer) IndexOpen(ctx context.Context, jobs []db.Job) error {
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		doc, err := search.FromJob(job)
		if err != nil {
			return fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		docs = append(docs, doc)
	}
	// IndexSemanticJobs embeds the whole batch (chunked to the backend's limit) and
	// upserts it as ONE Meilisearch task, so a large backfill isn't per-doc bound.
	return ix.client.IndexSemanticJobs(ctx, docs)
}

func (ix searchIndexer) RemoveClosed(ctx context.Context, ids []int64) error {
	return ix.client.DeleteSemanticJobs(ctx, ids)
}
