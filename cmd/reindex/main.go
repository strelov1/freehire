// Command reindex rebuilds a Meilisearch jobs index from Postgres. It ensures the
// index settings exist, then scans jobs in batches and upserts their documents.
// Run it on a schedule (e.g. cron); it processes the whole table and exits.
// Indexing is idempotent (upsert by id), so re-runs are safe.
//
// Two passes share this binary:
//
//   - default: the facet/keyword index (no embedder) — the fast, always-fresh
//     production search. A full rebuild is minutes, not hours.
//   - reindex --semantic: the hybrid index (adds the in-engine embedder). Slower
//     (it embeds new/changed documents); run on its own, looser schedule and only
//     while semantic search is enabled — it never blocks the facet pass.
//
// --since <duration> scopes either pass to jobs changed within that window
// (keyed on updated_at), so a frequent run re-pushes only the delta instead of
// the whole table. Meilisearch already skips re-embedding unchanged documents;
// --since additionally skips reading and re-pushing them at all.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to
// Meilisearch per round. Once the facet index dropped the per-document embedder,
// the per-batch round-trip became the throughput lever, so the batch is sized up
// from 500 to amortize it (Postgres read and the ~7KB-doc payload are both cheap
// at this size). A const for now; promote to config if it needs tuning.
const reindexBatchSize = 2000

// indexOps is the set of index operations a reindex pass drives. The default pass
// targets the facet/keyword index; --semantic targets the hybrid index. Selecting
// ops up front keeps the streaming loop identical for both.
type indexOps struct {
	name   string
	ensure func(context.Context) error
	index  func(context.Context, []search.JobDocument) error
	remove func(context.Context, []int64) error
}

func facetOps(c *search.Client) indexOps {
	return indexOps{"facet", c.EnsureIndex, c.IndexJobs, c.DeleteJobs}
}

func semanticOps(c *search.Client) indexOps {
	return indexOps{"semantic", c.EnsureSemanticIndex, c.IndexSemanticJobs, c.DeleteSemanticJobs}
}

// pageFetcher returns the next keyset page of jobs after the given id (empty when
// the scan is exhausted). The full scan returns every job; the incremental scan
// (reindex --since) returns only those changed at or after a cutoff.
type pageFetcher func(ctx context.Context, afterID int64) ([]db.Job, error)

func fullScan(q *db.Queries) pageFetcher {
	return func(ctx context.Context, afterID int64) ([]db.Job, error) {
		return q.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{AfterID: afterID, BatchSize: reindexBatchSize})
	}
}

func incrementalScan(q *db.Queries, since time.Time) pageFetcher {
	cutoff := pgtype.Timestamptz{Time: since, Valid: true}
	return func(ctx context.Context, afterID int64) ([]db.Job, error) {
		return q.ListJobsUpdatedAfter(ctx, db.ListJobsUpdatedAfterParams{
			AfterID: afterID, Since: cutoff, BatchSize: reindexBatchSize,
		})
	}
}

// progressInterval is how often reindex emits a heartbeat with its running totals.
// A full reindex pushes hundreds of thousands of docs to Meilisearch and otherwise
// logs only on completion, so the heartbeat distinguishes a slow run from a stalled
// one (the totals stop advancing).
const progressInterval = 60 * time.Second

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
	q := db.New(pool)

	ops := facetOps(client)
	if semanticRequested(os.Args[1:]) {
		ops = semanticOps(client)
	}

	fetch := fullScan(q)
	scope := "full"
	since, incremental, err := sinceFrom(os.Args[1:])
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	if incremental {
		fetch = incrementalScan(q, time.Now().Add(-since))
		scope = "since " + since.String()
	}
	log.Printf("reindex: target=%s scope=%s", ops.name, scope)

	indexed, deleted, err := reindexAll(ctx, fetch, ops)
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}

	log.Printf("reindex done: target=%s scope=%s indexed=%d deleted=%d", ops.name, scope, indexed, deleted)
	return 0
}

// sinceFrom parses an optional --since <duration> / --since=<duration> flag (e.g.
// "50h"). It reports (duration, true, nil) when present, (0, false, nil) when
// absent, and an error for a missing or unparseable value.
func sinceFrom(args []string) (time.Duration, bool, error) {
	for i, a := range args {
		var raw string
		switch {
		case a == "--since":
			if i+1 >= len(args) {
				return 0, false, fmt.Errorf("--since needs a duration (e.g. 50h)")
			}
			raw = args[i+1]
		case strings.HasPrefix(a, "--since="):
			raw = strings.TrimPrefix(a, "--since=")
		default:
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, false, fmt.Errorf("--since %q: %w", raw, err)
		}
		if d <= 0 {
			return 0, false, fmt.Errorf("--since must be positive, got %q", raw)
		}
		return d, true, nil
	}
	return 0, false, nil
}

// semanticRequested reports whether the args ask for the hybrid (embedder) pass.
func semanticRequested(args []string) bool {
	for _, a := range args {
		if a == "--semantic" || a == "semantic" {
			return true
		}
	}
	return false
}

// reindexAll ensures the index and streams jobs through it in batches, returning
// how many documents were indexed (open jobs) and deleted (closed jobs). fetch
// pages by keyset (id > last seen) — full or incremental (--since) — so rows
// inserted or re-ordered during the run cannot be skipped or repeated.
func reindexAll(ctx context.Context, fetch pageFetcher, ops indexOps) (int, int, error) {
	if err := ops.ensure(ctx); err != nil {
		return 0, 0, err
	}

	// Atomic so the heartbeat goroutine can read the running totals while the loop
	// advances them. Without the heartbeat a long reindex is silent until "done",
	// indistinguishable from a stalled push to Meilisearch.
	var indexed, deleted atomic.Int64
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("reindex: progress indexed=%d deleted=%d", indexed.Load(), deleted.Load())
	})
	defer stopHeartbeat()

	var afterID int64
	for {
		jobs, err := fetch(ctx, afterID)
		if err != nil {
			return int(indexed.Load()), int(deleted.Load()), err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		docs, deleteIDs, err := splitJobs(jobs)
		if err != nil {
			return int(indexed.Load()), int(deleted.Load()), err
		}
		if err := ops.index(ctx, docs); err != nil {
			return int(indexed.Load()), int(deleted.Load()), err
		}
		if err := ops.remove(ctx, deleteIDs); err != nil {
			return int(indexed.Load()), int(deleted.Load()), err
		}
		indexed.Add(int64(len(docs)))
		deleted.Add(int64(len(deleteIDs)))

		if len(jobs) < reindexBatchSize {
			break
		}
	}

	return int(indexed.Load()), int(deleted.Load()), nil
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
