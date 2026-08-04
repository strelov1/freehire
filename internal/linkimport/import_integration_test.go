//go:build integration

// Integration tests for importing one vacancy from its page URL: the link-source registry
// parses the page, and the vacancy is written through the canonical job write path with an
// enrichment enqueue. Both halves need a real Postgres (slug minting and the outbox live
// in SQL). Run with: go test -tags=integration ./internal/linkimport/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package linkimport

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/sources"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// pageClient is a test linksource.Client serving one canned HTML body for any URL.
type pageClient struct{ body string }

func (c pageClient) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return html.Parse(strings.NewReader(c.body))
}

func (c pageClient) GetHTMLResolved(_ context.Context, url string) (*html.Node, string, error) {
	n, err := html.Parse(strings.NewReader(c.body))
	return n, url, err
}

func (c pageClient) GetJSON(_ context.Context, url string, _ any) error {
	return fmt.Errorf("pageClient: no JSON route for %s", url)
}

// jobPostingPage is a server-rendered page carrying the schema.org block the generic
// resolver reads — the shape of a careers page no board feed enumerates.
const jobPostingPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Staff Java Backend Developer","description":"Lead the backend guild.",
 "datePosted":"2026-07-01","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

const plainPage = `<html><head><title>Our team</title></head><body>No vacancy here.</body></html>`

const pageURL = "https://careers.mindera.test/jobs/staff-java-backend-developer"

