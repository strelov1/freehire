//go:build integration

// Integration tests for the on-site application record (migration 0187): one row per
// candidate and vacancy, a retry that replaces an unsent application and never a sent
// one, and the daily cap that a retry does not consume. All of it is one SQL statement's
// behaviour, verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestDirectApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('c@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	vacancy := func(externalID string) int64 {
		t.Helper()
		p := ingestParams(externalID, "Platform Engineer")
		p.Source = "employer"
		p.URL = ""
		row, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatalf("upsert vacancy: %v", err)
		}
		return row.ID
	}
	apply := func(jobID int64, note string, limit int64) (int64, error) {
		return q.UpsertDirectApplication(ctx, UpsertDirectApplicationParams{
			JobID: jobID, UserID: userID, Note: note, Answers: []byte(`[]`), DailyLimit: limit,
		})
	}
	note := func(jobID int64) string {
		t.Helper()
		var n string
		if err := pool.QueryRow(ctx, `SELECT note FROM direct_applications WHERE job_id = $1 AND user_id = $2`, jobID, userID).Scan(&n); err != nil {
			t.Fatalf("read note: %v", err)
		}
		return n
	}

	first, second := vacancy("freehire:a"), vacancy("freehire:b")

	id, err := apply(first, "draft", 1)
	if err != nil {
		t.Fatalf("first application: %v", err)
	}

	t.Run("an unsent application is replaced by a retry, which the cap does not count", func(t *testing.T) {
		// The first delivery failed and released its claim.
		if err := q.ReleaseDirectApplication(ctx, id); err != nil {
			t.Fatalf("release: %v", err)
		}
		retryID, err := apply(first, "retried", 1)
		if err != nil {
			t.Fatalf("retry: %v", err)
		}
		if retryID != id {
			t.Errorf("retry wrote row %d, want the same row %d", retryID, id)
		}
		if got := note(first); got != "retried" {
			t.Errorf("note = %q, want the retry's content", got)
		}
	})

	t.Run("an application being sent is not claimed twice", func(t *testing.T) {
		// The retry above claimed the row (sending_at = now()); a second request arriving
		// while that delivery is in flight must not get it too, or the employer receives
		// the application twice.
		if _, err := apply(first, "double click", 10); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, want no row while a delivery holds the claim", err)
		}
		row, err := q.GetDirectApplication(ctx, GetDirectApplicationParams{JobID: first, UserID: userID})
		if err != nil || !row.Sending || row.SentAt.Valid {
			t.Fatalf("GetDirectApplication = %+v, %v; want an in-flight, unsent row", row, err)
		}
		// A delivery that failed releases the claim, so the retry is immediate.
		if err := q.ReleaseDirectApplication(ctx, id); err != nil {
			t.Fatalf("release: %v", err)
		}
		if _, err := apply(first, "retried", 1); err != nil {
			t.Fatalf("retry after release: %v", err)
		}
		// A claim abandoned by a crashed request expires.
		if _, err := pool.Exec(ctx, `UPDATE direct_applications SET sending_at = now() - interval '3 minutes' WHERE id = $1`, id); err != nil {
			t.Fatalf("age claim: %v", err)
		}
		if _, err := apply(first, "retried", 1); err != nil {
			t.Fatalf("retry after an abandoned claim: %v", err)
		}
	})

	t.Run("the cap refuses an application to another vacancy", func(t *testing.T) {
		if _, err := apply(second, "x", 1); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, want no row (cap reached)", err)
		}
		if _, err := q.GetDirectApplication(ctx, GetDirectApplicationParams{JobID: second, UserID: userID}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("a capped application must not be recorded, got err = %v", err)
		}
	})

	t.Run("a sent application is never replaced", func(t *testing.T) {
		if err := q.MarkDirectApplicationSent(ctx, id); err != nil {
			t.Fatalf("mark sent: %v", err)
		}
		if _, err := apply(first, "overwrite", 10); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, want no row (already sent)", err)
		}
		if got := note(first); got != "retried" {
			t.Errorf("note = %q, a sent application must keep what the employer received", got)
		}
		row, err := q.GetDirectApplication(ctx, GetDirectApplicationParams{JobID: first, UserID: userID})
		if err != nil || !row.SentAt.Valid {
			t.Errorf("GetDirectApplication = %+v, %v; want a sent row", row, err)
		}
	})

	t.Run("yesterday's applications do not count", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE direct_applications SET created_at = now() - interval '1 day'`); err != nil {
			t.Fatalf("age rows: %v", err)
		}
		if _, err := apply(second, "today", 1); err != nil {
			t.Errorf("a new day's first application: %v", err)
		}
	})
}
