// Command seed-adzuna-description-queue queues every existing, eligible Adzuna posting for a
// full-description fetch and exits. It is the one-off half of a deliberate split: cmd/ingest
// enqueues newly crawled Adzuna postings as they are written, which only ever reaches rows from
// here on — the ~21k Adzuna postings already in the catalogue before this feature shipped need a
// separate pass to queue. This command does only the queueing; cmd/hydrate-adzuna-description
// (already built for the ongoing case) does the actual fetch-and-store, so the two run paths
// share one Save/content_hash implementation rather than a backfill duplicating it.
//
// Run it once. It pages every source='adzuna' row by keyset and exits; run it again later (e.g.
// after adding a new Adzuna country board) and it costs nothing on rows already queued or already
// hydrated — EnqueueAdzunaDescriptionCapture's own gates handle both.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/adzunadesc"
	"github.com/strelov1/freehire/internal/platform/backfillpage"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// jobStore is the slice of the data layer this command needs: page adzuna's rows by keyset
// and enqueue one for a capture. *db.Queries satisfies it; the test uses a fake.
type jobStore interface {
	ListJobsBySourceAfter(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)
	EnqueueAdzunaDescriptionCapture(ctx context.Context, jobID int64) (int64, error)
}

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	scanned, queued, err := seedAll(ctx, db.New(pool))
	if err != nil {
		log.Printf("seed-adzuna-description-queue: %v", err)
		return 1
	}
	log.Printf("seed-adzuna-description-queue done: scanned=%d queued=%d", scanned, queued)
	return 0
}

// seedAll enqueues every source='adzuna' row adzunadesc.Eligible accepts. The keyset walk
// itself (id > last seen, so concurrent writes cannot skip or repeat rows) is shared with
// the sibling per-source passes via backfillpage.Rows.
func seedAll(ctx context.Context, store jobStore) (scanned, queued int, err error) {
	err = backfillpage.Rows(ctx, store.ListJobsBySourceAfter, "adzuna", func(j db.Job) error {
		scanned++
		if !adzunadesc.Eligible(j.Source, j.URL) {
			return nil
		}
		if _, err := store.EnqueueAdzunaDescriptionCapture(ctx, j.ID); err != nil {
			return err
		}
		queued++
		return nil
	})
	return scanned, queued, err
}
