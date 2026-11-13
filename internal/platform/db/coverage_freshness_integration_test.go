//go:build integration

// Integration tests for the ingest-time aggregator coverage gate's lookup: which of the asked
// companies still have RECENT coverage from a non-aggregator source. Every condition it tests
// is a SQL behaviour — the freshness cutoff, the aggregator exclusion, the closed-row
// exclusion, and the folded-slug match — so none of them can be verified without a real
// Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// aggregatorSources is the shape the caller passes: the provider keys ingest classifies as
// aggregators. Coverage may never come from one of these, however fresh.
var aggregatorSources = []string{"himalayas", "remoteok"}

func TestCompaniesWithFreshNonAggregatorCoverage(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	// seed writes one posting. slug is stored as written and folded the way UpsertJob folds
	// it, so the rows match what production holds rather than a shape only the test produces.
	seed := func(t *testing.T, source, externalID, slug string, lastSeen time.Time, closed bool) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  company_slug_folded, last_seen_at, closed_at)
			VALUES ($1, $2, 'https://x.test/'||$2, 'Engineer', $2, 'Co', $3,
			        replace($3, '-', ''), $4,
			        CASE WHEN $5::bool THEN now() ELSE NULL END)`,
			source, externalID, slug, lastSeen, closed)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
	}

	now := time.Now()
	cutoff := now.Add(-14 * 24 * time.Hour)

	ask := func(t *testing.T, folded ...string) map[string]bool {
		t.Helper()
		rows, err := q.CompaniesWithFreshNonAggregatorCoverage(ctx, CompaniesWithFreshNonAggregatorCoverageParams{
			FoldedCompanies: folded,
			Aggregators:     aggregatorSources,
			SeenAfter:       pgtype.Timestamptz{Time: cutoff, Valid: true},
		})
		if err != nil {
			t.Fatalf("CompaniesWithFreshNonAggregatorCoverage: %v", err)
		}
		got := make(map[string]bool, len(rows))
		for _, r := range rows {
			got[r] = true
		}
		return got
	}

	// seedPrivate writes a jd-tailor-intake posting: one user's pasted job description,
	// visible only to them and crawled from nowhere.
	seedPrivate := func(t *testing.T, externalID, slug string) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  company_slug_folded, last_seen_at, is_private)
			VALUES ('weblink', $1, 'https://x.test/'||$1, 'Engineer', $1, 'Co', $2,
			        replace($2, '-', ''), now(), true)`, externalID, slug)
		if err != nil {
			t.Fatalf("seed private %s: %v", externalID, err)
		}
	}

	// Every row is seeded before any subtest runs, so no subtest depends on another's writes
	// and each can be run alone with -run.
	seed(t, "greenhouse", "fresh:1", "freshco", now.Add(-time.Hour), false)
	seed(t, "trakstar", "stale:1", "staleco", now.Add(-31*24*time.Hour), false)
	seed(t, "himalayas", "agg:1", "aggonly", now, false)
	seed(t, "greenhouse", "closed:1", "closedco", now, true)
	seed(t, "lever", "fold:1", "cfoinsights", now, false)
	seedPrivate(t, "private:1", "privateco")

	t.Run("a recently seen non-aggregator posting is coverage", func(t *testing.T) {
		if !ask(t, "freshco")["freshco"] {
			t.Error("a posting seen an hour ago must count as coverage")
		}
	})

	t.Run("a posting unseen past the cutoff is NOT coverage", func(t *testing.T) {
		// The reported defect: a 2013 trakstar posting last seen 31 days ago held the slug
		// "pipe" covered and discarded every live aggregator posting for a different employer
		// of the same name. Coverage is a claim about the present.
		if ask(t, "staleco")["staleco"] {
			t.Error("a posting unseen for 31 days must not count as coverage")
		}
	})

	t.Run("an aggregator posting is never coverage, however fresh", func(t *testing.T) {
		if ask(t, "aggonly")["aggonly"] {
			t.Error("an aggregator posting must not cover its own company")
		}
	})

	t.Run("a closed posting is not coverage", func(t *testing.T) {
		if ask(t, "closedco")["closedco"] {
			t.Error("a closed posting must not count as coverage")
		}
	})

	t.Run("hyphenation does not decide the match", func(t *testing.T) {
		// The employer the ATS writes "cfoinsights" and the aggregator writes "cfo-insights"
		// is one employer. The caller folds before asking, so both spellings arrive as one
		// value and meet the stored fold.
		if !ask(t, "cfoinsights")["cfoinsights"] {
			t.Error("the folded spelling must match the stored fold")
		}
	})

	t.Run("a company with no posting at all is absent from the answer", func(t *testing.T) {
		got := ask(t, "freshco", "nobodyco")
		if got["nobodyco"] {
			t.Error("a company with no rows must not be reported as covered")
		}
		if !got["freshco"] {
			t.Error("asking about several companies must still answer for the covered one")
		}
	})

	t.Run("a private posting is not coverage, however fresh", func(t *testing.T) {
		// The jd-tailor-intake path: a job description one user pasted in, visible only to
		// them and never crawled. It cannot be evidence that the catalogue still crawls the
		// employer — and if it were, one user's pasted JD for "Acme" would silently discard
		// every aggregator posting for every other Acme.
		//
		// The search-backed lookup this replaced excluded these by accident, because
		// cmd/reindex drops is_private rows from the index. Reading the table directly makes
		// that an exclusion the query has to state, which is why it is pinned here.
		if ask(t, "privateco")["privateco"] {
			t.Error("a private posting must never count as coverage")
		}
	})

	t.Run("the index the query's plan depends on exists in a migrated database", func(t *testing.T) {
		// Not a performance test — a presence test, because the absence is SILENT. This index
		// lived only in a comment in migration 0109 for months (an operator built it on prod
		// by hand), so every result above was correct while the query seq-scanned ~7.4M rows.
		// cmd/ingest now runs it once per board run, which is where that stops being cheap.
		var def string
		err := pool.QueryRow(ctx,
			`SELECT indexdef FROM pg_indexes WHERE indexname = 'jobs_open_company_slug_folded_col_idx'`).Scan(&def)
		if err != nil {
			t.Fatalf("jobs_open_company_slug_folded_col_idx is missing from a freshly migrated "+
				"database — the query above falls back to a sequential scan: %v", err)
		}
		if !strings.Contains(def, "company_slug_folded") {
			t.Errorf("indexdef = %q, want an index on the company_slug_folded column", def)
		}
	})

	t.Run("the answer never carries a company nobody asked about", func(t *testing.T) {
		// The caller keys its result map by what it asked; a stray key would be a coverage
		// claim about a company nobody enquired about.
		for slug := range ask(t, "freshco") {
			if slug != "freshco" {
				t.Errorf("answer carries %q, which was not asked about", slug)
			}
		}
	})
}

