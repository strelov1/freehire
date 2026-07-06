package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// corruptDataSQLState is Postgres SQLSTATE XX001 (data_corrupted), raised when a
// row cannot be read because its on-disk storage is damaged — most visibly a
// "missing chunk number N for toast value ..." on a broken TOAST pointer.
const corruptDataSQLState = "XX001"

// IsCorruptedRow reports whether err is (or wraps) a Postgres data-corruption
// error. It is deliberately narrow: only XX001 opts a read into the skip path, so
// every other failure still surfaces to the caller unchanged.
func IsCorruptedRow(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == corruptDataSQLState
}

// PageReader is the narrow slice of DB access ResilientPage needs: a wide keyset
// batch (the fast path), an id-only projection of the same window (the degrade
// path, which never detoasts so it cannot fault on corruption), and a single-row
// fetch to isolate the readable rows from the corrupted one. Build one with
// NewFullScanReader / NewIncrementalReader; tests supply a fake.
type PageReader interface {
	Batch(ctx context.Context, afterID int64, batchSize int32) ([]db.Job, error)
	IDs(ctx context.Context, afterID int64, batchSize int32) ([]int64, error)
	Row(ctx context.Context, id int64) (db.Job, error)
}

// jobQueries is the subset of *db.Queries the readers call — narrowed to keep the
// readers testable and their dependency explicit.
type jobQueries interface {
	ListJobsByIDAfter(context.Context, db.ListJobsByIDAfterParams) ([]db.Job, error)
	ListJobIDsAfter(context.Context, db.ListJobIDsAfterParams) ([]int64, error)
	ListJobsUpdatedAfter(context.Context, db.ListJobsUpdatedAfterParams) ([]db.Job, error)
	ListJobIDsUpdatedAfter(context.Context, db.ListJobIDsUpdatedAfterParams) ([]int64, error)
	ListOpenJobsPostedAfter(context.Context, db.ListOpenJobsPostedAfterParams) ([]db.Job, error)
	ListOpenJobIDsPostedAfter(context.Context, db.ListOpenJobIDsPostedAfterParams) ([]int64, error)
	GetJob(context.Context, int64) (db.Job, error)
}

type fullScanReader struct{ q jobQueries }

// NewFullScanReader adapts *db.Queries to a PageReader over the whole jobs table
// (keyset by id).
func NewFullScanReader(q jobQueries) PageReader { return fullScanReader{q} }

func (r fullScanReader) Batch(ctx context.Context, afterID int64, bs int32) ([]db.Job, error) {
	return r.q.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{AfterID: afterID, BatchSize: bs})
}
func (r fullScanReader) IDs(ctx context.Context, afterID int64, bs int32) ([]int64, error) {
	return r.q.ListJobIDsAfter(ctx, db.ListJobIDsAfterParams{AfterID: afterID, BatchSize: bs})
}
func (r fullScanReader) Row(ctx context.Context, id int64) (db.Job, error) {
	return r.q.GetJob(ctx, id)
}

type incrementalReader struct {
	q     jobQueries
	since pgtype.Timestamptz
}

// NewIncrementalReader adapts *db.Queries to a PageReader over jobs changed at or
// after since (the `reindex --since` window), keyset by id.
func NewIncrementalReader(q jobQueries, since time.Time) PageReader {
	return incrementalReader{q: q, since: pgtype.Timestamptz{Time: since, Valid: true}}
}

func (r incrementalReader) Batch(ctx context.Context, afterID int64, bs int32) ([]db.Job, error) {
	return r.q.ListJobsUpdatedAfter(ctx, db.ListJobsUpdatedAfterParams{AfterID: afterID, Since: r.since, BatchSize: bs})
}
func (r incrementalReader) IDs(ctx context.Context, afterID int64, bs int32) ([]int64, error) {
	return r.q.ListJobIDsUpdatedAfter(ctx, db.ListJobIDsUpdatedAfterParams{AfterID: afterID, Since: r.since, BatchSize: bs})
}
func (r incrementalReader) Row(ctx context.Context, id int64) (db.Job, error) {
	return r.q.GetJob(ctx, id)
}

type postedSinceReader struct {
	q     jobQueries
	since pgtype.Timestamptz
}

// NewPostedSinceReader adapts *db.Queries to a PageReader over OPEN jobs whose
// effective posting date (COALESCE(posted_at, created_at)) is at or after since —
// the freshness window `reindex --semantic --posted-within` embeds. Keyset by id.
// Unlike the incremental reader it returns open jobs only, since the semantic swap
// rebuild it feeds never holds closed jobs (nothing to delete).
func NewPostedSinceReader(q jobQueries, since time.Time) PageReader {
	return postedSinceReader{q: q, since: pgtype.Timestamptz{Time: since, Valid: true}}
}

func (r postedSinceReader) Batch(ctx context.Context, afterID int64, bs int32) ([]db.Job, error) {
	return r.q.ListOpenJobsPostedAfter(ctx, db.ListOpenJobsPostedAfterParams{AfterID: afterID, PostedSince: r.since, BatchSize: bs})
}
func (r postedSinceReader) IDs(ctx context.Context, afterID int64, bs int32) ([]int64, error) {
	return r.q.ListOpenJobIDsPostedAfter(ctx, db.ListOpenJobIDsPostedAfterParams{AfterID: afterID, PostedSince: r.since, BatchSize: bs})
}
func (r postedSinceReader) Row(ctx context.Context, id int64) (db.Job, error) {
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
	if !IsCorruptedRow(err) {
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
			if IsCorruptedRow(rowErr) {
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
