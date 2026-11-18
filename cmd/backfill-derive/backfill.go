package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/ingest/rederive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// backfillRun is what one bounded pass did, and where the next one starts.
type backfillRun struct {
	Scanned, Updated, SlugsMoved int
	// MovedFrom and MovedTo are the distinct company slugs the run moved jobs away from and
	// onto, sorted: the scope of the companies reconcile. SlugsMoved counts ROWS.
	MovedFrom, MovedTo []string
	// Aliases are the moves that earned a redirect (see movedSlugs.add).
	Aliases []slugAlias
	// ResumeID is the id a follow-up run must be given as fromID: the first row this one did
	// not finish. Zero means the pass reached the end with nothing outstanding.
	ResumeID int64
}

// backfillPass re-derives every job's facets, fingerprint and slugs within win and rewrites
// the rows whose derived values differ. A single reader pages by keyset (id > last seen) so
// concurrent writes cannot skip or repeat rows, and `concurrency` workers derive and write in
// parallel. It reports rows written (Updated), rows that moved a slug (SlugsMoved) and where a
// follow-up run must start. The first store error cancels the run and is returned.
//
// This is backfillWindow with the production progress log attached; win may be empty, meaning
// the whole table.
func backfillPass(ctx context.Context, store deriveStore, concurrency int64, win scanWindow) (backfillRun, error) {
	start := time.Now()
	return backfillWindow(ctx, store, concurrency, win, progressEvery, func(scanned, updated, slugs int64) {
		log.Printf("backfill-derive: scanned %d, updated %d, slugs_moved %d, %s elapsed",
			scanned, updated, slugs, time.Since(start).Round(time.Second))
	})
}

// loadAliasRegistry reads company_slug_aliases into a folded-key lookup.
//
// A backfill that could not read it must FAIL rather than proceed: an empty registry looks like
// a catalogue with no merges, and proceeding would rewrite every merged posting back to its
// source spelling.
func loadAliasRegistry(ctx context.Context, store deriveStore) (map[string]string, error) {
	rows, err := store.ListCompanySlugAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("load company slug aliases: %w", err)
	}
	canon := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, seen := canon[r.FoldedKey]; !seen {
			canon[r.FoldedKey] = r.CanonicalSlug
		}
	}
	log.Printf("backfill-derive: company alias registry loaded (%d folded keys)", len(canon))
	return canon, nil
}

