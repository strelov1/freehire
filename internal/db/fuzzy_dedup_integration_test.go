//go:build integration

// Integration tests for the queries behind the fuzzy-description dedup pass: which companies it
// considers, which rows it offers for bucketing, and that its marker write is scoped and
// idempotent. The clustering itself is unit-tested in cmd/reindex.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFuzzyDedup_CandidatesExcludeRowsWithoutACompany(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Two rows sharing a title but no company_slug: on prod 105 212 such rows fall into 20 126
	// same-title buckets spanning up to four employers, so the bucket stops being a company
	// boundary and the pass must not see them at all.
	mustUpsert(t, q, jobWithCompany("nc:1", "Support Engineer", ""))
	mustUpsert(t, q, jobWithCompany("nc:2", "Support Engineer", ""))
	mustUpsert(t, q, jobWithCompany("acme:1", "Support Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:2", "Support Engineer", "acme"))

	companies, err := q.CompaniesWithFuzzyDedupCandidates(context.Background())
	if err != nil {
		t.Fatalf("CompaniesWithFuzzyDedupCandidates: %v", err)
	}
	for _, c := range companies {
		if c == "" {
			t.Fatalf("companies include the empty slug: %v", companies)
		}
	}
	found := false
	for _, c := range companies {
		if c == "acme" {
			found = true
		}
	}
	if !found {
		t.Errorf("companies = %v, want acme (two open canonical rows)", companies)
	}
}

func TestFuzzyDedup_CandidateTitlesSkipAlreadyMarkedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Platform Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:dup", "Platform Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:other", "Data Analyst", "acme"))

	canonID, _ := dupOf(t, pool, "acme:canon")
	dupID, _ := dupOf(t, pool, "acme:dup")
	// The exact pass already claimed this one; the fuzzy pass works over leftovers only.
	markDuplicate(t, pool, dupID, canonID)

	rows, err := q.FuzzyDedupCandidateTitlesForCompany(context.Background(), "acme")
	if err != nil {
		t.Fatalf("FuzzyDedupCandidateTitlesForCompany: %v", err)
	}
	for _, r := range rows {
		if r.ID == dupID {
			t.Errorf("row %d is already a duplicate and must not be offered again", dupID)
		}
	}
	if len(rows) != 2 {
		t.Errorf("got %d candidate rows, want 2 (the canon and the unrelated title)", len(rows))
	}
}

func TestFuzzyDedup_MarkIsScopedAndIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Backend Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:repost", "Backend Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("other:row", "Backend Engineer", "other"))

	canonID, _ := dupOf(t, pool, "acme:canon")
	repostID, _ := dupOf(t, pool, "acme:repost")
	otherID, _ := dupOf(t, pool, "other:row")

	// The other company's row is passed in deliberately: the company scope must reject it.
	arg := MarkFuzzyDuplicatesForCompanyParams{
		Ids:     []int64{repostID, otherID},
		Canons:  []int64{canonID, canonID},
		Company: "acme",
	}
	n, err := q.MarkFuzzyDuplicatesForCompany(context.Background(), arg)
	if err != nil {
		t.Fatalf("MarkFuzzyDuplicatesForCompany: %v", err)
	}
	if n != 1 {
		t.Errorf("marked %d rows, want 1 (the other company's row is out of scope)", n)
	}
	if _, dup := dupOf(t, pool, "acme:repost"); dup != canonID {
		t.Errorf("repost duplicate_of = %d, want %d", dup, canonID)
	}
	if _, dup := dupOf(t, pool, "other:row"); dup != -1 {
		t.Errorf("other company's row was marked %d, want untouched", dup)
	}

	// Re-running the same assignment writes nothing: the IS DISTINCT FROM guard makes the pass
	// free to run on every reindex.
	again, err := q.MarkFuzzyDuplicatesForCompany(context.Background(), arg)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if again != 0 {
		t.Errorf("re-run marked %d rows, want 0 (idempotent)", again)
	}
}

// jobWithCompany is ingestParams with the company slug varied — the axis these tests turn on,
// including the empty slug the pass must refuse to bucket.
func jobWithCompany(externalID, title, companySlug string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.CompanySlug = companySlug
	p.Company = companySlug
	return p
}

// markDuplicate stands in for whatever earlier pass claimed a row, so a test can assert the fuzzy
// pass works over leftovers only.
func markDuplicate(t *testing.T, pool *pgxpool.Pool, id, canon int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE jobs SET duplicate_of_role = $2 WHERE id = $1`, id, canon); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}
}