// TestCompaniesWithFreshNonAggregatorCoverageByCountry pins the national-market scope: asked
// with countries, only a posting hiring in one of them, or stating none, covers its company —
// the rule SuppressAggregatorDuplicatesForCompanies matches twins by. Country-blind, the UK
// Arriva's Workday board discarded the Slovenian Arriva's mojedelo postings (issue #4412).
func TestCompaniesWithFreshNonAggregatorCoverageByCountry(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(t *testing.T, externalID, slug string, countries []string) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  company_slug_folded, last_seen_at, countries)
			VALUES ('workday', $1, 'https://x.test/'||$1, 'Engineer', $1, 'Co', $2, $2, now(), $3)`,
			externalID, slug, countries)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
	}
	seed(t, "gb:1", "arriva", []string{"gb"})
	seed(t, "si:1", "akrapovic", []string{"si"})
	seed(t, "many:1", "jysk", []string{"de", "si"})
	seed(t, "none:1", "litostroj", []string{})

	ask := func(t *testing.T, countries []string) map[string]bool {
		t.Helper()
		rows, err := q.CompaniesWithFreshNonAggregatorCoverage(ctx, CompaniesWithFreshNonAggregatorCoverageParams{
			FoldedCompanies: []string{"arriva", "akrapovic", "jysk", "litostroj"},
			Aggregators:     aggregatorSources,
			SeenAfter:       pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Countries:       countries,
		})
		if err != nil {
			t.Fatalf("CompaniesWithFreshNonAggregatorCoverage: %v", err)
		}
		got := make(map[string]bool, len(rows))
		for _, r := range rows {
			got[r] = true
		}
		return got
	}

	scoped := ask(t, []string{"si"})
	for slug, want := range map[string]bool{"arriva": false, "akrapovic": true, "jysk": true, "litostroj": true} {
		if scoped[slug] != want {
			t.Errorf("scoped to si: %s covered = %v, want %v", slug, scoped[slug], want)
		}
	}
	// nil is what an unscoped aggregator passes: the gate stays country-blind.
	for _, slug := range []string{"arriva", "akrapovic", "jysk", "litostroj"} {
		if !ask(t, nil)[slug] {
			t.Errorf("unscoped: %s must be covered", slug)
		}
	}
}