// backfillWindow is backfillPass with the reporting cadence and sink injected. report fires on
// each multiple of every, from whichever worker crosses it (the counter is atomic, so exactly
// one goroutine observes each); it runs on the worker's goroutine, so it must stay cheap.
func backfillWindow(ctx context.Context, store deriveStore, concurrency int64, win scanWindow, every int64, report func(scanned, updated, slugsMoved int64)) (backfillRun, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	// Loaded ONCE per run, before any worker starts, so every row resolves against the same
	// registry.
	canon, err := loadAliasRegistry(ctx, store)
	if err != nil {
		return backfillRun{}, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var scannedN, updatedN, slugsN int64
	moved := newMovedSlugs()
	var errOnce sync.Once
	var runErr error
	fail := func(e error) {
		errOnce.Do(func() { runErr = e; cancel() })
	}

	jobsCh := make(chan db.Job, backfillBatchSize)

	// Reader (producer): pages the table by keyset and feeds the channel.
	//
	// It reads through worker.ResilientPage because this pass has no resume point: a row with
	// damaged TOAST fails the whole wide SELECT, and an aborting scan would re-fail at the same
	// id on every run. The helper degrades to row-by-row reads and skips only what is unreadable.
	// A bounded window reads only what can still surface; the zero cutoff keeps the whole-table
	// reader. Both satisfy PageReader, so the degrade path is identical.
	reader := worker.NewFullScanReader(store)
	if !win.closedSince.IsZero() {
		reader = worker.NewLiveScanReader(store, win.closedSince)
	}
	// Where the reader's own cursor was when it gave up; written by the reader goroutine alone
	// and read after readerWG.Wait(). Zero means it reached the end. It is a CEILING on the
	// resume point; what the run actually finished is what pending knows.
	var stoppedAt int64
	pending := newPendingRows()
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		defer close(jobsCh)
		// Owned by this goroutine alone. Reported once at the end; each skipped id is already
		// logged where it was met.
		var skipped int
		defer func() {
			if skipped > 0 {
				log.Printf("backfill-derive: skipped %d corrupted row(s); their derived columns are unchanged", skipped)
			}
		}()
		// The reader's keyset is strictly-greater while fromID names the first id to DO, so the
		// cursor starts one below it. fromID 0 means "from the beginning".
		afterID := win.fromID - 1
		if win.fromID == 0 {
			afterID = 0
		}
		var fed int64
		for {
			// Narrow the last page to what is left of the budget, so a run stops AT maxRows
			// rather than up to one batch past it.
			batch := int32(backfillBatchSize)
			if win.maxRows > 0 {
				left := win.maxRows - fed
				if left <= 0 {
					// Budget spent. If it landed exactly on the last row the table IS fully
					// derived, and reporting otherwise sends an operator back for an empty
					// pass; one indexed row settles which it was.
					rest, _, _, e := worker.ResilientPage(ctx, reader, afterID, 1)
					if e != nil {
						fail(e)
						return
					}
					if len(rest) > 0 {
						stoppedAt = afterID
					}
					return
				}
				if left < int64(batch) {
					batch = int32(left) //nolint:gosec // left < batch, an int32
				}
			}

			jobs, lastID, corrupted, e := worker.ResilientPage(ctx, reader, afterID, batch)
			if e != nil {
				stoppedAt = afterID
				fail(e)
				return
			}
			skipped += len(corrupted)
			for i := range jobs {
				// Registered BEFORE the hand-over, so a row in the channel's buffer counts as
				// unfinished; registering in the worker would hide the buffered backlog.
				pending.add(jobs[i].ID)
				select {
				case jobsCh <- jobs[i]:
				case <-ctx.Done():
					stoppedAt = afterID
					return
				}
			}
			fed += int64(len(jobs))
			// Keyset progress is the exhaustion signal. A "< batchSize" test would end the scan
			// at the first corrupted row, since the degrade path returns a short page.
			if lastID == afterID {
				return
			}
			afterID = lastID
		}
	}()

	// Workers (consumers): derive and write in parallel.
	var workerWG sync.WaitGroup
	for i := int64(0); i < concurrency; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for j := range jobsCh {
				n := atomic.AddInt64(&scannedN, 1)
				if every > 0 && n%every == 0 && report != nil {
					report(n, atomic.LoadInt64(&updatedN), atomic.LoadInt64(&slugsN))
				}
				params, changed, slugMoved := rederive.Row(j, canon)
				if !changed {
					// Finished: no write needed, and holding it pending would drag the resume
					// point back over work that is done.
					pending.done(j.ID)
					continue
				}
				if e := store.UpdateJobDerived(ctx, params); e != nil {
					// Deliberately NOT marked done: this row is why the run is stopping, so
					// the resume point must land before it.
					fail(e)
					return
				}
				pending.done(j.ID)
				atomic.AddInt64(&updatedN, 1)
				if slugMoved {
					atomic.AddInt64(&slugsN, 1)
					moved.add(j.CompanySlug, params.CompanySlug, j.Company)
				}
			}
		}()
	}

	workerWG.Wait()
	readerWG.Wait()

	// Read only here, after the reader returned and the pool joined, so no further
	// synchronisation is needed. The oldest unfinished row wins over the reader's cursor: a run
	// that stopped cleanly at the end can still hold pending rows (the pool abandons its backlog
	// when a write fails), and reporting `done` would leave them stale for good.
	resumeID := stoppedAt + 1
	if stoppedAt == 0 {
		resumeID = 0
	}
	if oldest, ok := pending.oldest(); ok && (resumeID == 0 || oldest < resumeID) {
		resumeID = oldest
	}
	return backfillRun{
		Scanned:    int(scannedN),
		Updated:    int(updatedN),
		SlugsMoved: int(slugsN),
		MovedFrom:  moved.sorted(moved.from),
		MovedTo:    moved.sorted(moved.to),
		Aliases:    moved.qualifiedAliases(),
		ResumeID:   resumeID,
	}, runErr
}
