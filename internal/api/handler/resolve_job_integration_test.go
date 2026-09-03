//go:build integration

// Integration tests for POST /api/v1/jobs/resolve, the one intake every surface enters
// through. Four outcomes share one wire shape: a page we already carry answers found; a
// vacancy on a board we crawl is imported and answered tracked; any other readable vacancy is
// imported and its board queued for onboarding; a page nothing can read is queued for triage.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
)

// vacancyPage carries the schema.org block the generic resolver reads; aboutPage does not.
const vacancyPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Staff Java Backend Developer","description":"Lead the backend guild.",
 "datePosted":"2026-07-01","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

// secondMinderaPage is another vacancy from a company the catalog already carries, on the same
// unrecognisable careers host — the case that used to be answered "this company is new to us".
const secondMinderaPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Principal Java Architect","description":"Shape the platform.",
 "datePosted":"2026-07-02","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

// storefrontPage is a vanity careers storefront over an ATS board: it carries a perfectly good
// JobPosting block, so the generic resolver reads it happily and would file it under the page's
// own URL — the duplicate a known board has to override.
const storefrontPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Frontend Product Software Engineer, Design Systems","description":"Design systems work.",
 "datePosted":"2026-07-20","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Globobox"}}
</script></head><body>Apply now</body></html>`

// nimbusPage is a storefront vacancy for a company whose posting the catalog already carries
// under a crawled source — the collapse case.
const nimbusPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Principal Java Architect","description":"Own the platform.",
 "datePosted":"2026-07-21","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Nimbus"}}
</script></head><body>Apply now</body></html>`

const aboutPage = `<html><head><title>About us</title></head><body>No vacancy here.</body></html>`

// fingerprintOf builds the role fingerprint the import derives, so a seeded canon lands in the
// same role cluster as the page. The description is part of it, not just title and company.
func fingerprintOf(title, companySlug, description string) string {
	return jobhash.RoleFingerprint(db.UpsertJobParams{
		Title: title, CompanySlug: companySlug, Description: description,
	})
}

// pagesClient is a test linksource.Client: it serves the body registered for the first
// path substring the requested URL contains, and errors for anything else.
type pagesClient map[string]string

func (c pagesClient) body(url string) (string, error) {
	for match, body := range c {
		if strings.Contains(url, match) {
			return body, nil
		}
	}
	return "", fmt.Errorf("pagesClient: no route for %s", url)
}

func (c pagesClient) GetHTML(_ context.Context, url string) (*html.Node, error) {
	b, err := c.body(url)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(b))
}

func (c pagesClient) GetHTMLResolved(_ context.Context, url string) (*html.Node, string, error) {
	b, err := c.body(url)
	if err != nil {
		return nil, "", err
	}
	n, err := html.Parse(strings.NewReader(b))
	return n, url, err
}

func (c pagesClient) GetJSON(_ context.Context, url string, _ any) error {
	return fmt.Errorf("pagesClient: no JSON route for %s", url)
}

type resolveReply struct {
	Data *struct {
		PublicSlug  *string `json:"public_slug"`
		Status      string  `json:"status"`
		CompanySlug string  `json:"company_slug"`
	} `json:"data"`
}

// fakeGreenhouse serves one greenhouse board, so a storefront link resolved to that board can
// be read from it. Its posting URL is the board's own, not the storefront's — which is exactly
// why the posting has to be matched by id.
type fakeGreenhouse struct{}

func (fakeGreenhouse) Provider() string { return "greenhouse" }

func (fakeGreenhouse) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	if e.Board != "globobox" {
		return nil, nil
	}
	return []sources.Job{{
		ExternalID: "7862086",
		URL:        "https://jobs.globobox.test/listing/7862086?gh_jid=7862086",
		Title:      "Frontend Product Software Engineer, Design Systems",
		Company:    "Globobox",
	}}, nil
}

// fakeRecruitee is a stand-in ingest adapter for a platform with no single-page link adapter,
// so the intake reaches it through board coverage — the path most pasted links now take.
type fakeRecruitee struct{}

