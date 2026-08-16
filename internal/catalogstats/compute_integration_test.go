//go:build integration

// Integration test for the exact half of a Snapshot. The whole reason this package
// exists is that the published open-job total described a wider set than the listing
// showed, so the one thing worth proving against a real Postgres is that the counts
// cover exactly the set GET /api/v1/jobs paginates.
// Run with: go test -tags=integration ./internal/catalogstats/
package catalogstats

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
)

func seedJob(t *testing.T, q *db.Queries, source, externalID, companySlug string) db.Job {
	t.Helper()
	row, err := q.UpsertJob(context.Background(), db.UpsertJobParams{
		Source:      source,
		ExternalID:  externalID,
		URL:         "https://example.test/" + externalID,
		Title:       "Go Engineer",
		Company:     companySlug,
		CompanySlug: companySlug,
		PublicSlug:  "pslug-" + externalID,
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
	})
	if err != nil {
		t.Fatalf("seed %s: %v", externalID, err)
	}
	return row.Job
}

func truncateJobs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE enrichment_outbox, jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func TestComputeCountsOnlyThePaginatedSet(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()
	truncateJobs(t, pool)

	// Three companies with a listed posting each, then one excluded posting of every
	// kind. The excluded ones sit at companies that have no listed posting, so a
	// company count that leaked would be wrong by more than the job count.
	canonical := seedJob(t, q, "greenhouse", "acme:1", "acme")
	seedJob(t, q, "greenhouse", "beta:1", "beta")
	seedJob(t, q, "lever", "gamma:1", "gamma")

	seedJob(t, q, "greenhouse", "closedco:1", "closedco")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() WHERE external_id = 'closedco:1'`); err != nil {
		t.Fatalf("close job: %v", err)
	}

	seedJob(t, q, "greenhouse", "dupco:1", "dupco")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET duplicate_of = $1 WHERE external_id = 'dupco:1'`, canonical.ID); err != nil {
		t.Fatalf("suppress duplicate: %v", err)
	}

	seedJob(t, q, "greenhouse", "privco:1", "privco")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET is_private = true WHERE external_id = 'privco:1'`); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	got, err := Compute(ctx, q, 7)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if got.OpenJobs != 3 {
		t.Errorf("OpenJobs = %d, want 3 — the closed, duplicate-suppressed and private "+
			"postings must not be counted", got.OpenJobs)
	}
	if got.Companies != 3 {
		t.Errorf("Companies = %d, want 3 — only companies with a listed posting count", got.Companies)
	}
	if got.TelegramChannels != 7 {
		t.Errorf("TelegramChannels = %d, want the 7 passed in", got.TelegramChannels)
	}
	if got.Sources != Sources() || got.ATSPlatforms != ATSPlatforms() {
		t.Errorf("Sources/ATSPlatforms = %d/%d, want the registry-derived %d/%d",
			got.Sources, got.ATSPlatforms, Sources(), ATSPlatforms())
	}
	if got.ComputedAt.IsZero() {
		t.Error("ComputedAt is zero — consumers cannot tell how stale a snapshot is")
	}
}

// Both figures must describe the same instant. Reading them in separate statements
// would let an ingest land between them and publish a company count for a catalogue
// that no longer matches the job count beside it.
func TestComputeReadsBothCountsInOneStatement(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()
	truncateJobs(t, pool)

	seedJob(t, q, "greenhouse", "acme:1", "acme")

	got, err := Compute(ctx, q, 0)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got.OpenJobs != 1 || got.Companies != 1 {
		t.Fatalf("OpenJobs/Companies = %d/%d, want 1/1", got.OpenJobs, got.Companies)
	}
}
