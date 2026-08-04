//go:build integration

// Integration tests for jdresolve.Resolver: the three input kinds (an existing job's
// slug, a URL, or pasted text) need a real Postgres — the private-job insert and the
// enrichment outbox both live in SQL. Run with: go test -tags=integration ./internal/jdresolve/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package jdresolve_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jdresolve"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/privatejob"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/testdb"
)

// pageClient is a test linksource.Client serving one canned HTML body for any URL —
// mirrors internal/linkimport's own test fixture.
type pageClient struct{ body string }

func (c pageClient) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return html.Parse(strings.NewReader(c.body))
}

func (c pageClient) GetHTMLResolved(_ context.Context, url string) (*html.Node, string, error) {
	n, err := html.Parse(strings.NewReader(c.body))
	return n, url, err
}

func (c pageClient) GetJSON(_ context.Context, url string, _ any) error {
	return nil
}

const jobPostingPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Staff Java Backend Developer","description":"Lead the backend guild.",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

const plainPage = `<html><head><title>Our team</title></head><body>No vacancy here.</body></html>`

// boardServing is a fake ingest adapter serving one posting on one board, so a test can
// reach a recognized-ATS resolution without a network — mirrors linkimport's own fixture.
type boardServing string

func (boardServing) Provider() string { return "recruitee" }

func (b boardServing) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID: "senior-go",
		URL:        "https://" + e.Board + ".recruitee.com/o/senior-go",
		Title:      string(b),
		Company:    "Acme",
	}}, nil
}

func fingerprintOf(title, companySlug, description string) string {
	return jobhash.RoleFingerprint(db.UpsertJobParams{
		Title: title, CompanySlug: companySlug, Description: description,
	})
}

// seedUser inserts a throwaway user and returns its id — created_by has a foreign key to
// users, so a private-job test needs a real row to attribute to.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestResolve_KnownJobSlugPassesThrough(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	var slug string
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
		VALUES ('greenhouse', 'acme:1', 'https://boards.greenhouse.io/acme/jobs/1',
		        'Senior Go Engineer', 'senior-go-acme', 'Acme', 'acme', $1)
		RETURNING public_slug`, fingerprintOf("Senior Go Engineer", "acme", "")).
		Scan(&slug); err != nil {
		t.Fatalf("seed existing job: %v", err)
	}

	r := jdresolve.New(q, linkimport.New(pool, q, nil, pageClient{}, nil, nil), nil)
	got, err := r.Resolve(ctx, 1, jdresolve.Request{JobSlug: slug})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != slug {
		t.Errorf("Resolve = %q, want the same slug %q", got, slug)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if rows != 1 {
		t.Errorf("catalog holds %d jobs, want 1 — passthrough must not create a row", rows)
	}
}

func TestResolve_UnknownJobSlugIsNotFound(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	r := jdresolve.New(q, linkimport.New(pool, q, nil, pageClient{}, nil, nil), nil)
	_, err := r.Resolve(ctx, 1, jdresolve.Request{JobSlug: "does-not-exist"})
	if !errors.Is(err, jdresolve.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestResolve_RecognizedATSURLBecomesAPublicJob(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	im := linkimport.New(pool, q, nil, pageClient{body: plainPage},
		map[string]sources.Source{"recruitee": boardServing("Senior Go Engineer")}, nil)
	r := jdresolve.New(q, im, nil)

	slug, err := r.Resolve(ctx, 1, jdresolve.Request{URL: "https://acme.recruitee.com/o/senior-go"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if slug == "" {
		t.Fatal("Resolve returned an empty slug")
	}

	var source string
	var isPrivate bool
	if err := pool.QueryRow(ctx, `SELECT source, is_private FROM jobs WHERE public_slug = $1`, slug).
		Scan(&source, &isPrivate); err != nil {
		t.Fatalf("read written job: %v", err)
	}
	if source != "recruitee" {
		t.Errorf("source = %q, want recruitee", source)
	}
	if isPrivate {
		t.Error("is_private = true, want false — a recognized-ATS resolution is a normal public job")
	}

	var queued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrichment_outbox o JOIN jobs j ON j.id = o.job_id
		WHERE j.public_slug = $1`, slug).Scan(&queued); err != nil {
		t.Fatalf("read enrichment queue: %v", err)
	}
	if queued == 0 {
		t.Error("a recognized-ATS job was not enqueued for enrichment")
	}
}

