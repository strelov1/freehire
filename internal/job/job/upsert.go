package job

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Upsert is the one full write of a posting: the generated UpsertJob, then the observation of the
// posting's job_activity row, in the caller's transaction. They are two commands because the
// insert trigger creates the activity row at the end of the INSERT statement, where no CTE of
// that statement can see it; a later command can. A caller that stopped after UpsertJob would
// leave a new row's hydration NULL (read as "stale body") and an existing row's last_seen_at
// frozen, so a test walks the module and allows only this file to call it.
//
// The transaction is a pgx.Tx by type: both commands must commit together, and the jobs row is
// locked before the activity row (the one global lock order).
func Upsert(ctx context.Context, tx pgx.Tx, p db.UpsertJobParams) (db.UpsertJobRow, error) {
	row, err := db.New(tx).UpsertJob(ctx, p)
	if err != nil {
		return db.UpsertJobRow{}, err
	}
	// A new row always read its body; an existing one did too unless the pipeline substituted the
	// stored body for a failed fetch (nothing was read, so the hydration stamp stays).
	// ResetStrikes is the old row's state under the write's own lock (it was closed), so the
	// observation clears the strikes exactly when the old single statement did, an expired row
	// that stays closed included.
	if err := Observe(ctx, tx, row.ID, row.Inserted.Bool || !p.BodyFromCatalogue, row.ResetStrikes); err != nil {
		return db.UpsertJobRow{}, err
	}
	return row, nil
}

// Observe records that a crawl saw the posting now: last_seen_at moves forward, hydration moves
// only when the body was read, strikes clear only when asked. Every job has exactly one activity
// row, so any other count is a broken invariant, not a no-op.
func Observe(ctx context.Context, tx pgx.Tx, jobID int64, bodyRead, resetStrikes bool) error {
	n, err := db.New(tx).ObserveJobActivity(ctx, db.ObserveJobActivityParams{
		JobID:        jobID,
		BodyRead:     bodyRead,
		ResetStrikes: resetStrikes,
	})
	if err != nil {
		return fmt.Errorf("observe job %d: %w", jobID, err)
	}
	if n != 1 {
		return fmt.Errorf("observe job %d: %d activity rows, want exactly 1", jobID, n)
	}
	return nil
}
