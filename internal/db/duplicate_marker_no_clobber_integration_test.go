//go:build integration

// Integration tests proving the defect this change fixes is gone: the three dedup passes no
// longer clear each other's verdicts. These exercise the OWNERSHIP property, not the matching
// rules — what each pass decides is covered by job_duplicate_of, aggregator_dedup,
// aggregator_subset_dedup and fuzzy_dedup, and is deliberately unchanged here.
//
// The fuzzy pass is driven by calling MarkFuzzyDuplicatesForCompany with an explicit
// assignment, the way fuzzy_dedup_integration_test.go does: its clustering lives in Go
// (cmd/reindex/fuzzy.go) and is not what these tests are about.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// markFuzzy points one row at a fuzzy canon, as the fuzzy pass does once its Go-side clustering
// has chosen the assignment.
func markFuzzy(t *testing.T, q *Queries, company string, id, canon int64) int64 {
	t.Helper()
	n, err := q.MarkFuzzyDuplicatesForCompany(context.Background(), MarkFuzzyDuplicatesForCompanyParams{
		Company: company,
		Ids:     []int64{id},
		Canons:  []int64{canon},
	})
	if err != nil {
		t.Fatalf("mark fuzzy %d -> %d: %v", id, canon, err)
	}
	return n
}

// dupExt reads a row's derived canon as an external_id, so an expectation can be written
// independently of the ids a run happens to mint. Empty string means canonical.
func dupExt(t *testing.T, pool *pgxpool.Pool, ext string) string {
	t.Helper()
	var canon *string
	if err := pool.QueryRow(context.Background(),
		`SELECT c.external_id FROM jobs j LEFT JOIN jobs c ON c.id = j.duplicate_of
		 WHERE j.external_id = $1`, ext).Scan(&canon); err != nil {
		t.Fatalf("read canon of %s: %v", ext, err)
	}
	if canon == nil {
		return ""
	}
	return *canon
}

// withRoleFP stamps a role fingerprint onto a fixture built by atsJob/aggJob.
func withRoleFP(p UpsertJobParams, fp string) UpsertJobParams {
	p.RoleFingerprint = pgtype.Text{String: fp, Valid: true}
	return p
}

// TestRoleRecomputeDoesNotClearAggregatorSuppression is the regression test for the defect.
//
// Before ownership, the role recompute derived duplicate_of from scratch over role_fingerprint
// clusters and wrote NULL to every row that was a canon or a singleton in its own cluster. A
// suppressed aggregator posting that happens to be a singleton by role is exactly that shape, so
// the recompute released it — and the suppression pass re-applied it later in the same run. On
// prod that was 125k rows re-marked every cycle, six cycles a day, with an hour between the
// clearing and the repair in which those postings stood in search as canonical.
func TestRoleRecomputeDoesNotClearAggregatorSuppression(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Each row is a singleton in its own role cluster, which is what made the recompute
	// clear the suppression.
	mustUpsert(t, q, withRoleFP(atsJob("acme:ats", "Platform Engineer", []string{"US"}), "fp-ats"))
	mustUpsert(t, q, withRoleFP(aggJob("acme:agg", "Platform Engineer", []string{"US"}), "fp-agg"))

	suppressAggregators(t, q)
	if got := dupExt(t, pool, "acme:agg"); got != "acme:ats" {
		t.Fatalf("suppression did not take: aggregator canon = %q, want acme:ats", got)
	}

	recomputeDuplicates(t, q)

	if got := dupExt(t, pool, "acme:agg"); got != "acme:ats" {
		t.Errorf("role recompute released the suppression: aggregator canon = %q, want acme:ats", got)
	}
	if got := dupExt(t, pool, "acme:ats"); got != "" {
		t.Errorf("ATS row stopped being canonical: canon = %q, want none", got)
	}
}

