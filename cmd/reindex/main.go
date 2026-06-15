// Command reindex rebuilds the Meilisearch jobs index from Postgres. It ensures
// the index settings exist, then scans jobs in batches and upserts their
// documents. Run it on a schedule (e.g. cron); it processes the whole table and
// exits. Indexing is idempotent (upsert by id), so re-runs are safe.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to
// Meilisearch per round. A const for now; promote to config if it needs tuning.
const reindexBatchSize = 500

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Bootstrap owns config + pool, so this required-config check lands just after
	// the pool opens rather than before it. The connect is cheap and cleanup closes
	// it on this early return, so the only cost of a missing key is one DB handshake.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	indexed, deleted, err := reindexAll(ctx, db.New(pool), client)
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}

	log.Printf("reindex done: indexed=%d deleted=%d", indexed, deleted)
	return 0
}

// reindexAll ensures the index and streams every job through it in batches,
// returning how many documents were indexed (open jobs) and deleted (closed
// jobs). It pages by keyset (id > last seen), so rows inserted or re-ordered
// during the run cannot be skipped or repeated.
func reindexAll(ctx context.Context, q *db.Queries, client *search.Client) (indexed, deleted int, err error) {
	if err := client.EnsureIndex(ctx); err != nil {
		return 0, 0, err
	}

	var afterID int64
	for {
		jobs, err := q.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
			AfterID:   afterID,
			BatchSize: reindexBatchSize,
		})
		if err != nil {
			return indexed, deleted, err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		docs, deleteIDs, err := splitJobs(jobs)
		if err != nil {
			return indexed, deleted, err
		}
		if err := client.IndexJobs(ctx, docs); err != nil {
			return indexed, deleted, err
		}
		if err := client.DeleteJobs(ctx, deleteIDs); err != nil {
			return indexed, deleted, err
		}
		indexed += len(docs)
		deleted += len(deleteIDs)

		if len(jobs) < reindexBatchSize {
			break
		}
	}

	return indexed, deleted, nil
}

// splitJobs partitions a batch from the (deliberately unfiltered) reindex feed:
// open jobs become index documents, closed jobs become deletions so they leave
// the index (the index contains only open jobs — see the job-search spec).
func splitJobs(jobs []db.Job) ([]search.JobDocument, []int64, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	deleteIDs := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		if j.ClosedAt.Valid {
			deleteIDs = append(deleteIDs, j.ID)
			continue
		}
		doc, err := search.FromJob(j)
		if err != nil {
			return nil, nil, err
		}
		docs = append(docs, doc)
	}
	return docs, deleteIDs, nil
}
