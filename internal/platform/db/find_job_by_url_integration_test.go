//go:build integration

// Integration tests for resolving a job page URL to the catalog posting stored under it —
// the second tier of /api/v1/jobs/find, used when no source identity can be read out of
// the URL. Both the normalization (an IMMUTABLE SQL function shared by the query and its
// index) and the open/canonical gate are SQL behaviors verifiable only against a real
// Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
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
	// The stale row is inserted SECOND, so it has the higher id: the id tiebreak favours it and only
	// last_seen_at (job_activity's) can put the fresh row first.
	mustUpsert(t, q, urlJob("himalayas:fresh", himalayasPosting))
	mustUpsert(t, q, urlJob("himalayas:stale", himalayasPosting))
	if _, err := pool.Exec(ctx, `UPDATE job_activity a SET last_seen_at = now() - interval '10 days'
		FROM jobs j WHERE j.id = a.job_id AND j.external_id = $1`, "himalayas:stale"); err != nil {
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
	// With one posting, scanning another open-row index and filtering its only
	// tuple costs the same as a URL lookup. Give the planner a selective URL in
	// an analysed catalogue, so this checks the actual lookup path without a GUC.
	if _, err := pool.Exec(ctx, `INSERT INTO jobs
		(source, external_id, url, title, public_slug)
		SELECT 'test', 'url-index-' || n,
		       'https://example.test/postings/' || n, 'Engineer', 'url-index-' || n
		FROM generate_series(1, 2000) n`); err != nil {
		t.Fatalf("seed nonmatching postings: %v", err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE jobs; ANALYZE job_activity"); err != nil {
		t.Fatalf("analyse lookup relations: %v", err)
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

// A link that normalizes to nothing must not answer with whichever open posting happens
// to store no URL (an employer vacancy taking applications on freehire).
func TestFindOpenJobByURL_EmptyNeverMatches(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := ingestUpsert(ctx, q, urlJob("freehire:0b6f", "")); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	for _, in := range []string{"", "https://www."} {
		if _, err := findByURL(t, q, in); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("FindOpenJobByURL(%q) err = %v, want no rows", in, err)
		}
	}
}

// normalize runs normalize_job_url in the database.
func normalize(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, raw string) string {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(), "SELECT normalize_job_url($1)", raw).Scan(&got); err != nil {
		t.Fatalf("normalize %q: %v", raw, err)
	}
	return got
}

// The posting freehire#4821 reported: a CGI vacancy whose stored url is a Njoyn page, where
// the query — not the path — names the posting.
const njoynPosting = "https://cgi.njoyn.com/corp/xweb/xweb.asp?clid=21001&page=jobdetails&jobid=J1026-1962&BRID=1342736&utm_source=freehire.me"

func TestNormalizeJobURL_KeepsTheQueryThatNamesThePosting(t *testing.T) {
	pool := startPostgres(t)
	want := normalize(t, pool, njoynPosting)
	if want != "cgi.njoyn.com/corp/xweb/xweb.asp?brid=1342736&clid=21001&jobid=j1026-1962&page=jobdetails" {
		t.Errorf("normalize_job_url(njoyn) = %q", want)
	}

	// Parameter order, case, tracking tags, a fragment, www. and the scheme are noise.
	for _, v := range []string{
		"http://www.cgi.njoyn.com/CORP/xweb/xweb.asp?Jobid=J1026-1962&Page=JobDetails&CLID=21001&BRID=1342736",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp?gclid=abc&clid=21001&page=jobdetails&jobid=J1026-1962&BRID=1342736&fbclid=x#apply",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp/?clid=21001&page=jobdetails&jobid=J1026-1962&BRID=1342736&utm_campaign=y&",
	} {
		if got := normalize(t, pool, v); got != want {
			t.Errorf("normalize_job_url(%q) = %q, want %q", v, got, want)
		}
	}

	// A different posting, the listing, and the bare script are different pages.
	for _, v := range []string{
		"https://cgi.njoyn.com/corp/xweb/xweb.asp?clid=21001&page=jobdetails&jobid=J1126-1603&BRID=1342736",
		"https://cgi.njoyn.com/CORP/xweb/xweb.asp?CLID=21001&page=joblisting",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp",
	} {
		if got := normalize(t, pool, v); got == want {
			t.Errorf("normalize_job_url(%q) collapsed into the posting's key %q", v, got)
		}
	}

	// A query of nothing but tracking tags leaves no query at all.
	if got, bare := normalize(t, pool, himalayasPosting+"?utm_source=a&ref=b&gclid=c"), normalize(t, pool, himalayasPosting); got != bare || strings.Contains(got, "?") {
		t.Errorf("tracking-only query normalized to %q, want %q", got, bare)
	}
}

func TestFindOpenJobByURL_AnswersOnlyThePageTheQueryNames(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	p := urlJob("wttj:cgi-fullstack", njoynPosting)
	p.Source = "wttj"
	mustUpsert(t, q, p)

	slug, err := findByURL(t, q, "https://cgi.njoyn.com/CORP/xweb/xweb.asp?Jobid=J1026-1962&Page=JobDetails&CLID=21001&BRID=1342736")
	if err != nil || slug != "pslug-wttj:cgi-fullstack" {
		t.Errorf("own page resolved to %q (err %v), want the posting", slug, err)
	}
	for _, other := range []string{
		"https://cgi.njoyn.com/CORP/xweb/xweb.asp?CLID=21001&page=joblisting",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp?NTKN=c&clid=21001&Page=JobDetails&Jobid=J1126-1603&BRID=1350470&lang=1",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp",
	} {
		if slug, err := findByURL(t, q, other); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("find %q = %q (err %v), want no rows", other, slug, err)
		}
	}
}

