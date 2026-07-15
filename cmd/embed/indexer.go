package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to embed.Indexer: embed+upsert a batch of open
// jobs' vectors in place (no swap), or remove a batch of closed jobs' documents. It
// builds each document from the persisted row (search.FromJob) so a re-embedded job
// keeps its enrichment facets — the same path the incremental ingest indexer uses. It
// also attaches the job-reality signal so a job first indexed here carries reality.class
// immediately, rather than only after the next full reindex.
type searchIndexer struct {
	client *search.Client
	q      *db.Queries
	// pgOnly makes the indexer embed to Postgres ONLY: it computes vectors (persisted by
	// the store's CompleteOpen) but never writes Meilisearch, so a bulk backfill is not
	// gated by Meili's serial task queue. The semantic index is rebuilt from Postgres
	// afterwards with `reindex --semantic --from-pg`.
	pgOnly bool
}

func (ix searchIndexer) IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]float32, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		doc, err := search.FromJob(job)
		if err != nil {
			return nil, fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		// The job-reality signal is a Meili-document facet, so pg-only mode (which never
		// builds a Meili doc) skips it — and its per-job cluster-count query. In the
		// normal path attach it so an incrementally-embedded job carries reality.class
		// immediately; a cluster-count lookup failure degrades to a unique role (1,1).
		if !ix.pgOnly {
			repost, mass := int64(1), int64(1)
			if c, err := ix.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
				CompanySlug:     job.CompanySlug,
				RoleFingerprint: job.RoleFingerprint,
			}); err != nil {
				log.Printf("embed: role-cluster count for job %d: %v", job.ID, err)
			} else {
				repost, mass = c.RepostCount, c.MassCount
			}
			reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
			doc.Reality = &reality
		}
		docs = append(docs, doc)
	}
	// pg-only: compute vectors only (the store persists them to Postgres); Meili is
	// rebuilt from Postgres afterwards via `reindex --semantic --from-pg`. Otherwise embed
	// the whole batch AND upsert it into the live semantic index as ONE Meili task.
	if ix.pgOnly {
		return ix.client.EmbedJobs(ctx, docs)
	}
	return ix.client.IndexSemanticJobs(ctx, docs)
}

func (ix searchIndexer) RemoveClosed(ctx context.Context, ids []int64) error {
	// pg-only mode leaves Meili untouched: the closed job's stamp+vector are cleared in
	// Postgres by CompleteClosed, and the --from-pg rebuild simply omits closed jobs.
	if ix.pgOnly {
		return nil
	}
	return ix.client.DeleteSemanticJobs(ctx, ids)
}
