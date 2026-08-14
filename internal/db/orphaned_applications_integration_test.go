//go:build integration

// Integration tests for ListOrphanedApplications' OFFSET support and
// CountOrphanedApplications — added because the board's paging previously re-read the
// same top-N orphaned applications (those whose posting cmd/prune removed) on every
// page, and CountUserJobs never counted them at all. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedOrphanedApplication inserts an application with no posting (job_id NULL) — the
// shape cmd/prune leaves behind (see TestApplication_OutlivesItsPosting), reproduced
// directly here since only the orphaned shape itself matters for these tests.
func seedOrphanedApplication(t *testing.T, pool *pgxpool.Pool, userID int64, company string, appliedAt time.Time) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at)
		 VALUES ($1, $2, 'Orphaned Role', NULL, $3) RETURNING id`,
		userID, company, appliedAt).Scan(&id); err != nil {
		t.Fatalf("seed orphaned application: %v", err)
	}
	return id
}

func TestListOrphanedApplicationsPagesWithOffset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "orphan-paging@example.test", true)

	// Five orphaned applications, applied_at strictly decreasing so ORDER BY applied_at
	// DESC gives a stable, known order: o0 (newest) .. o4 (oldest).
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	ids := make([]int64, 5)
	for i := 0; i < 5; i++ {
		ids[i] = seedOrphanedApplication(t, pool, user, "orphanco", base.Add(-time.Duration(i)*time.Minute))
	}

	page1, err := q.ListOrphanedApplications(ctx, ListOrphanedApplicationsParams{UserID: user, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page2, err := q.ListOrphanedApplications(ctx, ListOrphanedApplicationsParams{UserID: user, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("page1=%d page2=%d rows, want 2 and 2", len(page1), len(page2))
	}
	if page1[0].ID == page2[0].ID || page1[1].ID == page2[1].ID {
		t.Fatalf("page 1 (%d, %d) and page 2 (%d, %d) overlap — offset is not advancing the window",
			page1[0].ID, page1[1].ID, page2[0].ID, page2[1].ID)
	}
	// Newest-first, so page1 = [ids[0], ids[1]], page2 = [ids[2], ids[3]].
	if page1[0].ID != ids[0] || page1[1].ID != ids[1] {
		t.Errorf("page1 = [%d %d], want [%d %d]", page1[0].ID, page1[1].ID, ids[0], ids[1])
	}
	if page2[0].ID != ids[2] || page2[1].ID != ids[3] {
		t.Errorf("page2 = [%d %d], want [%d %d]", page2[0].ID, page2[1].ID, ids[2], ids[3])
	}

	// A page far enough out reaches the last, previously-unreachable row.
	page3, err := q.ListOrphanedApplications(ctx, ListOrphanedApplicationsParams{UserID: user, Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3) != 1 || page3[0].ID != ids[4] {
		t.Fatalf("page3 = %v, want exactly [%d] — the 5th orphan must be reachable by paging further", page3, ids[4])
	}
}

func TestCountOrphanedApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "orphan-count@example.test", true)
	other := seedResponseUser(t, q, "orphan-count-other@example.test", true)

	if got, err := q.CountOrphanedApplications(ctx, user); err != nil || got != 0 {
		t.Fatalf("count with no orphans = %d, err=%v, want 0", got, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	seedOrphanedApplication(t, pool, user, "acme", now)
	seedOrphanedApplication(t, pool, user, "beta", now.Add(-time.Hour))
	// A posting-backed application (job_id set) must not be counted as orphaned, and
	// neither must another user's orphan.
	job := seedResponseJob(t, q, "orphan-count-linked", "acme")
	if _, err := pool.Exec(ctx,
		`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at) VALUES ($1, 'acme', 'Linked Role', $2, $3)`,
		user, job, now); err != nil {
		t.Fatalf("seed linked application: %v", err)
	}
	seedOrphanedApplication(t, pool, other, "gamma", now)

	got, err := q.CountOrphanedApplications(ctx, user)
	if err != nil {
		t.Fatalf("CountOrphanedApplications: %v", err)
	}
	if got != 2 {
		t.Errorf("count = %d, want 2 (only the user's own orphaned applications)", got)
	}
}