func TestNormalizeJobURL_QueryEdgeCases(t *testing.T) {
	pool := startPostgres(t)
	bare := normalize(t, pool, himalayasPosting)
	for raw, want := range map[string]string{
		himalayasPosting + "?":                        bare,
		himalayasPosting + "?&&":                      bare,
		himalayasPosting + "#top?jobid=1":             bare,
		himalayasPosting + "?ref":                     bare,
		himalayasPosting + "?utmx=1":                  bare + "?utmx=1",
		himalayasPosting + "?b=2&a=1&a=0":             bare + "?a=0&a=1&b=2",
		himalayasPosting + "?jobid=7&amp;page=detail": bare + "?jobid=7&page=detail",
		"https://www.":                                "",
	} {
		if got := normalize(t, pool, raw); got != want {
			t.Errorf("normalize_job_url(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A kept query can be long; the key must still fit a B-tree entry, or the rebuild and every
// write of the posting fail. Oversized keys are hashed, and equality stays exact.
func TestNormalizeJobURL_BoundsALongKey(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	long := himalayasPosting + "?token=" + strings.Repeat("ab12", 3000)
	key := normalize(t, pool, long)
	if len(key) > 2000 {
		t.Errorf("key is %d bytes, want at most 2000", len(key))
	}
	if normalize(t, pool, long+"&utm_source=x") != key {
		t.Error("a tracking tag changed a bounded key")
	}
	if normalize(t, pool, long+"0") == key {
		t.Error("two different long URLs share a bounded key")
	}
	// Cyrillic is two bytes a character: the bound is in bytes, not characters.
	if k := normalize(t, pool, "https://hh.ru/vacancy?q="+strings.Repeat("я", 3000)); len(k) > 2000 {
		t.Errorf("multibyte key is %d bytes, want at most 2000", len(k))
	}

	// The posting is writable through every index over the function, and still found.
	mustUpsert(t, q, urlJob("himalayas:long", long))
	if slug, err := findByURL(t, q, long); err != nil || slug != "pslug-himalayas:long" {
		t.Errorf("long URL resolved to %q (err %v)", slug, err)
	}
}
