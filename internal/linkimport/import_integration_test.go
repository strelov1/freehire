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
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
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

	im := New(pool, q, nil, pageClient{body: jobPostingPage})

	res, ok, err := im.Import(ctx, pageURL)
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

	im := New(pool, q, nil, pageClient{body: jobPostingPage})

	first, _, err := im.Import(ctx, pageURL)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, ok, err := im.Import(ctx, pageURL)
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

	im := New(pool, q, nil, pageClient{body: plainPage})

	res, ok, err := im.Import(ctx, "https://careers.mindera.test/about-us")
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
