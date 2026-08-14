package worker

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgerr"
)

// PageReader is the narrow slice of DB access ResilientPage needs: a wide keyset
// batch (the fast path), an id-only projection of the same window (the degrade
// path, which never detoasts so it cannot fault on corruption), and a single-row
// fetch to isolate the readable rows from the corrupted one. Build one with
// NewFullScanReader; tests supply a fake.
type PageReader interface {
	Batch(ctx context.Context, afterID int64, batchSize int32) ([]db.Job, error)
	IDs(ctx context.Context, afterID int64, batchSize int32) ([]int64, error)
	Row(ctx context.Context, id int64) (db.Job, error)
}

// FullScanQueries is the subset of *db.Queries a whole-table reader calls.
type FullScanQueries interface {
	ListJobsByIDAfter(context.Context, db.ListJobsByIDAfterParams) ([]db.Job, error)
	ListJobIDsAfter(context.Context, db.ListJobIDsAfterParams) ([]int64, error)
	GetJob(context.Context, int64) (db.Job, error)
}

type fullScanReader struct{ q FullScanQueries }

// NewFullScanReader adapts a job store to a PageReader over the whole jobs table
// (keyset by id).
func NewFullScanReader(q FullScanQueries) PageReader { return fullScanReader{q} }

func (r fullScanReader) Batch(ctx context.Context, afterID int64, bs int32) ([]db.Job, error) {
	return r.q.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{AfterID: afterID, BatchSize: bs})
}
func (r fullScanReader) IDs(ctx context.Context, afterID int64, bs int32) ([]int64, error) {
	return r.q.ListJobIDsAfter(ctx, db.ListJobIDsAfterParams{AfterID: afterID, BatchSize: bs})
}
func (r fullScanReader) Row(ctx context.Context, id int64) (db.Job, error) {
	return r.q.GetJob(ctx, id)
}

// ResilientPage reads one keyset page. Normally it returns the batch as-is. If the
// batch faults with a data-corruption error (XX001) — one row's TOAST is damaged,
// which fails the whole SELECT * — it degrades: it re-lists the same window as bare
// ids and fetches each row individually, collecting the readable ones and skipping
// (with a log line) any that still fault with XX001. Non-corruption errors always
// propagate unchanged.
//
// lastID is the keyset cursor for the next call. On the degrade path it advances to
// the last listed id — past the skipped row — so the scan never loops on it. When
// nothing was read (empty batch, or an empty degrade window), lastID equals the
// input afterID, which the caller reads as "no progress → exhausted".
func ResilientPage(ctx context.Context, r PageReader, afterID int64, batchSize int32) (rows []db.Job, lastID int64, skipped []int64, err error) {
	rows, err = r.Batch(ctx, afterID, batchSize)
	if err == nil {
		if len(rows) == 0 {
			return nil, afterID, nil, nil
		}
		return rows, rows[len(rows)-1].ID, nil, nil
	}
	if !pgerr.IsDataCorrupted(err) {
		return nil, 0, nil, err
	}

	ids, idErr := r.IDs(ctx, afterID, batchSize)
	if idErr != nil {
		return nil, 0, nil, idErr
	}
	if len(ids) == 0 {
		return nil, afterID, nil, nil
	}

	rows = make([]db.Job, 0, len(ids))
	for _, id := range ids {
		job, rowErr := r.Row(ctx, id)
		if rowErr != nil {
			if pgerr.IsDataCorrupted(rowErr) {
				skipped = append(skipped, id)
				log.Printf("resilient scan: skipping corrupted row id=%d: %v", id, rowErr)
				continue
			}
			// The row vanished between the id-list and this fetch (a concurrent
			// close/delete). The fast keyset SELECT would simply omit it, so the
			// degrade path does too — stay symmetric rather than aborting the scan.
			if errors.Is(rowErr, pgx.ErrNoRows) {
				continue
			}
			return nil, 0, nil, rowErr
		}
		rows = append(rows, job)
	}
	return rows, ids[len(ids)-1], skipped, nil
}
