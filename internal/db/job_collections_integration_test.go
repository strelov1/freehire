//go:build integration

// Integration test for PropagateCollectionsToJobs: a company's curated-collection
// set is denormalized onto its jobs (matched by company_slug), an untagged
// company's jobs stay empty, and re-running is idempotent. This is SQL behavior
// (UPDATE ... FROM with an IS DISTINCT FROM guard), so it runs against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func insertCompanyWithCollections(t *testing.T, pool *pgxpool.Pool, slug, name string, collections []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name, collections) VALUES ($1, $2, $3)`,
		slug, name, collections); err != nil {
		t.Fatalf("insert company %q: %v", slug, err)
	}
}

// insertJobForCompany seeds an ordinary, SEARCHABLE posting: public, with a body and
// a resolved category. Those last two are not decoration — RefreshCompanyFacets scopes
// its counts to what the job search index will hold, so a row missing either would be
// counted by no company. Tests that want an unsearchable row ask for one by name.
func insertJobForCompany(t *testing.T, pool *pgxpool.Pool, externalID, companySlug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug,
		                   description, category)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2,
		         'We are hiring.', 'backend')`,
		externalID, companySlug); err != nil {
		t.Fatalf("insert job %q: %v", externalID, err)
	}
}

// insertUnsearchableJob seeds a posting the job search index drops, so it must not
// reach a company's counts or facets either. `flaw` picks which exclusion to trip:
// "private", "no-body", or "uncategorized".
func insertUnsearchableJob(t *testing.T, pool *pgxpool.Pool, externalID, companySlug, flaw string) {
	t.Helper()
	body, category, private := "We are hiring.", "backend", false
	switch flaw {
	case "private":
		private = true
	case "no-body":
		body = ""
	case "uncategorized":
		// Both halves must miss: search.CategoryUnresolved accepts the enrichment's
		// category when the column is empty, and treats "other" as no answer.
		category = ""
	default:
		t.Fatalf("insertUnsearchableJob: unknown flaw %q", flaw)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug,
		                   description, category, is_private, enrichment)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2,
		         $3, $4, $5, '{"category":"other"}'::jsonb)`,
		externalID, companySlug, body, category, private); err != nil {
		t.Fatalf("insert unsearchable job %q: %v", externalID, err)
	}
}

func jobCollections(t *testing.T, pool *pgxpool.Pool, externalID string) []string {
	t.Helper()
	var got []string
	if err := pool.QueryRow(context.Background(),
		`SELECT collections FROM jobs WHERE external_id = $1`, externalID).Scan(&got); err != nil {
		t.Fatalf("read job collections %q: %v", externalID, err)
	}
	return got
}

func TestPropagateCollectionsToJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	insertCompanyWithCollections(t, pool, "stripe", "Stripe", []string{"yc", "bigtech"})
	insertCompanyWithCollections(t, pool, "acme", "Acme", []string{})
	insertJobForCompany(t, pool, "j-stripe", "stripe")
	insertJobForCompany(t, pool, "j-acme", "acme")

	rows, err := q.PropagateCollectionsToJobs(ctx)
	if err != nil {
		t.Fatalf("PropagateCollectionsToJobs: %v", err)
	}
	// Only the tagged company's job changes (acme's empty set already matches).
	if rows != 1 {
		t.Errorf("rows updated = %d, want 1", rows)
	}

	if got := jobCollections(t, pool, "j-stripe"); !slices.Equal(got, []string{"yc", "bigtech"}) {
		t.Errorf("stripe job collections = %v, want [yc bigtech]", got)
	}
	if got := jobCollections(t, pool, "j-acme"); len(got) != 0 {
		t.Errorf("acme job collections = %v, want empty", got)
	}

	// Idempotent: a second run changes nothing.
	rows, err = q.PropagateCollectionsToJobs(ctx)
	if err != nil {
		t.Fatalf("PropagateCollectionsToJobs (2nd): %v", err)
	}
	if rows != 0 {
		t.Errorf("2nd run rows updated = %d, want 0 (idempotent)", rows)
	}
}
