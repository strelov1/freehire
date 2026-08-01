//go:build integration

// Integration tests for the orphan-company worklist: the companies the catalogue holds only
// through aggregators, which cmd/harvest-orphans turns into candidate ATS boards. The
// qualifying rule is an absence ("no open posting outside the aggregator set") evaluated
// against a different provider set than the one that supplies candidates, which is a SQL
// behavior verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

// allAggregators is the full aggregator set — the one the exclusion test must always use.
var allAggregators = []string{"himalayas", "remoteok", "gulftalent"}

// orphanJob builds an open posting for a company on a named source.
func orphanJob(source, externalID, company, slug string) UpsertJobParams {
	p := ingestParams(externalID, "Engineer")
	p.Source = source
	p.Company = company
	p.CompanySlug = slug
	return p
}

// orphanSlugs runs the worklist query and returns the company slugs it reported.
func orphanSlugs(t *testing.T, q *Queries, requested []string) map[string]string {
	t.Helper()
	rows, err := q.OrphanAggregatorCompanies(context.Background(), OrphanAggregatorCompaniesParams{
		Requested:   requested,
		Aggregators: allAggregators,
	})
	if err != nil {
		t.Fatalf("orphan companies: %v", err)
	}
	got := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, dup := got[r.CompanySlug]; dup {
			t.Errorf("company %q reported twice", r.CompanySlug)
		}
		got[r.CompanySlug] = r.Company
	}
	return got
}

func TestOrphanAggregatorCompanies(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	seed := []UpsertJobParams{
		// Held by one aggregator only — the target.
		orphanJob("himalayas", "h1", "Derq", "derq"),
		// Held by an aggregator AND its own ATS — already crawled, must be excluded.
		orphanJob("himalayas", "h2", "Covered Co", "covered"),
		orphanJob("greenhouse", "g1", "Covered Co", "covered"),
		// Held by two aggregators, no ATS — must appear exactly once.
		orphanJob("himalayas", "h3", "Twice Listed", "twice"),
		orphanJob("remoteok", "r1", "Twice Listed", "twice"),
		// Held only by an aggregator OUTSIDE the requested set.
		orphanJob("gulftalent", "gt1", "Gulf Only", "gulfonly"),
	}
	for _, p := range seed {
		mustUpsert(t, q, p)
	}

	got := orphanSlugs(t, q, []string{"himalayas", "remoteok"})

	if _, ok := got["derq"]; !ok {
		t.Error("an aggregator-only company must be in the worklist")
	}
	if got["derq"] != "Derq" {
		t.Errorf("company name = %q, want %q", got["derq"], "Derq")
	}
	if _, ok := got["covered"]; ok {
		t.Error("a company with its own ATS posting must be excluded")
	}
	if _, ok := got["twice"]; !ok {
		t.Error("a company held by two aggregators and no ATS must qualify")
	}
	if _, ok := got["gulfonly"]; ok {
		t.Error("a company held only outside the requested aggregators must not be reported")
	}
}

// Narrowing the requested aggregators must not turn another aggregator's posting into
// first-party ATS coverage: the exclusion test always considers every aggregator. A partial
// list here is the audit mistake that inflated the July aggregator-dedup leak count.
func TestOrphanAggregatorCompaniesNarrowedRequestStillExcludesOnlyATS(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	for _, p := range []UpsertJobParams{
		orphanJob("himalayas", "n1", "Both Aggregators", "both"),
		orphanJob("remoteok", "n2", "Both Aggregators", "both"),
	} {
		mustUpsert(t, q, p)
	}

	got := orphanSlugs(t, q, []string{"himalayas"})

	if _, ok := got["both"]; !ok {
		t.Error("the remoteok posting is another aggregator, not ATS coverage — company must still qualify")
	}
}

// A closed posting is not coverage: a company whose only ATS row has closed is an orphan
// again, and one whose only aggregator row has closed is not in the catalogue at all.
func TestOrphanAggregatorCompaniesIgnoresClosedPostings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)
	ctx := context.Background()

	for _, p := range []UpsertJobParams{
		orphanJob("himalayas", "c1", "Lapsed ATS", "lapsed"),
		orphanJob("greenhouse", "c2", "Lapsed ATS", "lapsed"),
		orphanJob("himalayas", "c3", "Gone", "gone"),
	} {
		mustUpsert(t, q, p)
	}
	if _, err := q.CloseJobBySourceExternalID(ctx, CloseJobBySourceExternalIDParams{
		Source: "greenhouse", ExternalID: "c2",
	}); err != nil {
		t.Fatalf("close ats posting: %v", err)
	}
	if _, err := q.CloseJobBySourceExternalID(ctx, CloseJobBySourceExternalIDParams{
		Source: "himalayas", ExternalID: "c3",
	}); err != nil {
		t.Fatalf("close aggregator posting: %v", err)
	}

	got := orphanSlugs(t, q, []string{"himalayas"})

	if _, ok := got["lapsed"]; !ok {
		t.Error("a company whose only ATS posting has closed is an orphan again")
	}
	if _, ok := got["gone"]; ok {
		t.Error("a company with no open aggregator posting must not be reported")
	}
}
