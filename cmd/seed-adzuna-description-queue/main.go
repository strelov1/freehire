// Command seed-adzuna-description-queue queues every existing, eligible Adzuna posting for a
// full-description fetch and exits. It is the one-off half of a split: cmd/ingest enqueues
// newly crawled Adzuna postings as they are written, so rows already in the catalogue need
// this separate pass. It only queues; cmd/hydrate-adzuna-description does the fetch and
// store, so both paths share one Save/content_hash implementation.
//
// Run it once. It pages every source='adzuna' row by keyset and exits; running it again
// costs nothing on rows already queued or hydrated (EnqueueAdzunaDescriptionCapture's own
// gates handle both).
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/adzunadesc"
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

// batchSize bounds how many rows are read per keyset page.
const batchSize = 500

// seedAll enqueues every source='adzuna' row adzunadesc.Eligible accepts. It walks the
// source by keyset (id > last seen, so concurrent writes cannot skip or repeat rows),
// stopping once a page comes back short of the batch size.
func seedAll(ctx context.Context, store jobStore) (scanned, queued int, err error) {
	var afterID int64
	for {
		jobs, err := store.ListJobsBySourceAfter(ctx, db.ListJobsBySourceAfterParams{
			Source:    "adzuna",
			AfterID:   afterID,
			BatchSize: batchSize,
		})
		if err != nil {
			return scanned, queued, err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		for _, j := range jobs {
			scanned++
			if !adzunadesc.Eligible(j.Source, j.URL) {
				continue
			}
			if _, err := store.EnqueueAdzunaDescriptionCapture(ctx, j.ID); err != nil {
				return scanned, queued, err
			}
			queued++
		}

		if len(jobs) < batchSize {
			break
		}
	}
	return scanned, queued, nil
}
