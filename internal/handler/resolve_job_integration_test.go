//go:build integration

// Integration tests for POST /api/v1/jobs/resolve, the endpoint behind the extension's
// "add this page to freehire". Three outcomes share one wire shape: a page we already
// carry answers found, a page an adapter parses is imported, and a page nothing can read
// is queued for manual triage. Run with: go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
)

// vacancyPage carries the schema.org block the generic resolver reads; aboutPage does not.
const vacancyPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Staff Java Backend Developer","description":"Lead the backend guild.",
 "datePosted":"2026-07-01","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

const aboutPage = `<html><head><title>About us</title></head><body>No vacancy here.</body></html>`

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
		PublicSlug *string `json:"public_slug"`
		Status     string  `json:"status"`
	} `json:"data"`
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
		"/about-us":                          aboutPage,
	}
	h := &contributionHandlers{
		queries:      queries,
		contribution: contribution.New(contribution.NewQueriesRepository(queries), nil),
		imports:      linkimport.New(pool, queries, nil, pages),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/resolve", keyAuth, h.ResolveJob)

	resolve := func(t *testing.T, url string, withCookie bool) (*http.Response, resolveReply) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"url": url})
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

	t.Run("a parseable page is imported", func(t *testing.T) {
		const page = "https://careers.mindera.test/jobs/staff-java-backend-developer"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "imported" || out.Data.PublicSlug == nil {
			t.Fatalf("body = %+v, want status imported with a slug", out.Data)
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
			`SELECT count(*) FROM link_contributions WHERE url = $1`, page).Scan(&queued); err != nil {
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
}