func (fakeRecruitee) Provider() string { return "recruitee" }

func (fakeRecruitee) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID: "222",
		URL:        "https://" + e.Board + ".recruitee.com/o/senior-go",
		Title:      "Senior Go Engineer",
		Company:    strings.ToTitle(e.Board),
	}}, nil
}

func TestResolveJobEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('importer@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// A posting we already carry, reachable by its stored URL.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('himalayas', ':https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer',
		         'https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer',
		         'Staff Java Backend Developer', 'staff-java-mindera')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)
	pages := pagesClient{
		"/jobs/staff-java-backend-developer": vacancyPage,
		"/jobs/principal-java-architect":     secondMinderaPage,
		"/jobs/7862086":                      storefrontPage,
		"/jobs/nimbus-principal-architect":   nimbusPage,
		"/about-us":                          aboutPage,
	}
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), nil)
	h := &contributionHandlers{
		contribution: contributionSvc,
		intake: &intakeService{
			queries:      queries,
			contribution: contributionSvc,
			imports: linkimport.New(pool, queries, nil, pages,
				map[string]sources.Source{"recruitee": fakeRecruitee{}, "greenhouse": fakeGreenhouse{}}, nil),
		},
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/resolve", keyAuth, h.ResolveJob)

	resolve := func(t *testing.T, url string, withCookie bool) (*http.Response, resolveReply) {
		t.Helper()
		// Every surface tags its intakes; the extension is the one this test stands in for.
		body, _ := json.Marshal(map[string]string{"url": url, "surface": contribution.SurfaceExtension})
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/jobs/resolve", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if withCookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("resolve %s: %v", url, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out resolveReply
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &out)
		}
		return resp, out
	}

	t.Run("unauthenticated is refused", func(t *testing.T) {
		resp, _ := resolve(t, "https://careers.mindera.test/jobs/staff-java-backend-developer", false)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("a page we already carry is found, not imported", func(t *testing.T) {
		resp, out := resolve(t,
			"https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer?utm_source=freehire.me", true)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "found" {
			t.Fatalf("body = %+v, want status found", out.Data)
		}
		if out.Data.PublicSlug == nil || *out.Data.PublicSlug != "staff-java-mindera" {
			t.Errorf("slug = %v, want staff-java-mindera", out.Data.PublicSlug)
		}
	})

	t.Run("a parseable page on an unrecognised host is imported and filed for review", func(t *testing.T) {
		// No board can be derived from a company's own careers domain, so nothing is queued for
		// onboarding — answering "imported" here would promise a crawl that never starts.
		const page = "https://careers.mindera.test/jobs/staff-java-backend-developer"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "review" || out.Data.PublicSlug == nil {
			t.Fatalf("body = %+v, want status review with a slug", out.Data)
		}
		if out.Data.CompanySlug != "" {
			t.Errorf("company_slug = %q, want empty — nothing else of Mindera's is in the catalog",
				out.Data.CompanySlug)
		}
		var stored string
		if err := pool.QueryRow(ctx,
			`SELECT source FROM jobs WHERE public_slug = $1`, *out.Data.PublicSlug).Scan(&stored); err != nil {
			t.Fatalf("read imported posting: %v", err)
		}
		if stored != "weblink" {
			t.Errorf("source = %q, want weblink", stored)
		}

		// Submitting it again resolves to the same posting, without a second row.
		_, again := resolve(t, page, true)
		if again.Data == nil || again.Data.PublicSlug == nil || *again.Data.PublicSlug != *out.Data.PublicSlug {
			t.Errorf("resubmit = %+v, want the same slug", again.Data)
		}
		var rows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE source = 'weblink'`).Scan(&rows); err != nil {
			t.Fatalf("count imports: %v", err)
		}
		if rows != 1 {
			t.Errorf("catalog holds %d imported postings, want 1", rows)
		}
	})

	t.Run("a storefront link lands on the board it fronts, not as a second posting", func(t *testing.T) {
		// The shape that started this: a vanity storefront over a Greenhouse board we already
		// crawl (dropbox.jobs over greenhouse/dropbox). Its host names no board and its path puts
		// the greenhouse job id BEFORE a readable slug, so every URL parse misses it — but the
		// catalogue already holds that id, which is what resolves the board. The posting must
		// update the row we crawl rather than appear a second time under (weblink, <the URL>).
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug)
			 VALUES ('greenhouse', 'globobox:7862086', 'https://jobs.globobox.test/listing/7862086',
			         'Frontend Product Software Engineer, Design Systems', 'frontend-design-systems-globobox',
			         'Globobox', 'globobox')`); err != nil {
			t.Fatalf("seed the crawled greenhouse posting: %v", err)
		}
		const page = "https://www.globobox.jobs/en/jobs/7862086/frontend-product-software-engineer-design-systems/"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "tracked" {
			t.Fatalf("body = %+v, want status tracked — we crawl the board behind that storefront", out.Data)
		}
		if out.Data.CompanySlug != "globobox" {
			t.Errorf("company_slug = %q, want globobox", out.Data.CompanySlug)
		}
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM jobs WHERE source = 'weblink' AND external_id = $1`, page).Scan(&rows); err != nil {
			t.Fatalf("count weblink duplicates: %v", err)
		}
		if rows != 0 {
			t.Errorf("the storefront link wrote %d weblink rows, want 0 — it duplicates the crawled posting", rows)
		}
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM jobs WHERE company_slug = 'globobox'`).Scan(&rows); err != nil {
			t.Fatalf("count globobox postings: %v", err)
		}
		if rows != 1 {
			t.Errorf("catalog holds %d Globobox postings, want 1 — the import must land on the crawled row", rows)
		}
	})

	t.Run("a vacancy we already carry answers found, and still records the board", func(t *testing.T) {
		// A storefront over an ATS we do not recognise, fronting a company whose posting the
		// catalog already holds. The vacancy is not new, so the answer is found — but the board
		// behind the storefront may still be worth onboarding, so the contribution must be
		// recorded before that answer is given.
		if _, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
			VALUES ('greenhouse', 'nimbus:41', 'https://boards.greenhouse.io/nimbus/jobs/41',
			        'Principal Java Architect', 'principal-java-nimbus', 'Nimbus', 'nimbus', $1)`,
			fingerprintOf("Principal Java Architect", "nimbus", "Own the platform.")); err != nil {
			t.Fatalf("seed the crawled posting: %v", err)
		}
		const page = "https://careers.nimbus.test/jobs/nimbus-principal-architect"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 — the catalog already carries this vacancy", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "found" {
			t.Fatalf("body = %+v, want status found", out.Data)
		}
		if out.Data.PublicSlug == nil || *out.Data.PublicSlug != "principal-java-nimbus" {
			t.Errorf("slug = %v, want the canonical principal-java-nimbus", out.Data.PublicSlug)
		}
		var recorded int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM board_submissions WHERE url = $1`, page).Scan(&recorded); err != nil {
			t.Fatalf("read the contribution queue: %v", err)
		}
		if recorded != 1 {
			t.Errorf("contribution rows = %d, want 1 — a found vacancy does not excuse losing the board", recorded)
		}

		// The premise that makes writing-and-marking better than skipping the write: the
		// storefront URL now resolves, and it resolves to the posting we already had.
		_, again := resolve(t, page, true)
		if again.Data == nil || again.Data.Status != "found" {
			t.Fatalf("resubmit = %+v, want status found", again.Data)
		}
		if again.Data.PublicSlug == nil || *again.Data.PublicSlug != "principal-java-nimbus" {
			t.Errorf("resubmit slug = %v, want the canonical principal-java-nimbus", again.Data.PublicSlug)
		}
	})

	t.Run("a company we already carry is not called new", func(t *testing.T) {
		// Whether we can name the BOARD and whether we know the COMPANY are separate questions:
		// an earlier subtest put a Mindera posting in the catalog, so this second vacancy from
		// the same company must come back naming it, however unrecognisable its host is.
		resp, out := resolve(t, "https://careers.mindera.test/jobs/principal-java-architect", true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "review" {
			t.Fatalf("body = %+v, want status review", out.Data)
		}
		if out.Data.CompanySlug != "mindera" {
			t.Errorf("company_slug = %q, want mindera — the catalog already carries this company",
				out.Data.CompanySlug)
		}
	})

	t.Run("an unreadable page is queued for triage", func(t *testing.T) {
		const page = "https://careers.mindera.test/about-us"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "queued" || out.Data.PublicSlug != nil {
			t.Fatalf("body = %+v, want status queued with a null slug", out.Data)
		}
		var queued int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM board_submissions WHERE url = $1`, page).Scan(&queued); err != nil {
			t.Fatalf("read triage queue: %v", err)
		}
		if queued != 1 {
			t.Errorf("triage queue holds %d rows for the page, want 1", queued)
		}
	})

	t.Run("a non-URL is rejected", func(t *testing.T) {
		resp, _ := resolve(t, "not a url", true)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("importing a vacancy also queues its board for onboarding", func(t *testing.T) {
		// The whole point of the change: reading one posting must not hide the board it came
		// from, or the other vacancies on it stay invisible to us forever.
		const page = "https://globex.recruitee.com/o/senior-go"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "imported" || out.Data.PublicSlug == nil {
			t.Fatalf("body = %+v, want status imported with a slug", out.Data)
		}
		var status, surface string
		if err := pool.QueryRow(ctx,
			`SELECT status, surface FROM boards WHERE provider = 'recruitee' AND board = 'globex'`).
			Scan(&status, &surface); err != nil {
			t.Fatalf("the imported vacancy's board was not queued: %v", err)
		}
		if status != contribution.StatusPending {
			t.Errorf("queued board status = %q, want pending", status)
		}
		if surface != contribution.SurfaceExtension {
			t.Errorf("surface = %q, want %q — an intake must say which door it came through",
				surface, contribution.SurfaceExtension)
		}
		// The posting carries the identity the ingest crawl of that board would give it, so a
		// later crawl dedups onto this row.
		var source, externalID string
		if err := pool.QueryRow(ctx,
			`SELECT source, external_id FROM jobs WHERE public_slug = $1`, *out.Data.PublicSlug).
			Scan(&source, &externalID); err != nil {
			t.Fatalf("read imported posting: %v", err)
		}
		if source != "recruitee" || externalID != "globex:222" {
			t.Errorf("identity = (%q, %q), want (recruitee, globex:222)", source, externalID)
		}
	})

	t.Run("a vacancy on a board we already crawl is tracked, not queued", func(t *testing.T) {
		// The company is already being crawled; this posting simply has not landed yet. Import
		// it now and say so, rather than asking the user to wait for a board we cover.
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug)
			 VALUES ('recruitee', 'acme:111', 'https://acme.recruitee.com/o/junior-go',
			         'Junior Go', 'junior-go-acme', 'Acme', 'acme')`); err != nil {
			t.Fatalf("seed crawled board: %v", err)
		}
		resp, out := resolve(t, "https://acme.recruitee.com/o/senior-go", true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "tracked" {
			t.Fatalf("body = %+v, want status tracked", out.Data)
		}
		if out.Data.PublicSlug == nil {
			t.Error("tracked answer carries no slug, want the imported posting")
		}
		if out.Data.CompanySlug != "acme" {
			t.Errorf("company_slug = %q, want acme — the caller is told who is already covered", out.Data.CompanySlug)
		}
		var queued int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM boards WHERE provider = 'recruitee' AND board = 'acme'`).
			Scan(&queued); err != nil {
			t.Fatalf("read triage queue: %v", err)
		}
		if queued != 0 {
			t.Errorf("queued %d rows for an already-crawled board, want 0 — it needs no onboarding", queued)
		}
	})
}
