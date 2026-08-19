// Command backfill-duplicate-marker-owner seeds the per-pass duplicate marker columns from the
// single jobs.duplicate_of that predates them, then exits.
//
// Migration 0114 gives each dedup pass its own column and 0115 derives duplicate_of from the
// three. Between those two this pass has to run: a derivation over three empty columns would clear
// every marker in the catalogue, so the columns must hold the current answer before the trigger
// exists.
//
// Provenance is not recoverable — a marked row records where it points, never which pass decided
// it — so the seed goes by shape, and fuzzy markers land in the role column. The first marker
// refresh after deploy sorts that out: the role recompute clears the ones that are not role
// clusters and the fuzzy pass re-sets them in its own column, one extra cycle of churn, once.
//
// Run it repeatedly and it costs nothing: a row that already has an owned column set no longer
// matches, so a re-run writes nothing and stopping mid-way is free. That property is also why
// there is no separate reconcile mode — running the pass again after the trigger lands picks up
// whatever was written while it was walking.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// defaultChunkSize is how many ids one UPDATE spans, overridable with BACKFILL_MARKER_CHUNK. It
// spans an ID RANGE, not a row count: prod's 7.4M rows are spread over ~1.6 billion ids because
// the sequence has run far ahead of the live row count through pruning, so most chunks cover empty
// stretches. See cmd/backfill-slug-folded, which measured that distinction into hours.
const defaultChunkSize = 50_000

func chunkSize() int64 {
	if v, err := strconv.ParseInt(os.Getenv("BACKFILL_MARKER_CHUNK"), 10, 64); err == nil && v > 0 {
		return v
	}
	return defaultChunkSize
}

// pauseBetweenChunks lets the host breathe between statements. This pass competes with the ingest
// and whatever reindex is running, and it is never urgent — until it completes, the markers it
// seeds are still the ones duplicate_of already holds.
const pauseBetweenChunks = 200 * time.Millisecond

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	bounds, err := q.DuplicateMarkerOwnerBackfillBounds(ctx)
	if err != nil {
		log.Printf("backfill-duplicate-marker-owner: bounds: %v", err)
		return 1
	}
	if bounds.Remaining == 0 {
		log.Print("backfill-duplicate-marker-owner: nothing to do")
		return 0
	}

	// The same provider set the suppression pass runs against, so a row seeded as the aggregator
	// pass's is one that pass would actually claim.
	aggregators := sources.AggregatorProviders(sources.Taxonomy())
	step := chunkSize()
	log.Printf("backfill-duplicate-marker-owner: %d rows to seed, ids %d..%d, chunk=%d (%d statements)",
		bounds.Remaining, bounds.MinID, bounds.MaxID, step, (bounds.MaxID-bounds.MinID)/step+1)

	var seeded int64
	lastLog := time.Now()
	for from := bounds.MinID; from <= bounds.MaxID; from += step {
		n, err := q.BackfillDuplicateMarkerOwnerChunk(ctx, db.BackfillDuplicateMarkerOwnerChunkParams{
			Aggregators: aggregators,
			FromID:      from,
			ToID:        from + step,
		})
		if err != nil {
			// Report what was already committed: every chunk is its own transaction, so the work
			// done so far survives and a re-run resumes from it.
			log.Printf("backfill-duplicate-marker-owner: chunk %d..%d after %d seeded: %v", from, from+step, seeded, err)
			return 1
		}
		seeded += n

		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-duplicate-marker-owner: progress seeded=%d at id=%d of %d", seeded, from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-duplicate-marker-owner: cancelled after %d seeded, resume by re-running", seeded)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}
	log.Printf("backfill-duplicate-marker-owner: done, seeded=%d", seeded)
	return 0
}
