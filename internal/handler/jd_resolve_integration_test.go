//go:build integration

// Integration tests for POST /api/v1/me/jd/resolve — the entry point that turns an
// existing job's slug, an external URL, or pasted JD text into a job usable by the tailor
// workspace. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jdresolve"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/privatejob"
)

const jdVacancyPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Staff Java Backend Developer","description":"Lead the backend guild.",
 "hiringOrganization":{"@type":"Organization","name":"Mindera"}}
</script></head><body>Apply now</body></html>`

const jdAboutPage = `<html><head><title>About us</title></head><body>No vacancy here.</body></html>`

type jdPageClient struct{ body string }

func (c jdPageClient) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return html.Parse(strings.NewReader(c.body))
}

func (c jdPageClient) GetHTMLResolved(_ context.Context, url string) (*html.Node, string, error) {
	n, err := html.Parse(strings.NewReader(c.body))
	return n, url, err
}

func (c jdPageClient) GetJSON(_ context.Context, _ string, _ any) error { return nil }

// failingPageClient simulates a network/fetch failure (timeout, DNS error, non-2xx) rather
// than "the page carries no vacancy" — the distinction resolveURL must map to the same 422
// as an unreadable page, not a 500.
type failingPageClient struct{}

func (failingPageClient) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return nil, errFakeNetwork
}

func (failingPageClient) GetHTMLResolved(_ context.Context, _ string) (*html.Node, string, error) {
	return nil, "", errFakeNetwork
}

func (failingPageClient) GetJSON(_ context.Context, _ string, _ any) error { return errFakeNetwork }

var errFakeNetwork = errors.New("fake network failure")

func TestJDResolveEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('jd-resolver@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var seededSlug string
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug)
		VALUES ('greenhouse', 'acme:1', 'https://boards.greenhouse.io/acme/jobs/1',
		        'Senior Go Engineer', 'senior-go-acme', 'Acme', 'acme')
		RETURNING public_slug`).Scan(&seededSlug); err != nil {
		t.Fatalf("seed existing job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)

	im := linkimport.New(pool, queries, nil, jdPageClient{body: jdVacancyPage}, nil, nil)
	resolver := jdresolve.New(queries, im, privatejob.NewWriter(queries))
	h := newJDResolveHandlers(resolver)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss, testVersions)
	app.Post("/api/v1/me/jd/resolve", cookieAuth, jdURLLimiter(), h.Resolve)

	post := func(t *testing.T, body map[string]string, withCookie bool) (*http.Response, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/jd/resolve", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if withCookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		var parsed map[string]any
		if len(out) > 0 {
			_ = json.Unmarshal(out, &parsed)
		}
		return resp, parsed
	}

	t.Run("known job_slug passes through", func(t *testing.T) {
		resp, body := post(t, map[string]string{"job_slug": seededSlug}, true)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].(map[string]any)
		if data["job_slug"] != seededSlug {
			t.Errorf("job_slug = %v, want %q", data["job_slug"], seededSlug)
		}
	})

	t.Run("a URL only the generic scrape reads becomes a private job", func(t *testing.T) {
		// A smoke test of the wire shape end-to-end (route -> handler -> resolver -> DB);
		// the recognized-vs-generic branching itself is covered exhaustively by
		// internal/jdresolve's own tests, which exercise a recognized-ATS match too.
		resp, body := post(t, map[string]string{"url": "https://careers.mindera.test/jobs/1"}, true)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].(map[string]any)
		slug, _ := data["job_slug"].(string)
		if slug == "" {
			t.Fatal("job_slug is empty")
		}
		var isPrivate bool
		if err := pool.QueryRow(ctx, `SELECT is_private FROM jobs WHERE public_slug = $1`, slug).Scan(&isPrivate); err != nil {
			t.Fatalf("read written job: %v", err)
		}
		if !isPrivate {
			t.Error("is_private = false, want true — a generic scrape must not join the public catalog")
		}
	})

	t.Run("pasted text becomes a private job", func(t *testing.T) {
		resp, body := post(t, map[string]string{"text": "We are hiring a backend engineer.", "title": "Backend Engineer"}, true)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].(map[string]any)
		if data["job_slug"] == "" || data["job_slug"] == nil {
			t.Error("job_slug is empty")
		}
	})

	t.Run("no input is a 400", func(t *testing.T) {
		resp, _ := post(t, map[string]string{}, true)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("both url and text is a 400", func(t *testing.T) {
		resp, _ := post(t, map[string]string{"url": "https://example.test/x", "text": "hi"}, true)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("unauthenticated is a 401", func(t *testing.T) {
		resp, _ := post(t, map[string]string{"job_slug": seededSlug}, false)
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("unreadable URL is a 422", func(t *testing.T) {
		unreadableIM := linkimport.New(pool, queries, nil, jdPageClient{body: jdAboutPage}, nil, nil)
		unreadableResolver := jdresolve.New(queries, unreadableIM, privatejob.NewWriter(queries))
		unreadableH := newJDResolveHandlers(unreadableResolver)
		unreadableApp := fiber.New(fiber.Config{ErrorHandler: RenderError})
		unreadableApp.Post("/api/v1/me/jd/resolve", cookieAuth, unreadableH.Resolve)

		raw, _ := json.Marshal(map[string]string{"url": "https://careers.mindera.test/about-us"})
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/jd/resolve", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := unreadableApp.Test(r)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("a network failure resolving the URL is a 422, not a 500", func(t *testing.T) {
		failingIM := linkimport.New(pool, queries, nil, failingPageClient{}, nil, nil)
		failingResolver := jdresolve.New(queries, failingIM, privatejob.NewWriter(queries))
		failingH := newJDResolveHandlers(failingResolver)
		failingApp := fiber.New(fiber.Config{ErrorHandler: RenderError})
		failingApp.Post("/api/v1/me/jd/resolve", cookieAuth, failingH.Resolve)

		raw, _ := json.Marshal(map[string]string{"url": "https://careers.mindera.test/unreachable"})
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/jd/resolve", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := failingApp.Test(r)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422 — a fetch failure is caller input, not a server fault", resp.StatusCode)
		}
	})

	t.Run("job_slug and text requests never spend the url rate limit", func(t *testing.T) {
		// jdURLLimiterPerHour (== contributionsPerHour, 20) would reject request #21 if
		// job_slug/text shared the url branch's budget. None of these touch the network, so
		// none should ever be limited.
		for i := 0; i < jdURLLimiterPerHour+5; i++ {
			resp, _ := post(t, map[string]string{"job_slug": seededSlug}, true)
			if resp.StatusCode == fiber.StatusTooManyRequests {
				t.Fatalf("request %d: got 429, want job_slug requests to never be url-rate-limited", i+1)
			}
		}
	})

	t.Run("url requests are still rate limited on their own budget", func(t *testing.T) {
		var limited bool
		for i := 0; i < jdURLLimiterPerHour+1; i++ {
			resp, _ := post(t, map[string]string{"url": "https://careers.mindera.test/many"}, true)
			if resp.StatusCode == fiber.StatusTooManyRequests {
				limited = true
				break
			}
		}
		if !limited {
			t.Errorf("%d url requests never hit 429, want the url branch's own budget enforced", jdURLLimiterPerHour+1)
		}
	})
}