// Resubmitting a URL the catalog already carries under its (source, external_id) must
// resolve to the SAME job, not a duplicate — Write's UpsertJob dedups on that key, and
// resolveURL must surface that idempotency, not just "a" slug.
func TestResolve_RecognizedATSURLForAnAlreadyCarriedPostingDedups(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	im := linkimport.New(pool, q, nil, pageClient{body: plainPage},
		map[string]sources.Source{"recruitee": boardServing("Senior Go Engineer")}, nil)
	r := jdresolve.New(q, im, nil)
	url := "https://acme.recruitee.com/o/senior-go"

	first, err := r.Resolve(ctx, 1, jdresolve.Request{URL: url})
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	second, err := r.Resolve(ctx, 1, jdresolve.Request{URL: url})
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if second != first {
		t.Errorf("second Resolve = %q, want the same slug %q", second, first)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if rows != 1 {
		t.Errorf("catalog holds %d jobs, want 1 — re-resolving the same URL must not duplicate it", rows)
	}
}

func TestResolve_GenericURLBecomesAPrivateJob(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	im := linkimport.New(pool, q, nil, pageClient{body: jobPostingPage}, nil, nil)
	pw := privatejob.NewWriter(q)
	r := jdresolve.New(q, im, pw)
	userID := seedUser(t, pool, "generic-url@example.test")

	slug, err := r.Resolve(ctx, userID, jdresolve.Request{URL: "https://careers.mindera.test/jobs/1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var source string
	var isPrivate bool
	var createdBy int64
	if err := pool.QueryRow(ctx, `SELECT source, is_private, created_by FROM jobs WHERE public_slug = $1`, slug).
		Scan(&source, &isPrivate, &createdBy); err != nil {
		t.Fatalf("read written job: %v", err)
	}
	if source != "weblink" {
		t.Errorf("source = %q, want weblink", source)
	}
	if !isPrivate {
		t.Error("is_private = false, want true — a generic scrape must not join the public catalog")
	}
	if createdBy != userID {
		t.Errorf("created_by = %d, want %d", createdBy, userID)
	}

	var queued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrichment_outbox o JOIN jobs j ON j.id = o.job_id
		WHERE j.public_slug = $1`, slug).Scan(&queued); err != nil {
		t.Fatalf("read enrichment queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("enrichment queue holds %d rows for a private job, want 0", queued)
	}
}

func TestResolve_UnreadableURLCreatesNoRow(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	im := linkimport.New(pool, q, nil, pageClient{body: plainPage}, nil, nil)
	r := jdresolve.New(q, im, privatejob.NewWriter(q))

	_, err := r.Resolve(ctx, 1, jdresolve.Request{URL: "https://careers.mindera.test/about-us"})
	if !errors.Is(err, jdresolve.ErrUnreadableURL) {
		t.Errorf("err = %v, want ErrUnreadableURL", err)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if rows != 0 {
		t.Errorf("catalog holds %d jobs, want none", rows)
	}
}

func TestResolve_PastedTextBecomesAPrivateJob(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	r := jdresolve.New(q, linkimport.New(pool, q, nil, pageClient{}, nil, nil), privatejob.NewWriter(q))
	userID := seedUser(t, pool, "pasted-text@example.test")

	slug, err := r.Resolve(ctx, userID, jdresolve.Request{
		Text:  "We are hiring a backend engineer with Go experience.",
		Title: "Backend Engineer",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var source string
	var isPrivate bool
	if err := pool.QueryRow(ctx, `SELECT source, is_private FROM jobs WHERE public_slug = $1`, slug).
		Scan(&source, &isPrivate); err != nil {
		t.Fatalf("read written job: %v", err)
	}
	if source != "pasted" {
		t.Errorf("source = %q, want pasted", source)
	}
	if !isPrivate {
		t.Error("is_private = false, want true")
	}

	var queued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrichment_outbox o JOIN jobs j ON j.id = o.job_id
		WHERE j.public_slug = $1`, slug).Scan(&queued); err != nil {
		t.Fatalf("read enrichment queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("enrichment queue holds %d rows for a private job, want 0", queued)
	}
}

func TestResolve_SameTextFromTwoUsersCreatesTwoRows(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	r := jdresolve.New(q, linkimport.New(pool, q, nil, pageClient{}, nil, nil), privatejob.NewWriter(q))
	req := jdresolve.Request{Text: "Identical job description.", Title: "Engineer"}
	userA := seedUser(t, pool, "submitter-a@example.test")
	userB := seedUser(t, pool, "submitter-b@example.test")

	slugA, err := r.Resolve(ctx, userA, req)
	if err != nil {
		t.Fatalf("Resolve (user A): %v", err)
	}
	slugB, err := r.Resolve(ctx, userB, req)
	if err != nil {
		t.Fatalf("Resolve (user B): %v", err)
	}
	if slugA == slugB {
		t.Errorf("both submissions got slug %q, want distinct rows", slugA)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if rows != 2 {
		t.Errorf("catalog holds %d jobs, want 2", rows)
	}
}
