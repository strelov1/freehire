//go:build integration

// Integration tests for the job-lifecycle SQL contract (openspec change
// close-stale-jobs): UpsertJob refreshes liveness and reopens closed jobs, the
// CloseUnseenJobs sweep closes only jobs past the cutoff, and the open-job
// filters keep closed jobs out of list/count/company surfaces while the detail
// lookup still resolves them.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// ageJob backdates a job's last_seen_at so sweep cutoffs can be exercised
// without sleeping.
func ageJob(t *testing.T, pool *pgxpool.Pool, id int64, seenAgo time.Duration) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"UPDATE jobs SET last_seen_at = now() - $2::interval WHERE id = $1",
		id, seenAgo.String())
	if err != nil {
		t.Fatalf("backdate last_seen_at: %v", err)
	}
}

func TestUpsertJobRefreshesLivenessAndReopens(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Engineer"))
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if !job.LastSeenAt.Valid {
		t.Fatal("insert must stamp last_seen_at")
	}
	if job.ClosedAt.Valid {
		t.Fatal("new job must be open")
	}

	// Make the job stale and closed, as the sweep would.
	ageJob(t, pool, job.ID, 72*time.Hour)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", job.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}

	// The posting reappears in a crawl: liveness refreshes, the job reopens.
	reingested, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Engineer"))
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if reingested.ClosedAt.Valid {
		t.Fatal("re-ingest must reopen a closed job")
	}
	// The job was backdated 72h above, so a fresh stamp proves the refresh.
	if time.Since(reingested.LastSeenAt.Time) > time.Minute {
		t.Fatalf("re-ingest must refresh last_seen_at, got %v", reingested.LastSeenAt.Time)
	}
}

func TestCloseUnseenJobsClosesOnlyStaleJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	stale, err := ingestUpsert(ctx, q, ingestParams("acme:stale", "Stale"))
	if err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	fresh, err := ingestUpsert(ctx, q, ingestParams("acme:fresh", "Fresh"))
	if err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}
	ageJob(t, pool, stale.ID, 49*time.Hour)
	ageJob(t, pool, fresh.ID, 6*time.Hour)

	closed, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour))})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if closed != 1 {
		t.Fatalf("sweep closed %d jobs, want 1", closed)
	}

	staleAfter, err := q.GetJob(ctx, stale.ID)
	if err != nil {
		t.Fatalf("get stale: %v", err)
	}
	if !staleAfter.ClosedAt.Valid {
		t.Fatal("stale job must be closed")
	}
	freshAfter, err := q.GetJob(ctx, fresh.ID)
	if err != nil {
		t.Fatalf("get fresh: %v", err)
	}
	if freshAfter.ClosedAt.Valid {
		t.Fatal("fresh job must stay open")
	}

	// Idempotent: a second sweep with the same cutoff closes nothing.
	again, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour))})
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if again != 0 {
		t.Fatalf("second sweep closed %d jobs, want 0", again)
	}
}

func TestClosedJobsLeaveListSurfacesButResolveOnDetail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	open, err := ingestUpsert(ctx, q, ingestParams("acme:open", "Open"))
	if err != nil {
		t.Fatalf("upsert open: %v", err)
	}
	closed, err := ingestUpsert(ctx, q, ingestParams("acme:closed", "Closed"))
	if err != nil {
		t.Fatalf("upsert closed: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}

	jobs, err := q.ListJobs(ctx, ListJobsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != open.ID {
		t.Fatalf("list must contain only the open job, got %d rows", len(jobs))
	}

	// Assert the closed job is excluded from the open count. Use an exact count
	// here (not the production EstimateOpenJobs, which is an approximate planner
	// estimate) so this data invariant is tested deterministically.
	var total int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE closed_at IS NULL").Scan(&total); err != nil {
		t.Fatalf("count open jobs: %v", err)
	}
	if total != 1 {
		t.Fatalf("open count = %d, want 1 (closed excluded)", total)
	}

	byCompany, err := q.ListJobsByCompany(ctx, ListJobsByCompanyParams{
		CompanySlug: "acme", Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("list by company: %v", err)
	}
	if len(byCompany) != 1 || byCompany[0].ID != open.ID {
		t.Fatalf("company jobs must contain only the open job, got %d rows", len(byCompany))
	}

	// companies.job_count is denormalized: UpsertJob does not maintain it,
	// cmd/recount-companies does. Recompute before asserting on the column.
	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("recount company job counts: %v", err)
	}

	companies, err := q.ListCompanies(ctx, ListCompaniesParams{Search: "", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list companies: %v", err)
	}
	if len(companies) != 1 || companies[0].JobCount != 1 {
		t.Fatalf("company job_count must count only open jobs, got %+v", companies)
	}

	// Detail still resolves the closed job, carrying its closed_at.
	detail, err := q.GetJobBySlug(ctx, closed.PublicSlug)
	if err != nil {
		t.Fatalf("detail for closed job: %v", err)
	}
	if !detail.ClosedAt.Valid {
		t.Fatal("detail must carry closed_at")
	}
}
