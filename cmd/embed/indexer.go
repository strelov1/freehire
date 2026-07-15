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
}

func (ix searchIndexer) IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]float32, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		doc, err := search.FromJob(job)
		if err != nil {
			return nil, fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		// Attach the job-reality signal so an incrementally-embedded job carries
		// reality.class immediately, matching the ingest incremental path. Without it a
		// recommendations query that positively filters on reality.class would silently
		// exclude the job until reindex --semantic reconciles it. A cluster-count lookup
		// failure degrades to a unique role (counts 1) rather than dropping the doc.
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
		docs = append(docs, doc)
	}
	// IndexSemanticJobs embeds the whole batch (chunked to the backend's limit) and
	// upserts it as ONE Meilisearch task, so a large backfill isn't per-doc bound. It
	// returns the computed vectors so the store can persist them beside the stamp.
	return ix.client.IndexSemanticJobs(ctx, docs)
}

func (ix searchIndexer) RemoveClosed(ctx context.Context, ids []int64) error {
	return ix.client.DeleteSemanticJobs(ctx, ids)
}