// TestRoleRecomputeDoesNotClearFuzzyCollapse is the same defect on the other pass. The fuzzy pass
// collapses reposts whose descriptions differ, so its rows carry DIFFERENT role fingerprints —
// singletons by role, and therefore cleared by the recompute.
func TestRoleRecomputeDoesNotClearFuzzyCollapse(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withRoleFP(atsJob("acme:canon", "Backend Engineer", []string{"US"}), "fp-1"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:near", "Backend Engineer", []string{"US"}), "fp-2"))
	canonID, _ := dupOf(t, pool, "acme:canon")
	nearID, _ := dupOf(t, pool, "acme:near")

	markFuzzy(t, q, "acme", nearID, canonID)
	if got := dupExt(t, pool, "acme:near"); got != "acme:canon" {
		t.Fatalf("fuzzy collapse did not take: canon = %q, want acme:canon", got)
	}

	recomputeDuplicates(t, q)

	if got := dupExt(t, pool, "acme:near"); got != "acme:canon" {
		t.Errorf("role recompute released the fuzzy collapse: canon = %q, want acme:canon", got)
	}
}

// TestFullMarkerRefreshIsIdempotent is the acceptance criterion in miniature. A second cycle over
// an unchanged catalogue must write nothing. Before ownership the recompute alone re-marked the
// entire aggregator and fuzzy populations on every cycle — ~470k rows on prod — because it was
// clearing what the other two had just written.
func TestFullMarkerRefreshIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// One role cluster, one aggregator/ATS pair, one fuzzy pair — one row for each pass to own.
	mustUpsert(t, q, withRoleFP(atsJob("acme:role1", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:role2", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:ats", "Platform Engineer", []string{"US"}), "fp-ats"))
	mustUpsert(t, q, withRoleFP(aggJob("acme:agg", "Platform Engineer", []string{"US"}), "fp-agg"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:fcanon", "Backend Engineer", []string{"US"}), "fp-f1"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:fnear", "Backend Engineer", []string{"US"}), "fp-f2"))
	fcanonID, _ := dupOf(t, pool, "acme:fcanon")
	fnearID, _ := dupOf(t, pool, "acme:fnear")

	cycle := func() (role, agg, fuzzy int64) {
		companies, err := q.CompaniesWithRoleClusters(ctx)
		if err != nil {
			t.Fatalf("companies with clusters: %v", err)
		}
		if len(companies) > 0 {
			if role, err = q.RecomputeRoleDuplicatesForCompanies(ctx, companies); err != nil {
				t.Fatalf("recompute: %v", err)
			}
		}
		aggCompanies, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
		if err != nil {
			t.Fatalf("companies with aggregator postings: %v", err)
		}
		if len(aggCompanies) > 0 {
			agg, err = q.SuppressAggregatorDuplicatesForCompanies(ctx, SuppressAggregatorDuplicatesForCompaniesParams{
				FoldedCompanies: aggCompanies,
				Aggregators:     aggregators,
			})
			if err != nil {
				t.Fatalf("suppress: %v", err)
			}
		}
		fuzzy = markFuzzy(t, q, "acme", fnearID, fcanonID)
		return role, agg, fuzzy
	}

	if role, agg, fuzzy := cycle(); role == 0 && agg == 0 && fuzzy == 0 {
		t.Fatalf("first cycle marked nothing (role=%d agg=%d fuzzy=%d) — the fixture is not "+
			"exercising the passes", role, agg, fuzzy)
	}

	role, agg, fuzzy := cycle()
	if role != 0 {
		t.Errorf("second cycle: role recompute re-marked %d rows, want 0 — it is still clearing "+
			"another pass's markers", role)
	}
	if agg != 0 {
		t.Errorf("second cycle: aggregator suppression re-marked %d rows, want 0", agg)
	}
	if fuzzy != 0 {
		t.Errorf("second cycle: fuzzy collapse re-marked %d rows, want 0", fuzzy)
	}

	// And the end state is still correct, not merely stable.
	if got := dupExt(t, pool, "acme:role2"); got != "acme:role1" {
		t.Errorf("role repost canon = %q, want acme:role1", got)
	}
	if got := dupExt(t, pool, "acme:agg"); got != "acme:ats" {
		t.Errorf("aggregator canon = %q, want acme:ats", got)
	}
	if got := dupExt(t, pool, "acme:fnear"); got != "acme:fcanon" {
		t.Errorf("fuzzy repost canon = %q, want acme:fcanon", got)
	}
}

// TestMarkerRefreshIsOrderIndependent pins what ownership buys beyond idempotence: pass order
// stops being load-bearing for correctness. It stays as a cost and merge-quality rule — the fuzzy
// pass still only considers rows the other two left canonical — but a run interrupted between
// passes, or reordered, no longer leaves a duplicate standing as canonical.
//
// The comparison is on the DERIVED canon. An owned column may legitimately differ between the two
// orders: run fuzzy first and it marks a row the role pass then also claims, leaving a redundant
// fuzzy marker that COALESCE never surfaces. Harmless, and precisely the kind of disagreement the
// precedence exists to settle.
func TestMarkerRefreshIsOrderIndependent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	fixture := func() (fcanonID, fnearID int64) {
		truncate(t, pool)
		mustUpsert(t, q, withRoleFP(atsJob("acme:role1", "Staff Engineer", []string{"US"}), "fp-role"))
		mustUpsert(t, q, withRoleFP(atsJob("acme:role2", "Staff Engineer", []string{"US"}), "fp-role"))
		mustUpsert(t, q, withRoleFP(atsJob("acme:ats", "Platform Engineer", []string{"US"}), "fp-ats"))
		mustUpsert(t, q, withRoleFP(aggJob("acme:agg", "Platform Engineer", []string{"US"}), "fp-agg"))
		mustUpsert(t, q, withRoleFP(atsJob("acme:fcanon", "Backend Engineer", []string{"US"}), "fp-f1"))
		mustUpsert(t, q, withRoleFP(atsJob("acme:fnear", "Backend Engineer", []string{"US"}), "fp-f2"))
		c, _ := dupOf(t, pool, "acme:fcanon")
		n, _ := dupOf(t, pool, "acme:fnear")
		return c, n
	}

	role := func() {
		companies, err := q.CompaniesWithRoleClusters(ctx)
		if err != nil {
			t.Fatalf("companies with clusters: %v", err)
		}
		if len(companies) > 0 {
			if _, err := q.RecomputeRoleDuplicatesForCompanies(ctx, companies); err != nil {
				t.Fatalf("recompute: %v", err)
			}
		}
	}

	canons := func() map[string]string {
		out := map[string]string{}
		for _, ext := range []string{"acme:role1", "acme:role2", "acme:ats", "acme:agg", "acme:fcanon", "acme:fnear"} {
			out[ext] = dupExt(t, pool, ext)
		}
		return out
	}

	// Production order: role, aggregator, fuzzy.
	fcanonID, fnearID := fixture()
	role()
	suppressAggregators(t, q)
	markFuzzy(t, q, "acme", fnearID, fcanonID)
	forward := canons()

	// Reversed: fuzzy, aggregator, role.
	fcanonID, fnearID = fixture()
	markFuzzy(t, q, "acme", fnearID, fcanonID)
	suppressAggregators(t, q)
	role()
	reversed := canons()

	for ext, want := range forward {
		if got := reversed[ext]; got != want {
			t.Errorf("%s: canon = %q under the reversed order, %q under the production order — "+
				"pass order is still load-bearing", ext, got, want)
		}
	}
}
