//go:build integration

// Integration tests for the queries behind the fuzzy-description dedup pass: which companies it
// considers, which rows it offers for bucketing, and that its marker write is scoped, reversible
// and idempotent. The clustering itself is unit-tested in cmd/reindex.
//
// Reversibility is the property these guard hardest. Migrations 0114/0115 moved the marker into
// duplicate_of_fuzzy and left the derived duplicate_of to a trigger, which quietly ended the old
// arrangement where the role recompute reversed this pass — the role pass writes a DIFFERENT
// column and cannot clear a fuzzy marker, and the COALESCE keeps surfacing it. Releasing is this
// pass's own duty now. Measured on prod 2026-09-01, 42 633 open duplicates were sitting behind a
// closed owner: invisible in search, and with no pass that would ever let them go.
// Run with: go test -tags=integration ./internal/platform/db/
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
	if !containsSlug(companies, "acme") {
		t.Errorf("companies = %v, want acme (two open unclaimed rows)", companies)
	}
}

// A company whose only remaining work is RELEASING a marker must still be visited. Without this
// the driver query is a collapse-only worklist and a marker whose cluster dissolved is never
// reconsidered — the shape that left 42 633 rows stranded on prod.
func TestFuzzyDedup_CompaniesIncludeOnesWithOnlyAMarkerToRelease(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("solo:canon", "Platform Engineer", "solo"))
	mustUpsert(t, q, jobWithCompany("solo:dup", "Platform Engineer", "solo"))
	canonID, _ := dupOf(t, pool, "solo:canon")
	dupID, _ := dupOf(t, pool, "solo:dup")
	markFuzzy(t, q, "solo", dupID, canonID)
	// The canon closes, so the company is down to ONE open unclaimed row and would fail a
	// "more than one candidate" test on its own.
	closeJob(t, pool, canonID)

	companies, err := q.CompaniesWithFuzzyDedupCandidates(context.Background())
	if err != nil {
		t.Fatalf("CompaniesWithFuzzyDedupCandidates: %v", err)
	}
	if !containsSlug(companies, "solo") {
		t.Errorf("companies = %v, want solo — it still holds a fuzzy marker to reconsider", companies)
	}
}

// The pass works over what the EXACT passes did not claim. A row carrying duplicate_of_role (or
// duplicate_of_aggregator) is theirs, and re-deciding it here would contradict a deterministic
// collapse.
func TestFuzzyDedup_CandidateTitlesSkipRowsClaimedByAnExactPass(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Platform Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:dup", "Platform Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:other", "Data Analyst", "acme"))

	canonID, _ := dupOf(t, pool, "acme:canon")
	dupID, _ := dupOf(t, pool, "acme:dup")
	markDuplicate(t, pool, dupID, canonID)

	rows, err := q.FuzzyDedupCandidateTitlesForCompany(context.Background(), "acme")
	if err != nil {
		t.Fatalf("FuzzyDedupCandidateTitlesForCompany: %v", err)
	}
	for _, r := range rows {
		if r.ID == dupID {
			t.Errorf("row %d belongs to the exact pass and must not be offered", dupID)
		}
	}
	if len(rows) != 2 {
		t.Errorf("got %d candidate rows, want 2 (the canon and the unrelated title)", len(rows))
	}
}

