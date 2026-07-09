package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to embed.Indexer: embed+upsert an open job's
// vector in place (no swap), or remove a closed job's document. It builds the document
// from the persisted row (search.FromJob) so a re-embedded job keeps its enrichment
// facets — the same path the incremental ingest indexer uses.
type searchIndexer struct {
	client *search.Client
}

func (ix searchIndexer) IndexOpen(ctx context.Context, job db.Job) error {
	doc, err := search.FromJob(job)
	if err != nil {
		return fmt.Errorf("build document: %w", err)
	}
	return ix.client.IndexSemanticJobs(ctx, []search.JobDocument{doc})
}

func (ix searchIndexer) RemoveClosed(ctx context.Context, jobID int64) error {
	return ix.client.DeleteSemanticJobs(ctx, []int64{jobID})
}
