package handler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/db"
)

// failingDBTX answers every read with the same error, which is what lets the ghost
// signal's degrade path be exercised without a database: *db.Queries is a concrete type
// built over this interface, so this is the only seam a unit test has into it.
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

// captureLog collects what fn writes to the standard logger.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	fn()
	return buf.String()
}

// Losing these two reads takes the whole outcome tier out of the ghost verdict —
// LevelLikely needs evidence, so it becomes unreachable — and there is no QueryTracer in
// internal/platform/db, so a swallowed error left no trace anywhere: the badge simply
// stopped appearing while every page went on answering 200.
func TestGhostEvidenceFor_SaysSoWhenTheLookupFails(t *testing.T) {
	q := db.New(failingDBTX{err: errors.New("connection reset by peer")})

	var evidence int
	out := captureLog(t, func() {
		evidence = len(ghostEvidenceFor(context.Background(), q, []int64{1, 2, 3}))
	})

	if evidence != 0 {
		t.Errorf("evidence entries = %d, want none — the read failed", evidence)
	}
	if !strings.Contains(out, "connection reset by peer") {
		t.Errorf("nothing named the failure, so it is invisible everywhere:\n%q", out)
	}
	if !strings.Contains(out, "ghost") {
		t.Errorf("the log line does not name the signal it degraded:\n%q", out)
	}
}

// A reader who navigates away cancels the request and every in-flight query fails with
// it. That is the commonest event on the site, so it must not be logged as a fault —
// hence the searchdrain/embed idiom: classify on the CONTEXT, not on the error, since a
// driver's own error type need not unwrap to a context sentinel at all.
func TestGhostEvidenceFor_TellsCancellationApartFromAFault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := db.New(failingDBTX{err: errors.New("connection reset by peer")})

	out := captureLog(t, func() { ghostEvidenceFor(ctx, q, []int64{1, 2, 3}) })

	if strings.Contains(out, "failed") {
		t.Errorf("an abandoned request was logged as a fault:\n%q", out)
	}
	if !strings.Contains(out, "abandoned") {
		t.Errorf("the abandoned read was not reported at all:\n%q", out)
	}
}
