//go:build integration

// Integration tests for the cross-source aggregator suppression pass: per company, an
// open aggregator posting is marked duplicate_of an open canonical ATS posting of the
// same normalized title and compatible country, keeping the ATS row canonical. The
// title normalization, country gate, candidate selection (never clobbering a role-pass
// repost), and failover on close are SQL behaviors verifiable only against a real
// Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"
)

// aggregators is the provider set treated as aggregators in these tests.
var aggregators = []string{"himalayas", "gulftalent"}

// atsJob builds an open ATS (non-aggregator) posting.
func atsJob(externalID, title string, countries []string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.Source = "greenhouse"
	p.Countries = countries
	return p
}

// aggJob builds an open aggregator posting.
func aggJob(externalID, title string, countries []string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.Source = "himalayas"
	p.Countries = countries
	return p
}

// suppressAggregators drives the pass per company, as cmd/ingest does.
func suppressAggregators(t *testing.T, q *Queries) {
	t.Helper()
	ctx := context.Background()
	companies, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
	if err != nil {
		t.Fatalf("companies with aggregator postings: %v", err)
	}
	for _, c := range companies {
		if _, err := q.SuppressAggregatorDuplicatesForCompany(ctx, SuppressAggregatorDuplicatesForCompanyParams{
			Company:     c,
			Aggregators: aggregators,
		}); err != nil {
			t.Fatalf("suppress company %q: %v", c, err)
		}
	}
}

func mustUpsert(t *testing.T, q *Queries, p UpsertJobParams) {
	t.Helper()
	if _, err := q.UpsertJob(context.Background(), p); err != nil {
		t.Fatalf("upsert %s: %v", p.ExternalID, err)
	}
}

func TestSuppressAggregator_MarksAggregatorDuplicateOfATS(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Staff Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Staff Engineer", []string{"US"}))

	suppressAggregators(t, q)

	atsID, atsDup := dupOf(t, pool, "acme:ats")
	if atsDup != -1 {
		t.Errorf("ATS row duplicate_of = %d, want NULL (canonical)", atsDup)
	}
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d", aggDup, atsID)
	}

	// Idempotent: a second run writes nothing new.
	suppressAggregators(t, q)
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("after re-run aggregator duplicate_of = %d, want ATS %d", aggDup, atsID)
	}
}

func TestSuppressAggregator_EmptyCountryStillMatches(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// The geography dictionary is sparse; an unresolved (empty) country must not veto.
	mustUpsert(t, q, atsJob("acme:ats", "Data Scientist", nil))
	mustUpsert(t, q, aggJob("acme:agg", "Data Scientist", []string{"AE"}))

	suppressAggregators(t, q)

	atsID, _ := dupOf(t, pool, "acme:ats")
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (empty country must not veto)", aggDup, atsID)
	}
}

func TestSuppressAggregator_DifferentCountryNotSuppressed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// The 7-Eleven false-merge: same title, disjoint non-empty countries.
	mustUpsert(t, q, atsJob("711:us", "Store Associate", []string{"US"}))
	mustUpsert(t, q, aggJob("711:sg", "Store Associate", []string{"SG"}))

	suppressAggregators(t, q)

	if _, dup := dupOf(t, pool, "711:sg"); dup != -1 {
		t.Errorf("aggregator across country duplicate_of = %d, want NULL (not suppressed)", dup)
	}
}

func TestSuppressAggregator_NeverDemotesATS(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Two ATS rows, no aggregator: the pass must touch nothing.
	mustUpsert(t, q, atsJob("acme:ats1", "Backend Engineer", []string{"US"}))
	mustUpsert(t, q, atsJob("acme:ats2", "Backend Engineer", []string{"US"}))

	suppressAggregators(t, q)

	if _, d := dupOf(t, pool, "acme:ats1"); d != -1 {
		t.Errorf("ats1 duplicate_of = %d, want NULL", d)
	}
	if _, d := dupOf(t, pool, "acme:ats2"); d != -1 {
		t.Errorf("ats2 duplicate_of = %d, want NULL (this pass never demotes an ATS row)", d)
	}
}

