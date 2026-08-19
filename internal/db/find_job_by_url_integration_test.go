//go:build integration

// Integration tests for resolving a job page URL to the catalog posting stored under it —
// the second tier of /api/v1/jobs/find, used when no source identity can be read out of
// the URL. Both the normalization (an IMMUTABLE SQL function shared by the query and its
// index) and the open/canonical gate are SQL behaviors verifiable only against a real
// Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// urlJob builds an open posting that stores the given detail URL.
func urlJob(externalID, url string) UpsertJobParams {
	p := ingestParams(externalID, "Staff Java Backend Developer")
	p.Source = "himalayas"
	p.URL = url
	return p
}

// findByURL runs the lookup under test.
func findByURL(t *testing.T, q *Queries, url string) (string, error) {
	t.Helper()
	return q.FindOpenJobByURL(context.Background(), url)
}

const himalayasPosting = "https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer"

func TestNormalizeJobURL_CollapsesCosmeticVariants(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// Every variant of the same posting address must normalize alike: scheme, host case,
	// a www. prefix, a tracking query, a fragment, and trailing slashes are all noise.
	variants := []string{
		himalayasPosting,
		"http://himalayas.app/companies/mindera/jobs/staff-java-backend-developer",
		"https://www.himalayas.app/companies/mindera/jobs/staff-java-backend-developer",
		"https://Himalayas.app/companies/mindera/jobs/Staff-Java-Backend-Developer",
		himalayasPosting + "?utm_source=freehire.me",
		himalayasPosting + "#apply",
		himalayasPosting + "///",
	}
	var want string
	if err := pool.QueryRow(ctx, "SELECT normalize_job_url($1)", himalayasPosting).Scan(&want); err != nil {
		t.Fatalf("normalize base URL: %v", err)
	}
	if strings.HasPrefix(want, "http") {
		t.Errorf("normalize_job_url kept the scheme: %q", want)
	}
	for _, v := range variants {
		var got string
		if err := pool.QueryRow(ctx, "SELECT normalize_job_url($1)", v).Scan(&got); err != nil {
			t.Fatalf("normalize %q: %v", v, err)
		}
		if got != want {
			t.Errorf("normalize_job_url(%q) = %q, want %q", v, got, want)
		}
	}

	// A genuinely different posting must not collapse into the same key.
	var other string
	if err := pool.QueryRow(ctx,
		"SELECT normalize_job_url($1)", "https://himalayas.app/companies/mindera/jobs/staff-go-developer").Scan(&other); err != nil {
		t.Fatalf("normalize sibling URL: %v", err)
	}
	if other == want {
		t.Errorf("two different postings normalized alike: %q", other)
	}
}

func TestFindOpenJobByURL_ResolvesDespiteURLNoise(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, urlJob("himalayas:mindera", himalayasPosting))

	// The link the user followed off freehire carries our own tracking tag; the stored
	// URL does not. They are the same himalayasPosting.
	for _, requested := range []string{
		himalayasPosting,
		himalayasPosting + "?utm_source=freehire.me",
		"https://www.himalayas.app/companies/mindera/jobs/staff-java-backend-developer/",
		"http://himalayas.app/companies/mindera/jobs/staff-java-backend-developer",
	} {
		slug, err := findByURL(t, q, requested)
		if err != nil {
			t.Errorf("find %q: %v", requested, err)
			continue
		}
		if slug != "pslug-himalayas:mindera" {
			t.Errorf("find %q = %q, want the seeded himalayasPosting", requested, slug)
		}
	}
}