// A row this pass marked itself MUST come back as a candidate, or it can never be re-decided.
// The old predicate filtered the DERIVED duplicate_of, which a fuzzy marker sets, so the pass
// was blind to its own output — the defect that made every marker permanent.
func TestFuzzyDedup_CandidateTitlesOfferRowsThisPassMarked(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Platform Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:fuzzy", "Platform Engineer", "acme"))

	canonID, _ := dupOf(t, pool, "acme:canon")
	fuzzyID, _ := dupOf(t, pool, "acme:fuzzy")
	markFuzzy(t, q, "acme", fuzzyID, canonID)

	rows, err := q.FuzzyDedupCandidateTitlesForCompany(context.Background(), "acme")
	if err != nil {
		t.Fatalf("FuzzyDedupCandidateTitlesForCompany: %v", err)
	}
	var sawFuzzy bool
	for _, r := range rows {
		if r.ID == fuzzyID {
			sawFuzzy = true
		}
	}
	if !sawFuzzy {
		t.Errorf("row %d carries this pass's own marker and must be offered again; got %+v", fuzzyID, rows)
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
		Candidates: []int64{canonID, repostID, otherID},
		Ids:        []int64{repostID, otherID},
		Canons:     []int64{canonID, canonID},
		Company:    "acme",
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

// A candidate the pass considered and did NOT assign is released. This is the whole reversal
// mechanism: whatever dissolved the cluster — the canon closing, the descriptions diverging, an
// exact pass claiming the row — reaches the marker through the same door.
func TestFuzzyDedup_AConsideredCandidateWithNoAssignmentIsReleased(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Backend Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:repost", "Backend Engineer", "acme"))
	canonID, _ := dupOf(t, pool, "acme:canon")
	repostID, _ := dupOf(t, pool, "acme:repost")
	markFuzzy(t, q, "acme", repostID, canonID)
	if _, dup := dupOf(t, pool, "acme:repost"); dup != canonID {
		t.Fatalf("fixture: repost is not marked")
	}

	// A later run considers both rows and clusters neither.
	n, err := q.MarkFuzzyDuplicatesForCompany(ctx, MarkFuzzyDuplicatesForCompanyParams{
		Candidates: []int64{canonID, repostID},
		Ids:        nil,
		Canons:     nil,
		Company:    "acme",
	})
	if err != nil {
		t.Fatalf("MarkFuzzyDuplicatesForCompany: %v", err)
	}
	if n != 1 {
		t.Errorf("released %d rows, want 1", n)
	}
	if _, dup := dupOf(t, pool, "acme:repost"); dup != -1 {
		t.Errorf("repost duplicate_of = %d, want NULL (released)", dup)
	}

	// A released row must go back into the live index, through the same duplicate→canonical
	// bookkeeping a released role or aggregator duplicate already uses.
	var queued int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM search_outbox WHERE job_id = $1", repostID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Errorf("search_outbox rows for the released job = %d, want 1", queued)
	}
}

// The concrete reason a marker goes stale on prod: the canon closes, so it is no longer among
// the open rows the pass considers, and nothing is left to cluster the survivor onto.
func TestFuzzyDedup_AMarkerIsReleasedWhenItsCanonCloses(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, jobWithCompany("acme:canon", "Backend Engineer", "acme"))
	mustUpsert(t, q, jobWithCompany("acme:repost", "Backend Engineer", "acme"))
	canonID, _ := dupOf(t, pool, "acme:canon")
	repostID, _ := dupOf(t, pool, "acme:repost")
	markFuzzy(t, q, "acme", repostID, canonID)
	closeJob(t, pool, canonID)

	// What the pass now sees for this company: one open unclaimed row, which cannot cluster.
	rows, err := q.FuzzyDedupCandidateTitlesForCompany(ctx, "acme")
	if err != nil {
		t.Fatalf("FuzzyDedupCandidateTitlesForCompany: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != repostID {
		t.Fatalf("candidates = %+v, want just the survivor %d", rows, repostID)
	}
	candidates := []int64{rows[0].ID}

	if _, err := q.MarkFuzzyDuplicatesForCompany(ctx, MarkFuzzyDuplicatesForCompanyParams{
		Candidates: candidates, Company: "acme",
	}); err != nil {
		t.Fatalf("MarkFuzzyDuplicatesForCompany: %v", err)
	}
	if _, dup := dupOf(t, pool, "acme:repost"); dup != -1 {
		t.Errorf("repost duplicate_of = %d, want NULL — its canon is closed", dup)
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

// containsSlug is the string counterpart of the id-keyed contains in
// job_duplicate_semantic_integration_test.go.
func containsSlug(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// markDuplicate stands in for whatever EXACT pass claimed a row, so a test can assert the fuzzy
// pass works over leftovers only.
func markDuplicate(t *testing.T, pool *pgxpool.Pool, id, canon int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE jobs SET duplicate_of_role = $2 WHERE id = $1`, id, canon); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}
}
