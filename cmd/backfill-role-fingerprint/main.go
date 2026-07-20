// Command backfill-role-fingerprint recomputes jobs.role_fingerprint for the live
// catalogue using the current internal/jobhash.RoleFingerprint. role_fingerprint is
// otherwise written only at ingest (UpsertJob), so a change to the fingerprint's title
// normalization — e.g. stripping a trailing city clause so per-city variants of one role
// cluster — reaches existing rows only as they re-crawl. This one-shot applies it to the
// whole table at once; follow it with a reindex (make reindex), whose duplicate_of
// recompute then collapses the newly-clustered reposts and unions their geography onto
// each canon.
//
// It mirrors backfill-derive: one reader pages the table by keyset and feeds a channel,
// N workers recompute and write in parallel. The UpdateJobRoleFingerprint guard writes
// only rows whose fingerprint actually moved, so re-runs are cheap and idempotent.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/worker"
)

// backfillBatchSize bounds how many jobs are read per keyset page.
const backfillBatchSize = 500

// fingerprintStore is the slice of the data layer the backfill needs: page the table by
// keyset and rewrite a row's role_fingerprint. *db.Queries satisfies it; tests use a
// fake. UpdateJobRoleFingerprint is called concurrently by the worker pool, and pgxpool
// hands each goroutine its own connection, so the store must be safe for concurrent use.
type fingerprintStore interface {
	ListJobsByIDAfter(ctx context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error)
	UpdateJobRoleFingerprint(ctx context.Context, arg db.UpdateJobRoleFingerprintParams) (int64, error)
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

	concurrency := backfillConcurrency()
	log.Printf("backfill-role-fingerprint starting: concurrency=%d", concurrency)
	scanned, updated, err := backfillAll(ctx, db.New(pool), concurrency)
	if err != nil {
		log.Printf("backfill-role-fingerprint: %v", err)
		return 1
	}
	log.Printf("backfill-role-fingerprint done: scanned=%d updated=%d (follow with a reindex)", scanned, updated)
	return 0
}

// backfillConcurrency reads the worker-pool size from BACKFILL_CONCURRENCY, defaulting
// to 1 for any unset/invalid value.
func backfillConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("BACKFILL_CONCURRENCY")); err == nil && n > 0 {
		return n
	}
	return 1
}

// fingerprintUpdate recomputes a job's role_fingerprint and reports whether it differs
// from what is stored (i.e. a write is needed). Only the role-identity fields feed the
// hash (see jobhash.RoleFingerprint), so a minimal params value is enough. Pure — safe
// to call concurrently.
func fingerprintUpdate(j db.Job) (db.UpdateJobRoleFingerprintParams, bool) {
	fp := jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: j.CompanySlug,
		Title:       j.Title,
		Description: j.Description,
	})
	changed := fp != j.RoleFingerprint.String
	return db.UpdateJobRoleFingerprintParams{
		ID:              j.ID,
		RoleFingerprint: pgtype.Text{String: fp, Valid: true},
	}, changed
}

// backfillAll recomputes every job's role_fingerprint and rewrites the rows whose
// fingerprint moved. A single reader pages the table by keyset (id > last seen) so
// concurrent writes cannot skip or repeat rows, and a pool of `concurrency` workers
// recomputes and writes in parallel (order-independent). The first store error cancels
// the run and is returned.
func backfillAll(ctx context.Context, store fingerprintStore, concurrency int) (scanned, updated int, err error) {
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var scannedN, updatedN int64
	var errOnce sync.Once
	var runErr error
	fail := func(e error) {
		errOnce.Do(func() { runErr = e; cancel() })
	}

	jobsCh := make(chan db.Job, backfillBatchSize)

	// Reader (producer): pages the table by keyset and feeds the channel.
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		defer close(jobsCh)
		var afterID int64
		for {
			jobs, e := store.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
				AfterID:   afterID,
				BatchSize: backfillBatchSize,
			})
			if e != nil {
				fail(e)
				return
			}
			if len(jobs) == 0 {
				return
			}
			afterID = jobs[len(jobs)-1].ID
			for i := range jobs {
				select {
				case jobsCh <- jobs[i]:
				case <-ctx.Done():
					return
				}
			}
			if len(jobs) < backfillBatchSize {
				return
			}
		}
	}()

	// Workers (consumers): recompute + write in parallel.
	var workerWG sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for j := range jobsCh {
				atomic.AddInt64(&scannedN, 1)
				params, changed := fingerprintUpdate(j)
				if !changed {
					continue
				}
				if _, e := store.UpdateJobRoleFingerprint(ctx, params); e != nil {
					fail(e)
					return
				}
				atomic.AddInt64(&updatedN, 1)
			}
		}()
	}

	workerWG.Wait()
	readerWG.Wait()
	return int(scannedN), int(updatedN), runErr
}