func TestFindOpenJobByURL_SkipsClosedAndFollowsDuplicates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("closed", func(t *testing.T) {
		truncate(t, pool)
		mustUpsert(t, q, urlJob("himalayas:closed", himalayasPosting))
		if _, err := pool.Exec(ctx,
			"UPDATE jobs SET closed_at = now() WHERE external_id = $1", "himalayas:closed"); err != nil {
			t.Fatalf("close himalayasPosting: %v", err)
		}
		if slug, err := findByURL(t, q, himalayasPosting); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("closed posting resolved to %q (err %v), want no rows", slug, err)
		}
	})

	// A duplicate is not "nothing here". The candidate is standing on a page whose
	// vacancy the catalogue holds; the dedup passes merely picked a twin to represent
	// the group. Answer with that twin — one in five open postings is a duplicate, so
	// the alternative is telling every fifth page it is unknown.
	t.Run("duplicate resolves to what it duplicates", func(t *testing.T) {
		truncate(t, pool)
		mustUpsert(t, q, urlJob("himalayas:canonical", "https://himalayas.app/companies/mindera/jobs/other"))
		mustUpsert(t, q, urlJob("himalayas:dup", himalayasPosting))
		canonicalID, _ := dupOf(t, pool, "himalayas:canonical")
		const canonicalSlug = "pslug-himalayas:canonical"
		if _, err := pool.Exec(ctx,
			"UPDATE jobs SET duplicate_of_role = $1 WHERE external_id = $2", canonicalID, "himalayas:dup"); err != nil {
			t.Fatalf("suppress himalayasPosting: %v", err)
		}
		slug, err := findByURL(t, q, himalayasPosting)
		if err != nil {
			t.Fatalf("duplicate page: %v", err)
		}
		if slug != canonicalSlug {
			t.Errorf("duplicate page resolved to %q, want the canonical %q", slug, canonicalSlug)
		}
	})

	// Unless the representative is gone. Then the open row in hand is the best answer
	// there is, and reporting nothing would hide a vacancy that is still live.
	t.Run("duplicate of a closed canonical answers with itself", func(t *testing.T) {
		truncate(t, pool)
		mustUpsert(t, q, urlJob("himalayas:canonical", "https://himalayas.app/companies/mindera/jobs/other"))
		mustUpsert(t, q, urlJob("himalayas:dup", himalayasPosting))
		canonicalID, _ := dupOf(t, pool, "himalayas:canonical")
		if _, err := pool.Exec(ctx,
			"UPDATE jobs SET duplicate_of_role = $1 WHERE external_id = $2", canonicalID, "himalayas:dup"); err != nil {
			t.Fatalf("suppress himalayasPosting: %v", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", canonicalID); err != nil {
			t.Fatalf("close canonical: %v", err)
		}
		slug, err := findByURL(t, q, himalayasPosting)
		if err != nil {
			t.Fatalf("duplicate page: %v", err)
		}
		if slug == "" || slug == "pslug-himalayas:canonical" {
			t.Errorf("resolved to %q, want the duplicate's own slug", slug)
		}
	})
}

func TestFindOpenJobByURL_UnknownPageHasNoRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, urlJob("himalayas:mindera", himalayasPosting))

	if slug, err := findByURL(t, q, "https://example.test/careers/some-job"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown page resolved to %q (err %v), want no rows", slug, err)
	}
}

func TestFindOpenJobByURL_PrefersTheMostRecentlySeenPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two open canonical rows can legitimately link to the same page (an aggregator and
	// an ATS row that the dedup passes have not collapsed). The card shows the row we
	// confirmed most recently.
	mustUpsert(t, q, urlJob("himalayas:stale", himalayasPosting))
	mustUpsert(t, q, urlJob("himalayas:fresh", himalayasPosting))
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET last_seen_at = now() - interval '10 days' WHERE external_id = $1", "himalayas:stale"); err != nil {
		t.Fatalf("age the stale row: %v", err)
	}

	slug, err := findByURL(t, q, himalayasPosting)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if slug != "pslug-himalayas:fresh" {
		t.Errorf("find = %q, want the most recently seen himalayasPosting", slug)
	}
}

func TestFindOpenJobByURL_UsesTheExpressionIndex(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, urlJob("himalayas:mindera", himalayasPosting))
	// The planner picks a sequential scan on a table this small no matter what, so ask it
	// to cost the index path: what this asserts is that the query's expression matches the
	// index's, which is the property that silently breaks if the two definitions drift.
	if _, err := pool.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatalf("disable seqscan: %v", err)
	}

	// EXPLAIN the generated query itself, not a copy of it — a copy would keep passing
	// after the real query changed.
	rows, err := pool.Query(ctx, "EXPLAIN "+findOpenJobByURL, himalayasPosting)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	// The open-rows index (0051), not the canonical-only one (0042): this lookup now
	// matches duplicates too. Without an index it is a sequential scan of every open
	// posting, which is what this endpoint was rewritten to escape.
	if !strings.Contains(plan.String(), "jobs_normalized_url_open_idx") {
		t.Errorf("plan does not use jobs_normalized_url_open_idx:\n%s", plan.String())
	}
}