func TestSuppressAggregator_TwoAggregatorsWithoutATSUntouched(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Same title, both aggregators, no ATS twin: this pass suppresses neither
	// (cross-aggregator collapse is the role-cluster pass's job).
	p1 := aggJob("acme:h", "Product Manager", []string{"US"})
	p2 := aggJob("acme:g", "Product Manager", []string{"US"})
	p2.Source = "gulftalent"
	mustUpsert(t, q, p1)
	mustUpsert(t, q, p2)

	suppressAggregators(t, q)

	if _, d := dupOf(t, pool, "acme:h"); d != -1 {
		t.Errorf("himalayas duplicate_of = %d, want NULL", d)
	}
	if _, d := dupOf(t, pool, "acme:g"); d != -1 {
		t.Errorf("gulftalent duplicate_of = %d, want NULL", d)
	}
}

func TestSuppressAggregator_ReleasesWhenATSTwinCloses(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "SRE", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "SRE", []string{"US"}))
	suppressAggregators(t, q)
	atsID, _ := dupOf(t, pool, "acme:ats")
	if _, d := dupOf(t, pool, "acme:agg"); d != atsID {
		t.Fatalf("precondition: aggregator should be suppressed to ATS, got %d", d)
	}

	// The ATS twin closes; the aggregator copy must un-suppress and re-enter surfaces.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE external_id = $1", "acme:ats"); err != nil {
		t.Fatalf("close ATS twin: %v", err)
	}
	suppressAggregators(t, q)

	if _, d := dupOf(t, pool, "acme:agg"); d != -1 {
		t.Errorf("after ATS twin closed, aggregator duplicate_of = %d, want NULL (released)", d)
	}
}

func TestSuppressedAggregator_HiddenFromListAndEnrichmentButServedBySlug(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Platform Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Platform Engineer", []string{"US"}))
	suppressAggregators(t, q)

	atsID, _ := dupOf(t, pool, "acme:ats")
	aggID, aggDup := dupOf(t, pool, "acme:agg")
	if aggDup != atsID {
		t.Fatalf("precondition: aggregator should be suppressed to ATS, got %d", aggDup)
	}

	// ListJobs returns the ATS canon, not the suppressed aggregator copy.
	jobs, err := q.ListJobs(ctx, ListJobsParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	seen := map[int64]bool{}
	for _, j := range jobs {
		seen[j.ID] = true
	}
	if !seen[atsID] {
		t.Errorf("ListJobs missing ATS canon %d", atsID)
	}
	if seen[aggID] {
		t.Errorf("ListJobs returned suppressed aggregator %d, want it hidden", aggID)
	}

	// EnqueuePendingJobs enqueues the ATS canon, not the suppressed aggregator copy.
	if _, err := q.EnqueuePendingJobs(ctx, EnqueuePendingJobsParams{TargetVersion: 1}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !outboxHas(t, pool, atsID) {
		t.Errorf("ATS canon %d not enqueued", atsID)
	}
	if outboxHas(t, pool, aggID) {
		t.Errorf("suppressed aggregator %d enqueued, want it skipped", aggID)
	}

	// The suppressed copy is still fetchable by its public slug (direct link).
	if _, err := q.GetJobBySlug(ctx, "pslug-acme:agg"); err != nil {
		t.Errorf("GetJobBySlug for suppressed aggregator: %v", err)
	}
}

// verify the driver only returns companies that actually have an open aggregator posting.
func TestCompaniesWithAggregatorPostings_OnlyAggregatorCompanies(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, aggJob("acme:agg", "Engineer", []string{"US"}))
	atsOnly := atsJob("other:ats", "Engineer", []string{"US"})
	atsOnly.CompanySlug = "other"
	atsOnly.Company = "Other"
	mustUpsert(t, q, atsOnly)

	got, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	if !slices.Contains(got, "acme") {
		t.Errorf("driver = %v, want it to include %q", got, "acme")
	}
	if slices.Contains(got, "other") {
		t.Errorf("driver = %v, want it to exclude ATS-only company %q", got, "other")
	}
}
