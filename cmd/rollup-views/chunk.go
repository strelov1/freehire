package main

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// applyChunkRows is how many (day, job) rows one transaction applies. Each row bumps
// jobs.view_count, and that UPDATE holds the jobs row lock until commit, blocking ingest's
// TouchJob / RefreshUnchangedJob / UpsertJob; one transaction for a whole file stalled the
// crawl fleet. A var only so tests can shrink it.
var applyChunkRows = 1000

// touchedJobs collects every job whose view_count a committed chunk moved, for the index
// write at the end of the run (pushViewCounts). Package state is fine for a run-once worker.
var touchedJobs = map[int64]struct{}{}

// errAlreadyApplied reports a file another run has applied, or is applying.
var errAlreadyApplied = errors.New("view log file already applied")

// viewKey is the order a file's rows are applied in and the cursor remembering how far a
// file has got: ascending job id, then day (see buildParams for why job id first).
type viewKey struct {
	jobID int64
	day   pgtype.Date
}

func (k viewKey) after(o viewKey) bool {
	if k.jobID != o.jobID {
		return k.jobID > o.jobID
	}
	return k.day.Time.After(o.day.Time)
}

// applyChunked applies one file's rows, already sorted by key, in transactions of
// applyChunkRows, then marks the file processed. It returns the views THIS run applied.
//
// The counts are additive, so partial commits are safe only because each chunk moves the
// file's cursor (view_log_cursors) in its own transaction: a failed run's re-run resumes
// after the last committed row. The move is a compare-and-swap against the key this run
// read, so a concurrent run loses its first chunk and stops with errAlreadyApplied. Each
// chunk retries on deadlock by itself.
func applyChunked[P any](
	ctx context.Context, pool *pgxpool.Pool, q *db.Queries, sig int64, name string,
	params []P, key func(P) viewKey, views func(P) int,
	exec func(context.Context, *db.Queries, []P) error,
) (int, error) {
	var prev *viewKey
	switch cur, err := q.GetViewLogCursor(ctx, sig); {
	case err == nil:
		prev = &viewKey{jobID: cur.JobID, day: cur.Day}
		params = params[sort.Search(len(params), func(i int) bool { return key(params[i]).after(*prev) }):]
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, err
	}

	applied := 0
	for len(params) > 0 {
		chunk := params[:min(applyChunkRows, len(params))]
		last := key(chunk[len(chunk)-1])
		err := applyWithRetry(ctx, name, func() error {
			return inTx(ctx, pool, q, func(qtx *db.Queries) error {
				p := db.AdvanceViewLogCursorParams{Signature: sig, Filename: name, JobID: last.jobID, Day: last.day}
				if prev != nil {
					p.PrevJobID = pgtype.Int8{Int64: prev.jobID, Valid: true}
					p.PrevDay = prev.day
				}
				moved, err := qtx.AdvanceViewLogCursor(ctx, p)
				if err != nil {
					return err
				}
				if moved == 0 {
					return errAlreadyApplied
				}
				return exec(ctx, qtx, chunk)
			})
		})
		if err != nil {
			return applied, err
		}
		for _, p := range chunk {
			applied += views(p)
			touchedJobs[key(p).jobID] = struct{}{}
		}
		prev = &last
		params = params[len(chunk):]
	}

	claimed, err := q.MarkViewLogFileProcessed(ctx, db.MarkViewLogFileProcessedParams{Signature: sig, Filename: name})
	if err != nil {
		return applied, err
	}
	if claimed == 0 && applied == 0 {
		return 0, errAlreadyApplied
	}
	return applied, nil
}

// inTx runs fn in one transaction and commits it.
func inTx(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, fn func(*db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// execBatch drains a :batchexec result, returning its first error.
func execBatch(br interface {
	Exec(func(int, error))
	Close() error
}) error {
	var batchErr error
	br.Exec(func(_ int, e error) {
		if e != nil && batchErr == nil {
			batchErr = e
		}
	})
	if cerr := br.Close(); cerr != nil && batchErr == nil {
		batchErr = cerr
	}
	return batchErr
}