func TestImport_WritesTheParsedVacancy(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	im := New(pool, q, nil, pageClient{body: jobPostingPage}, nil, nil)

	res, ok, err := im.Import(ctx, pageURL, Board{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !ok {
		t.Fatal("import reported nothing parsed, want the vacancy written")
	}
	if res.PublicSlug == "" {
		t.Error("imported posting has no public slug")
	}

	var source, externalID, title string
	if err := pool.QueryRow(ctx,
		`SELECT source, external_id, title FROM jobs WHERE public_slug = $1`, res.PublicSlug).
		Scan(&source, &externalID, &title); err != nil {
		t.Fatalf("read written posting: %v", err)
	}
	if source != "weblink" {
		t.Errorf("source = %q, want weblink", source)
	}
	if externalID != pageURL {
		t.Errorf("external_id = %q, want the page URL", externalID)
	}
	if title != "Staff Java Backend Developer" {
		t.Errorf("title = %q, want the posting's title", title)
	}

	// The import must join the enrichment outbox, like every other write path.
	var queued int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM enrichment_outbox o
		 JOIN jobs j ON j.id = o.job_id WHERE j.public_slug = $1`, res.PublicSlug).Scan(&queued); err != nil {
		t.Fatalf("read enrichment queue: %v", err)
	}
	if queued == 0 {
		t.Error("imported posting was not enqueued for enrichment")
	}
}

func TestImport_IsIdempotentForTheSameURL(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	im := New(pool, q, nil, pageClient{body: jobPostingPage}, nil, nil)

	first, _, err := im.Import(ctx, pageURL, Board{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, ok, err := im.Import(ctx, pageURL, Board{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if !ok || second.PublicSlug != first.PublicSlug {
		t.Errorf("second import = %q (ok=%v), want the same posting %q", second.PublicSlug, ok, first.PublicSlug)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count postings: %v", err)
	}
	if rows != 1 {
		t.Errorf("catalog holds %d postings, want 1", rows)
	}
}

func TestImport_ReportsAPageThatIsNotAVacancy(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	im := New(pool, q, nil, pageClient{body: plainPage}, nil, nil)

	res, ok, err := im.Import(ctx, "https://careers.mindera.test/about-us", Board{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if ok {
		t.Errorf("import reported %q written, want nothing parsed", res.PublicSlug)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count postings: %v", err)
	}
	if rows != 0 {
		t.Errorf("catalog holds %d postings, want none", rows)
	}
}

// Resolve is Import's resolution half without the write — the seam a caller needs when it
// must decide how to persist a match differently depending on which adapter matched (see
// internal/jdresolve, which writes a generic-fallback match as a private job instead of the
// public catalog write Import always performs).
func TestResolve_DoesNotWriteAnything(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	im := New(pool, q, nil, pageClient{body: jobPostingPage}, nil, nil)

	resolved, ok, err := im.Resolve(ctx, pageURL, Board{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ok {
		t.Fatal("resolve reported nothing parsed, want the vacancy resolved")
	}
	if resolved.Source != linksource.GenericSource {
		t.Errorf("Source = %q, want %q", resolved.Source, linksource.GenericSource)
	}
	if resolved.Job.Title != "Staff Java Backend Developer" {
		t.Errorf("Job.Title = %q, want the posting's title", resolved.Job.Title)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
		t.Fatalf("count postings: %v", err)
	}
	if rows != 0 {
		t.Errorf("catalog holds %d postings after Resolve, want none — Resolve must not write", rows)
	}
}

// fingerprintOf builds the role fingerprint write() derives for a posting, so a seeded row
// lands in the same role cluster as the imported page. The DESCRIPTION is part of it, not just
// the title and company — two postings of one role whose descriptions differ do not cluster.
// Derived rather than hardcoded so the test follows the definition if it moves; markup
// differences are fine, since the fingerprint normalizes to visible text.
func fingerprintOf(title, companySlug, description string) string {
	return jobhash.RoleFingerprint(db.UpsertJobParams{
		Title: title, CompanySlug: companySlug, Description: description,
	})
}

func TestImport_CollapsesOntoAPostingTheCatalogAlreadyCarries(t *testing.T) {
	// A storefront over an ATS board we crawl. The page parses, so the generic resolver would
	// file it under (weblink, <the URL>) — a second row for a vacancy the catalog already holds.
	// It must still be written (that row is what makes the storefront URL resolvable), but
	// marked a duplicate of the crawled row and kept out of the enrichment queue.
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	var canonID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
		VALUES ('greenhouse', 'mindera:1', 'https://boards.greenhouse.io/mindera/jobs/1',
		        'Staff Java Backend Developer', 'staff-java-mindera', 'Mindera', 'mindera', $1)
		RETURNING id`, fingerprintOf("Staff Java Backend Developer", "mindera", "Lead the backend guild.")).
		Scan(&canonID); err != nil {
		t.Fatalf("seed the crawled posting: %v", err)
	}

	im := New(pool, q, nil, pageClient{body: jobPostingPage}, nil, nil)

	res, ok, err := im.Import(ctx, pageURL, Board{})
	if err != nil || !ok {
		t.Fatalf("import = (ok %v, err %v), want the vacancy imported", ok, err)
	}
	if !res.Deduped {
		t.Error("Deduped = false, want true — the catalog already carries this vacancy")
	}
	if res.PublicSlug != "staff-java-mindera" {
		t.Errorf("PublicSlug = %q, want the canonical staff-java-mindera", res.PublicSlug)
	}

	var dupOf *int64
	if err := pool.QueryRow(ctx,
		`SELECT duplicate_of FROM jobs WHERE source = 'weblink' AND external_id = $1`, pageURL).
		Scan(&dupOf); err != nil {
		t.Fatalf("read the written row: %v", err)
	}
	if dupOf == nil || *dupOf != canonID {
		t.Errorf("duplicate_of = %v, want %d", dupOf, canonID)
	}

	var queued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrichment_outbox o
		JOIN jobs j ON j.id = o.job_id
		WHERE j.source = 'weblink'`).Scan(&queued); err != nil {
		t.Fatalf("read enrichment queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("enrichment queue holds %d rows for the duplicate, want 0 — it never reaches search", queued)
	}
}

func TestImport_ABoardIdentityIsNeverCollapsed(t *testing.T) {
	// A posting written under a board's own identity is deduplicated by
	// UpsertJob's ON CONFLICT (source, external_id). Running the role-cluster check on it too
	// would demote a crawled posting to a duplicate of a hand-imported one.
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
		VALUES ('weblink', 'https://careers.acme.test/go', 'https://careers.acme.test/go',
		        'Senior Go Engineer', 'senior-go-acme-web', 'Acme', 'acme', $1)`,
		fingerprintOf("Senior Go Engineer", "acme", "")); err != nil {
		t.Fatalf("seed the earlier import: %v", err)
	}

	im := New(pool, q, nil, pageClient{body: plainPage},
		map[string]sources.Source{"recruitee": boardServing("Senior Go Engineer")}, nil)

	res, ok, err := im.Import(ctx, "https://acme.recruitee.com/o/senior-go", Board{})
	if err != nil || !ok {
		t.Fatalf("import = (ok %v, err %v), want the board posting written", ok, err)
	}
	if res.Deduped {
		t.Error("Deduped = true for a board identity, want false — it dedups on (source, external_id)")
	}
	var dupOf *int64
	if err := pool.QueryRow(ctx,
		`SELECT duplicate_of FROM jobs WHERE source = 'recruitee'`).Scan(&dupOf); err != nil {
		t.Fatalf("read the board posting: %v", err)
	}
	if dupOf != nil {
		t.Errorf("duplicate_of = %d, want NULL — a crawled posting is not demoted by an import", *dupOf)
	}
}

// boardServing is an ingest adapter serving one posting on one board, so an import can reach a
// board identity without a network.
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
