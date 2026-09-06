package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// failingDBTX fails every statement, which is the only seam a unit test has into the
// marker passes: they take a concrete *db.Queries.
type failingDBTX struct{ err error }

func (f failingDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}
func (f failingDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, f.err }
func (f failingDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return errRow{cause: f.err} }
func (f failingDBTX) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults  { return nil }

// errRow is the pgx.Row a failing QueryRow hands back. Its field is named apart from
// failingDBTX's so the two are not the same shape — nothing here is a conversion.
type errRow struct{ cause error }

func (r errRow) Scan(...any) error { return r.cause }

// The three passes are log-and-continue because a marker refresh failure must not stop
// the rebuild that follows it. REINDEX_DEDUP_ONLY has no rebuild to protect, so in that
// mode the passes ARE the run — and it short-circuited to `return 0` regardless, which
// made a run where all three failed indistinguishable from one with nothing to re-mark.
func TestRefreshDuplicateMarkersReportsHowManyPassesFailed(t *testing.T) {
	q := db.New(failingDBTX{err: errors.New("statement timeout")})

	failed := refreshDuplicateMarkers(context.Background(), q)

	if failed != 3 {
		t.Errorf("failed passes = %d, want 3", failed)
	}
	// The reason the count is worth having at all: it is what the marker-only mode
	// turns into an exit code cron can see.
	if got := worker.ExitCode(failed, 0); got != 1 {
		t.Errorf("exit code = %d, want 1 for a run that re-marked nothing it was asked to", got)
	}
}
