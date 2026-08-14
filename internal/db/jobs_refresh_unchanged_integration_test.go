//go:build integration

// Integration tests for the cheap ingest write: RefreshUnchangedJob advances last_seen_at on a
// re-seen posting that matches the stored row on the cheap path's key, writing nothing else —
// not even updated_at, which is what turns the column into "content last changed". It must match
// nothing when the content hash moved, when the structured cities moved, when the stored hash is
// NULL, when the row is closed, or when there is no row at all, so each of those falls through
// to the full upsert. All of it is SQL behaviour verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// unchangedParams is a re-crawl of the posting seedForRefresh wrote: same identity, same hash,
// same cities. Each case below varies exactly one of those to prove it is part of the match.
func unchangedParams() RefreshUnchangedJobParams {
	return RefreshUnchangedJobParams{
		Source:      "greenhouse",
		ExternalID:  "acme:1",
		ContentHash: pgtype.Text{String: "h1", Valid: true},
		Cities:      []string{"berlin"},
	}
}

// seedForRefresh ingests one open posting carrying the given cities and back-dates its liveness
// and change stamps, so a refresh that touches either is visible as a move rather than as two
// equal timestamps.
func seedForRefresh(t *testing.T, pool *pgxpool.Pool, q *Queries, cities []string) Job {
	t.Helper()
	ctx := context.Background()

	p := ingestParams("acme:1", "Engineer")
	p.ContentHash = pgtype.Text{String: "h1", Valid: true}
	p.Cities = cities
	if _, err := ingestUpsert(ctx, q, p); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET last_seen_at = now() - interval '10 days', updated_at = now() - interval '10 days'
		 WHERE source = 'greenhouse' AND external_id = 'acme:1'`,
	); err != nil {
		t.Fatalf("back-date stamps: %v", err)
	}

	got, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:1"})
	if err != nil {
		t.Fatalf("seed readback: %v", err)
	}
	return got
}

func readBack(t *testing.T, q *Queries) Job {
	t.Helper()
	got, err := q.GetJobBySourceExternalID(context.Background(),
		GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:1"})
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	return got
}

func TestRefreshUnchangedJobWritesOnlyLastSeenAt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	before := seedForRefresh(t, pool, q, []string{"berlin"})

	row, err := q.RefreshUnchangedJob(ctx, unchangedParams())
	if err != nil {
		t.Fatalf("RefreshUnchangedJob: %v", err)
	}
	// The narrow projection carries what the caller needs: the id for the enrichment enqueue and
	// the apply-form queue, source and company_slug for the crawled-set.
	if row.ID != before.ID || row.Source != "greenhouse" || row.CompanySlug != "acme" {
		t.Errorf("returned row = %+v, want id %d / greenhouse / acme", row, before.ID)
	}

	// Assert against the PERSISTED row, not the projection: what matters is what was written.
	got := readBack(t, q)
	if !got.LastSeenAt.Time.After(before.LastSeenAt.Time) {
		t.Errorf("last_seen_at = %v, want advanced from %v", got.LastSeenAt.Time, before.LastSeenAt.Time)
	}
	if time.Since(got.LastSeenAt.Time) > time.Minute {
		t.Errorf("last_seen_at = %v, want ~now", got.LastSeenAt.Time)
	}
	// The change stamp does NOT move, which is what makes updated_at mean "content changed".
	if !got.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Errorf("updated_at = %v, want unchanged %v", got.UpdatedAt.Time, before.UpdatedAt.Time)
	}

	// Nothing else moved either. Compared as whole rows with the one column the query is allowed
	// to write zeroed, so a column added to jobs later is covered without editing this.
	a, b := before, got
	a.LastSeenAt, b.LastSeenAt = pgtype.Timestamptz{}, pgtype.Timestamptz{}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("row changed beyond last_seen_at:\n before = %+v\n after  = %+v", a, b)
	}
}

// The highest-volume shape in production is a posting whose location pins no city at all, which
// reaches the cheap path with a nil slice against a stored '{}'. If array equality did not hold
// there, the optimisation would silently do nothing for most of the catalogue while every other
// test stayed green.
func TestRefreshUnchangedJobMatchesAPostingWithNoCities(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	before := seedForRefresh(t, pool, q, nil)

	p := unchangedParams()
	p.Cities = nil
	if _, err := q.RefreshUnchangedJob(ctx, p); err != nil {
		t.Fatalf("RefreshUnchangedJob with no cities: %v", err)
	}
	if got := readBack(t, q); !got.LastSeenAt.Time.After(before.LastSeenAt.Time) {
		t.Errorf("last_seen_at = %v, want advanced — a city-less posting must take the cheap path",
			got.LastSeenAt.Time)
	}
}

func TestRefreshUnchangedJobMatchesNothingWhenTheRowWouldChange(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	movedHash := unchangedParams()
	movedHash.ContentHash = pgtype.Text{String: "h2", Valid: true}

	movedCities := unchangedParams()
	movedCities.Cities = []string{"munich"}

	movedSalary := unchangedParams()
	movedSalary.SalaryMinSource = pgtype.Int4{Int32: 90000, Valid: true}

	absent := unchangedParams()
	absent.ExternalID = "acme:never-ingested"

	cases := map[string]struct {
		params   RefreshUnchangedJobParams
		closed   bool
		nullHash bool
	}{
		// A moved hash means the posting's indexed content changed: the full upsert must run.
		"content hash moved": {params: movedHash},
		// cities and the structured source salary are the columns the content hash does not
		// cover, so they are matched on separately — see
		// TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites.
		"structured cities moved": {params: movedCities},
		"structured salary moved": {params: movedSalary},
		// A closed row must reach UpsertJob, which is what reopens it. Refreshing its liveness
		// here would leave it closed forever while the unseen sweep kept seeing it.
		"row is closed": {params: unchangedParams(), closed: true},
		// A legacy row predating content_hash knows nothing about what it holds, so it cannot be
		// declared unchanged. NULL compares unequal to every value, which is the behaviour wanted.
		"stored hash is NULL": {params: unchangedParams(), nullHash: true},
		// A brand-new posting matches no row either, and wants the same fall-through — the caller
		// cannot and need not tell "changed" from "absent".
		"no such row": {params: absent},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			truncate(t, pool)
			seedForRefresh(t, pool, q, []string{"berlin"})
			if tc.closed {
				if _, err := pool.Exec(ctx,
					`UPDATE jobs SET closed_at = now() WHERE source = 'greenhouse' AND external_id = 'acme:1'`,
				); err != nil {
					t.Fatalf("close: %v", err)
				}
			}
			if tc.nullHash {
				if _, err := pool.Exec(ctx,
					`UPDATE jobs SET content_hash = NULL WHERE source = 'greenhouse' AND external_id = 'acme:1'`,
				); err != nil {
					t.Fatalf("null the hash: %v", err)
				}
			}

			if _, err := q.RefreshUnchangedJob(ctx, tc.params); !errors.Is(err, pgx.ErrNoRows) {
				t.Errorf("err = %v, want pgx.ErrNoRows so the caller falls through to UpsertJob", err)
			}
		})
	}
}

// The cheap write can only stay heap-only if jobs has room on a page for the new tuple version,
// which is what migration 0073's fillfactor buys. A migration present in the repo but not
// applied to the schema is invisible otherwise — the query still works, it just quietly
// maintains all 21 indexes on every refresh.
func TestJobsCarriesTheWriteStorageParameters(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var opts []string
	if err := pool.QueryRow(ctx,
		`SELECT reloptions FROM pg_class WHERE relname = 'jobs' AND relkind = 'r'`).Scan(&opts); err != nil {
		t.Fatalf("read jobs reloptions: %v", err)
	}

	for _, want := range []string{"fillfactor=90", "autovacuum_vacuum_scale_factor=0.02"} {
		if !slices.Contains(opts, want) {
			t.Errorf("jobs reloptions = %v, missing %q — migration 0073 did not reach this schema",
				opts, want)
		}
	}
}
