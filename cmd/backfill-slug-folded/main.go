// Command backfill-slug-folded fills jobs.company_slug_folded for the rows that predate
// the column, then exits.
//
// The column duplicates `replace(company_slug, '-', ”)` as stored data so the
// aggregator-suppression pass can filter on something the planner can estimate — see
// migrations/0109 for the measurements (271s per batch against the expression, 491ms
// against a plain column). Every write path fills it going forward; this is the one-off
// pass for the 7.4M rows already in the table.
//
// Run it repeatedly and it costs nothing: each chunk's UPDATE is guarded by
// `IS DISTINCT FROM`, so rows already correct are not rewritten and produce no dead
// tuples. That is what makes it safe to stop and resume — including stopping it because
// the host is busy, which is the expected way to run it.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// chunkSize is how many ids one UPDATE spans. Sized so a single statement stays a short
// transaction on a 7.4M-row table: long transactions here would hold back autovacuum
// exactly while the pass is generating the dead rows it needs to clean.
//
// It spans an ID RANGE, not a row count — ids are sparse (pruning hard-deletes), so a
// chunk covers fewer rows than its width, never more.
const chunkSize = 50_000

// pauseBetweenChunks lets the host breathe between statements. The pass competes with
// the ingest and with whatever reindex is running, and it is never urgent — the
// suppression it feeds degrades to suppressing less, not to suppressing wrongly, until
// it completes.
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
	bounds, err := q.CompanySlugFoldedBackfillBounds(ctx)
	if err != nil {
		log.Printf("backfill-slug-folded: bounds: %v", err)
		return 1
	}
	if bounds.Remaining == 0 {
		log.Print("backfill-slug-folded: nothing to do")
		return 0
	}
	log.Printf("backfill-slug-folded: %d rows to fill, ids %d..%d", bounds.Remaining, bounds.MinID, bounds.MaxID)

	var filled int64
	lastLog := time.Now()
	for from := bounds.MinID; from <= bounds.MaxID; from += chunkSize {
		n, err := q.BackfillCompanySlugFoldedChunk(ctx, db.BackfillCompanySlugFoldedChunkParams{
			FromID: from,
			ToID:   from + chunkSize,
		})
		if err != nil {
			// Report what was already committed: every chunk is its own transaction, so
			// the work done so far survives and a re-run resumes from it.
			log.Printf("backfill-slug-folded: chunk %d..%d after %d filled: %v", from, from+chunkSize, filled, err)
			return 1
		}
		filled += n

		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-slug-folded: progress filled=%d at id=%d of %d", filled, from, bounds.MaxID)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-slug-folded: cancelled after %d filled, resume by re-running", filled)
			return 1
		case <-time.After(pauseBetweenChunks):
		}
	}
	log.Printf("backfill-slug-folded: done, filled=%d", filled)
	return 0
}
