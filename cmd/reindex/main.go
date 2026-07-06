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

// progressInterval is how often reindex emits a heartbeat with its running totals.
// A full reindex pushes hundreds of thousands of docs to Meilisearch and otherwise
// logs only on completion, so the heartbeat distinguishes a slow run from a stalled
// one (the totals stop advancing).
const progressInterval = 60 * time.Second

func main() {
	worker.Main(run)
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

	semantic := semanticRequested(os.Args[1:])
	target := "facet"
	if semantic {
		target = "semantic"
	}

	since, incremental, err := sinceFrom(os.Args[1:])
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}

	postedWithin, scoped, err := postedWithinFrom(os.Args[1:])
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	// --posted-within scopes the fresh window the semantic swap rebuild embeds; it is
	// meaningless for the facet index (which holds the whole catalogue) and for the
	// in-place --since delta (which cannot be swapped in). Reject those combos loudly
	// rather than silently ignoring the flag.
	if scoped && (!semantic || incremental) {
		log.Print("reindex: --posted-within applies only to a full --semantic rebuild")
		return 1
	}

	// --since is a delta into the LIVE index in place: a partial set cannot be
	// swapped in wholesale (it would drop everything else). A full pass instead
	// builds a fresh index and atomically swaps it over the live one, which keeps
	// merges cheap (the new index grows from empty) and never exposes a half-built
	// index to search.
	if incremental {
		ops := facetOps(client)
		if semantic {
			ops = semanticOps(client)
		}
		scope := "since " + since.String()
		log.Printf("reindex: target=%s scope=%s mode=in-place", target, scope)
		indexed, deleted, skipped, err := reindexAll(ctx, worker.NewIncrementalReader(q, time.Now().Add(-since)), ops)
		if err != nil {
			log.Printf("reindex: %v", err)
			return 1
		}
		log.Printf("reindex done: target=%s scope=%s indexed=%d deleted=%d skipped=%d", target, scope, indexed, deleted, skipped)
		return 0
	}

	var b rebuilder = client.NewFacetRebuild()
	if semantic {
		b = client.NewSemanticRebuild()
	}
	// The semantic rebuild optionally scopes to a fresh posting window (--posted-within);
	// every other full pass scans the whole table. A scoped reader returns open jobs only,
	// which the swap rebuild wants anyway (closed jobs are simply absent).
	reader := worker.NewFullScanReader(q)
	scope := "full"
	if scoped {
		reader = worker.NewPostedSinceReader(q, time.Now().Add(-postedWithin))
		scope = "posted-within " + postedWithin.String()
	}
	log.Printf("reindex: target=%s scope=%s mode=swap", target, scope)
	indexed, skipped, err := reindexFull(ctx, reader, b)
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	log.Printf("reindex done: target=%s scope=full indexed=%d skipped=%d", target, indexed, skipped)
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

// postedWithinFrom parses an optional --posted-within <duration> / --posted-within=<duration>
// flag (e.g. "168h" for 7 days). It scopes a full --semantic rebuild to jobs posted
// within that window, since the in-engine embedder cannot embed the whole catalogue in
// reasonable time. Reports (duration, true, nil) when present, (0, false, nil) when
// absent, and an error for a missing or unparseable value. Mirrors sinceFrom.
func postedWithinFrom(args []string) (time.Duration, bool, error) {
	for i, a := range args {
		var raw string
		switch {
		case a == "--posted-within":
			if i+1 >= len(args) {
				return 0, false, fmt.Errorf("--posted-within needs a duration (e.g. 168h)")
			}
			raw = args[i+1]
		case strings.HasPrefix(a, "--posted-within="):
			raw = strings.TrimPrefix(a, "--posted-within=")
		default:
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, false, fmt.Errorf("--posted-within %q: %w", raw, err)
		}
		if d <= 0 {
			return 0, false, fmt.Errorf("--posted-within must be positive, got %q", raw)
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
func reindexAll(ctx context.Context, reader worker.PageReader, ops indexOps) (int, int, int, error) {
	if err := ops.ensure(ctx); err != nil {
		return 0, 0, 0, err
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
	var skipped int
	for {
		jobs, lastID, corrupted, err := worker.ResilientPage(ctx, reader, afterID, reindexBatchSize)
		if err != nil {
			return int(indexed.Load()), int(deleted.Load()), skipped, err
		}
		skipped += len(corrupted)

		if len(jobs) > 0 {
			docs, deleteIDs, err := splitJobs(jobs)
			if err != nil {
				return int(indexed.Load()), int(deleted.Load()), skipped, err
			}
			if err := ops.index(ctx, docs); err != nil {
				return int(indexed.Load()), int(deleted.Load()), skipped, err
			}
			if err := ops.remove(ctx, deleteIDs); err != nil {
				return int(indexed.Load()), int(deleted.Load()), skipped, err
			}
			indexed.Add(int64(len(docs)))
			deleted.Add(int64(len(deleteIDs)))
		}

		// Keyset progress is the exhaustion signal (see reindexFull).
		if lastID == afterID {
			break
		}
		afterID = lastID
	}

	return int(indexed.Load()), int(deleted.Load()), skipped, nil
}

// rebuilder builds a brand-new index out of band and atomically swaps it into
// production. A full reindex uses it instead of mutating the live index in place:
// Prepare creates a fresh, empty rebuild index; Push streams document batches into
// it WITHOUT waiting per batch (so Meilisearch auto-batches them — the throughput
// lever); Promote waits for the pushes to finish, swaps the rebuild index over the
// live one in a single atomic step, and drops the old one. Search keeps serving the
// old index untouched until the swap, and merges stay cheap because the rebuild
// index grows from empty rather than re-merging into a full one.
type rebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.JobDocument) error
	Promote(ctx context.Context) error
}

// reindexFull rebuilds the index from scratch and swaps it in. It streams ONLY
// open jobs into the fresh index — closed jobs are simply absent (the rebuild
// index never held them, so unlike the in-place path there is nothing to delete).
// fetch pages by keyset (id > last seen) so rows inserted or re-ordered during the
// run cannot be skipped or repeated. Used for full passes; --since stays in place
// (reindexAll) since a partial index cannot be swapped in wholesale.
func reindexFull(ctx context.Context, reader worker.PageReader, b rebuilder) (int, int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, 0, err
	}

	var indexed atomic.Int64
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("reindex: progress indexed=%d", indexed.Load())
	})
	defer stopHeartbeat()

	var afterID int64
	var skipped int
	for {
		jobs, lastID, corrupted, err := worker.ResilientPage(ctx, reader, afterID, reindexBatchSize)
		if err != nil {
			return int(indexed.Load()), skipped, err
		}
		skipped += len(corrupted)

		if len(jobs) > 0 {
			docs, _, err := splitJobs(jobs) // closed jobs (deleteIDs) are dropped, not indexed
			if err != nil {
				return int(indexed.Load()), skipped, err
			}
			if err := b.Push(ctx, docs); err != nil {
				return int(indexed.Load()), skipped, err
			}
			indexed.Add(int64(len(docs)))
		}

		// Keyset progress is the exhaustion signal: ResilientPage advances lastID
		// past a skipped (corrupted) row, so a short page from the degrade path does
		// not end the scan early the way a "< batchSize" check would.
		if lastID == afterID {
			break
		}
		afterID = lastID
	}

	if err := b.Promote(ctx); err != nil {
		return int(indexed.Load()), skipped, err
	}
	return int(indexed.Load()), skipped, nil
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
