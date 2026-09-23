package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// PageReader is the narrow slice of DB access ResilientPage needs: a wide keyset
// batch (the fast path), an id-only projection of the same window (the degrade
// path, which never detoasts so it cannot fault on corruption), and a single-row
// fetch to isolate the readable rows from the corrupted one. Build one with
// NewFullScanReader (the whole table) or NewLiveScanReader (only the rows that can
// still reach the catalogue); tests supply a fake.
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

// LiveScanQueries is the subset a reader over the rows that can still reach the
// catalogue calls — the same three reads as FullScanQueries, narrowed by a cutoff.
type LiveScanQueries interface {
	ListLiveJobsByIDAfter(context.Context, db.ListLiveJobsByIDAfterParams) ([]db.Job, error)
	ListLiveJobIDsAfter(context.Context, db.ListLiveJobIDsAfterParams) ([]int64, error)
	GetJob(context.Context, int64) (db.Job, error)
}

type liveScanReader struct {
	q           LiveScanQueries
	closedSince time.Time
}

// NewLiveScanReader adapts a job store to a PageReader over the rows that can still
// surface: open postings, plus ones closed at or after closedSince.
//
// The cutoff is a parameter rather than a constant because the right value depends on
// what is being re-derived, and the caller is the one that knows. A closed posting is
// not permanently out of reach — ingest reopens it without rewriting its facets (see
// the ListLiveJobsByIDAfter comment) — so a reader that stopped at `closed_at IS NULL`
// would leave stale rows to drift back into the catalogue unannounced.
func NewLiveScanReader(q LiveScanQueries, closedSince time.Time) PageReader {
	return liveScanReader{q: q, closedSince: closedSince}
}

func (r liveScanReader) Batch(ctx context.Context, afterID int64, bs int32) ([]db.Job, error) {
	return r.q.ListLiveJobsByIDAfter(ctx, db.ListLiveJobsByIDAfterParams{
		AfterID: afterID, BatchSize: bs, ClosedSince: pgtype.Timestamptz{Time: r.closedSince, Valid: true},
	})
}
func (r liveScanReader) IDs(ctx context.Context, afterID int64, bs int32) ([]int64, error) {
	return r.q.ListLiveJobIDsAfter(ctx, db.ListLiveJobIDsAfterParams{
		AfterID: afterID, BatchSize: bs, ClosedSince: pgtype.Timestamptz{Time: r.closedSince, Valid: true},
	})
}
func (r liveScanReader) Row(ctx context.Context, id int64) (db.Job, error) {
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
