package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/db"
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
}

// The count above is only worth having because the marker-only mode turns it into an exit
// code cron can see. Asserting worker.ExitCode(failed, 0) beside the count would re-derive
// the production line in the test rather than check it — and it did, which is why reverting
// the mode to `refreshDuplicateMarkers(ctx, q); return 0` left this package green. This one
// goes through the seam the mode actually runs.
func TestDedupOnlyExitsNonZeroWhenThePassesFailed(t *testing.T) {
	if got := dedupOnlyExit(context.Background(), db.New(failingDBTX{err: errors.New("statement timeout")})); got != 1 {
		t.Errorf("exit code = %d, want 1 for a run that re-marked nothing it was asked to", got)
	}
}
