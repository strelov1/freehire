//go:build integration

// Integration tests for the age rule (openspec change telegram-vacancy-expiry): a job from
// a source that carries no close signal at all is closed once it is older than the window.
// The rule closes on a guess rather than on evidence, so its blast radius matters more than
// its reach — the tests below pin both edges of the window and prove it cannot touch a
// source that some other mechanism already covers.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setPosting pins a job's effective posting date to an ABSOLUTE instant rather than an
// offset from now(). The age rule compares posted_at against a cutoff the caller computes,
// so a boundary case seeded as "now() - window" drifts past the cutoff by however long the
// test takes to reach the query — the boundary would then be untestable by construction.
func setPosting(t *testing.T, pool *pgxpool.Pool, id int64, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"UPDATE jobs SET posted_at = $2 WHERE id = $1", id, at)
	if err != nil {
		t.Fatalf("set posted_at: %v", err)
	}
}

// seedJob inserts one open job for a given source, posted at the given instant.
func seedJob(t *testing.T, q *Queries, pool *pgxpool.Pool, source, externalID string, at time.Time) int64 {
	t.Helper()
	p := ingestParams(externalID, "Engineer")
	p.Source = source
	p.PublicSlug = source + "-" + externalID
	job, err := ingestUpsert(context.Background(), q, p)
	if err != nil {
		t.Fatalf("seed %s/%s: %v", source, externalID, err)
	}
	setPosting(t, pool, job.ID, at)
	return job.ID
}

func isClosed(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var closed bool
	if err := pool.QueryRow(context.Background(),
		"SELECT closed_at IS NOT NULL FROM jobs WHERE id = $1", id).Scan(&closed); err != nil {
		t.Fatalf("read closed_at: %v", err)
	}
	return closed
}

func TestCloseStaleUnsignalledJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// One cutoff, fixed up front, and every row placed relative to IT — not to now().
	cutoff := time.Now().Add(-45 * 24 * time.Hour)

	stale := seedJob(t, q, pool, "telegram", "tg:stale", cutoff.Add(-24*time.Hour))
	boundary := seedJob(t, q, pool, "telegram", "tg:boundary", cutoff)
	fresh := seedJob(t, q, pool, "telegram", "tg:fresh", cutoff.Add(24*time.Hour))
	// Sources some other mechanism already covers: the sweep owns greenhouse, the liveness
	// probe owns manual. Age must not override evidence, however old the row is.
	board := seedJob(t, q, pool, "greenhouse", "gh:ancient", cutoff.Add(-365*24*time.Hour))
	probeable := seedJob(t, q, pool, "manual", "man:ancient", cutoff.Add(-365*24*time.Hour))

	n, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
		Sources: []string{"telegram"},
		Cutoff:  pgTimestamptz(cutoff),
	})
	if err != nil {
		t.Fatalf("age rule: %v", err)
	}
	if n != 1 {
		t.Fatalf("age rule closed %d jobs, want 1", n)
	}

	if !isClosed(t, pool, stale) {
		t.Error("a telegram job past the window must close")
	}
	if got := closeReason(t, pool, stale); got != "expired" {
		t.Errorf("closed_reason = %q, want %q", got, "expired")
	}
	// Exactly at the cutoff the row is not yet PAST the window, so it survives. Pinned
	// because an off-by-one here silently closes a day's worth of live vacancies.
	if isClosed(t, pool, boundary) {
		t.Error("a telegram job exactly at the cutoff must stay open")
	}
	if isClosed(t, pool, fresh) {
		t.Error("a telegram job inside the window must stay open")
	}
	if isClosed(t, pool, board) {
		t.Error("age must not close a board job the ingest sweep owns")
	}
	if isClosed(t, pool, probeable) {
		t.Error("age must not close a job the liveness probe owns")
	}
}

func TestCloseStaleUnsignalledJobsIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	cutoff := pgTimestamptz(time.Now().Add(-45 * 24 * time.Hour))
	seedJob(t, q, pool, "telegram", "tg:once", time.Now().Add(-60*24*time.Hour))

	first, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
		Sources: []string{"telegram"}, Cutoff: cutoff,
	})
	if err != nil || first != 1 {
		t.Fatalf("first run closed %d (err %v), want 1", first, err)
	}
	second, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
		Sources: []string{"telegram"}, Cutoff: cutoff,
	})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second != 0 {
		t.Errorf("second run closed %d jobs, want 0 — a cron worker runs this repeatedly", second)
	}
}

func TestCloseStaleUnsignalledJobsFallsBackToCreatedAt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Telegram extraction leaves posted_at NULL when the post carries no readable date.
	// The rule reads COALESCE(posted_at, created_at), so such a row still ages.
	p := ingestParams("tg:nodate", "Engineer")
	p.Source = "telegram"
	p.PublicSlug = "telegram-nodate"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET posted_at = NULL, created_at = now() - interval '60 days' WHERE id = $1",
		job.ID); err != nil {
		t.Fatalf("backdate created_at: %v", err)
	}

	if _, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
		Sources: []string{"telegram"},
		Cutoff:  pgTimestamptz(time.Now().Add(-45 * 24 * time.Hour)),
	}); err != nil {
		t.Fatalf("age rule: %v", err)
	}
	if !isClosed(t, pool, job.ID) {
		t.Error("a row with no posted_at must age by created_at")
	}
}
