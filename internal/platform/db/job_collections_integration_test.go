//go:build integration

// Integration test for PropagateCollectionsToJobsBatch: a company's curated-collection
// set is denormalized onto its jobs (matched by company_slug), an untagged
// company's jobs stay empty, and re-running is idempotent. This is SQL behavior
// (UPDATE ... FROM with an IS DISTINCT FROM guard), so it runs against a real
// Postgres. Run with: go test -tags=integration ./internal/platform/db/
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
// a resolved category. Public and bodied are not decoration — RefreshCompanyFacets scopes
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
// "private" or "no-body".
func insertUnsearchableJob(t *testing.T, pool *pgxpool.Pool, externalID, companySlug, flaw string) {
	t.Helper()
	body, category, private := "We are hiring.", "backend", false
	switch flaw {
	case "private":
		private = true
	case "no-body":
		body = ""
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

func TestPropagateCollectionsToJobsBatch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	insertCompanyWithCollections(t, pool, "stripe", "Stripe", []string{"yc", "bigtech"})
	insertCompanyWithCollections(t, pool, "acme", "Acme", []string{})
	insertJobForCompany(t, pool, "j-stripe-1", "stripe")
	insertJobForCompany(t, pool, "j-acme", "acme")
	insertJobForCompany(t, pool, "j-stripe-2", "stripe")

	// A batch of one walks the keyset: each batch takes the next diverging job past
	// the cursor, and acme's job (its empty set already matches) is never a batch.
	var after, total int64
	for batches := 0; ; batches++ {
		if batches > 3 {
			t.Fatal("propagation did not converge")
		}
		got, err := q.PropagateCollectionsToJobsBatch(ctx, PropagateCollectionsToJobsBatchParams{AfterID: after, BatchSize: 1})
		if err != nil {
			t.Fatalf("PropagateCollectionsToJobsBatch: %v", err)
		}
		if got.Updated == 0 {
			break
		}
		if got.LastID <= after {
			t.Fatalf("cursor did not advance: %d -> %d", after, got.LastID)
		}
		after, total = got.LastID, total+got.Updated
	}
	if total != 2 {
		t.Errorf("jobs updated = %d, want 2", total)
	}

	for _, id := range []string{"j-stripe-1", "j-stripe-2"} {
		if got := jobCollections(t, pool, id); !slices.Equal(got, []string{"yc", "bigtech"}) {
			t.Errorf("%s collections = %v, want [yc bigtech]", id, got)
		}
	}
	if got := jobCollections(t, pool, "j-acme"); len(got) != 0 {
		t.Errorf("acme job collections = %v, want empty", got)
	}

	// Idempotent: a fresh pass from the start finds nothing to change.
	got, err := q.PropagateCollectionsToJobsBatch(ctx, PropagateCollectionsToJobsBatchParams{AfterID: 0, BatchSize: 100})
	if err != nil {
		t.Fatalf("PropagateCollectionsToJobsBatch (2nd pass): %v", err)
	}
	if got.Updated != 0 {
		t.Errorf("2nd pass updated = %d, want 0 (idempotent)", got.Updated)
	}
}
