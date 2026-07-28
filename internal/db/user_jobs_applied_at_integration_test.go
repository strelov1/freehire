//go:build integration

// Integration tests for the dated-apply SQL semantics: an application recorded
// from a piece of mail carries that mail's timestamp instead of now(), never
// rewrites an earlier application's date, and still bumps the job's
// applied_count exactly once. This is SQL behavior (a CTE guarded by the
// pre-upsert applied_at) and can only be verified against a real Postgres. Run
// with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestMarkJobAppliedAtUsesTheSuppliedDate asserts a dated recording carries the
// supplied timestamp rather than now(), so an application recorded from
// three-week-old mail does not read as applied today.
func TestMarkJobAppliedAtUsesTheSuppliedDate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "datedapply@example.test")
	jid := insertJob(t, pool, "dated-1")

	when := time.Now().Add(-21 * 24 * time.Hour).UTC().Truncate(time.Second)
	row, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid,
		At: pgtype.Timestamptz{Time: when, Valid: true},
	})
	if err != nil {
		t.Fatalf("MarkJobApplied dated: %v", err)
	}
	if !row.AppliedAt.Valid {
		t.Fatal("applied_at is NULL, want the supplied date")
	}
	if got := row.AppliedAt.Time.UTC().Truncate(time.Second); !got.Equal(when) {
		t.Errorf("applied_at = %v, want the supplied %v", got, when)
	}
	if row.Stage.String != "applied" {
		t.Errorf("stage = %q, want applied (seeded as for any application)", row.Stage.String)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT applied_count FROM jobs WHERE id = $1`, jid).Scan(&count); err != nil {
		t.Fatalf("read applied_count: %v", err)
	}
	if count != 1 {
		t.Errorf("applied_count = %d, want 1", count)
	}
}

// TestMarkJobAppliedAtKeepsAnEarlierDate asserts a later dated recording neither
// rewrites the stored date nor bumps the counter a second time — recording the
// same application twice is one application.
func TestMarkJobAppliedAtKeepsAnEarlierDate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "datedkeep@example.test")
	jid := insertJob(t, pool, "dated-2")

	first := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: first, Valid: true},
	}); err != nil {
		t.Fatalf("first dated apply: %v", err)
	}

	later := time.Now().Add(-2 * 24 * time.Hour).UTC().Truncate(time.Second)
	row, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: later, Valid: true},
	})
	if err != nil {
		t.Fatalf("second dated apply: %v", err)
	}
	if got := row.AppliedAt.Time.UTC().Truncate(time.Second); !got.Equal(first) {
		t.Errorf("applied_at = %v, want the original %v (a later recording must not rewrite it)", got, first)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT applied_count FROM jobs WHERE id = $1`, jid).Scan(&count); err != nil {
		t.Fatalf("read applied_count: %v", err)
	}
	if count != 1 {
		t.Errorf("applied_count = %d, want 1 (a repeat recording must not double-count)", count)
	}
}

// TestMarkJobAppliedUndatedIsUnchanged pins the ordinary apply path: with no
// supplied date it still stamps now(), so adding the dated variant did not
// quietly change what the Apply button does.
func TestMarkJobAppliedUndatedIsUnchanged(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "undatedapply@example.test")
	jid := insertJob(t, pool, "dated-3")

	before := time.Now().Add(-time.Minute)
	row, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid})
	if err != nil {
		t.Fatalf("undated apply: %v", err)
	}
	if !row.AppliedAt.Valid || row.AppliedAt.Time.Before(before) {
		t.Errorf("applied_at = %v, want approximately now", row.AppliedAt.Time)
	}
}
