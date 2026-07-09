//go:build integration

// Integration test for the job-derived remote_regions facet: RefreshCompanyFacets
// maintains companies.remote_regions as the distinct union of regions over the
// company's OPEN REMOTE jobs (work_mode='remote'), a subset of the broader regions
// array. Verified against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// insertJobWithMode seeds an open job carrying a regions array and a work mode, so
// the recompute can scope remote_regions to remote jobs.
func insertJobWithMode(t *testing.T, pool *pgxpool.Pool, externalID, companySlug, workMode string, regions []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, regions, work_mode)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4)`,
		externalID, companySlug, regions, workMode); err != nil {
		t.Fatalf("insert job %q: %v", externalID, err)
	}
}

func sortedCopy(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func TestRefreshCompanyFacetsDerivesRemoteRegions(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Globex hires remotely in eu and apac (two remote jobs) and has an onsite job
	// in north_america — remote_regions must exclude the onsite region.
	insertCompany(t, pool, "globex", "Globex")
	insertJobWithMode(t, pool, "globex:r1", "globex", "remote", []string{"eu"})
	insertJobWithMode(t, pool, "globex:r2", "globex", "remote", []string{"apac"})
	insertJobWithMode(t, pool, "globex:on", "globex", "onsite", []string{"north_america"})

	// Onsite-only company: no remote job → empty remote_regions.
	insertCompany(t, pool, "onsiteco", "Onsite Co")
	insertJobWithMode(t, pool, "onsiteco:1", "onsiteco", "onsite", []string{"eu"})

	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("refresh facets: %v", err)
	}

	globex, err := q.GetCompany(ctx, "globex")
	if err != nil {
		t.Fatalf("get globex: %v", err)
	}
	if got := sortedCopy(globex.RemoteRegions); !slices.Equal(got, []string{"apac", "eu"}) {
		t.Errorf("globex remote_regions = %v, want [apac eu] (remote jobs only)", got)
	}
	if got := sortedCopy(globex.Regions); !slices.Equal(got, []string{"apac", "eu", "north_america"}) {
		t.Errorf("globex regions = %v, want [apac eu north_america] (all open jobs)", got)
	}

	onsite, err := q.GetCompany(ctx, "onsiteco")
	if err != nil {
		t.Fatalf("get onsiteco: %v", err)
	}
	if len(onsite.RemoteRegions) != 0 {
		t.Errorf("onsiteco remote_regions = %v, want empty (no remote job)", onsite.RemoteRegions)
	}
}
