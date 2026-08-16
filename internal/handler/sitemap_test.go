package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

// stubSitemapIndex stands in for the Meilisearch-backed sitemapLister. Both
// sitemaps read a search index rather than Postgres, so every endpoint here is
// testable without either — the engine's only contributions are a page and a total.
type stubSitemapIndex struct {
	docs []search.SitemapDocument
	err  error
	// lastOffset/lastLimit record what the handler asked for, so a test can assert
	// the query parameters reached the index rather than only the response shape.
	lastOffset, lastLimit int
}

func (s *stubSitemapIndex) ListSitemapPage(_ context.Context, offset, limit int) ([]search.SitemapDocument, int64, error) {
	s.lastOffset, s.lastLimit = offset, limit
	if s.err != nil {
		return nil, 0, s.err
	}
	total := int64(len(s.docs))
	if offset >= len(s.docs) {
		return nil, total, nil
	}
	return s.docs[offset:min(offset+limit, len(s.docs))], total, nil
}

func (s *stubSitemapIndex) CountSitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := s.ListSitemapPage(ctx, 0, 1)
	return total, err
}

func stubDocs(n int) []search.SitemapDocument {
	docs := make([]search.SitemapDocument, n)
	for i := range docs {
		docs[i] = search.SitemapDocument{
			Slug:      "job-" + string(rune('a'+i%26)),
			UpdatedAt: time.Unix(int64(1700000000+i), 0).UTC(),
		}
	}
	return docs
}

// newSitemapTestApp mounts the sitemap routes through the handler's own register,
// so the route ORDER under test is the one production wires — see
// TestSitemapRoutesAreNotShadowedBySlug.
func newSitemapTestApp(jobs sitemapLister) *fiber.App {
	return newSitemapTestAppWith(jobs, jobs)
}

func newSitemapTestAppWith(jobs, companies sitemapLister) *fiber.App {
	h := &sitemapHandlers{jobs: jobs, companies: companies}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h.register(api)
	return app
}

func sitemapGet(t *testing.T, app *fiber.App, url string) (int, []byte) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", url, nil))
	if err != nil {
		t.Fatalf("request %q: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func decodeOffsets(t *testing.T, body []byte) []int64 {
	t.Helper()
	var d struct {
		Data []int64 `json:"data"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return d.Data
}

func TestJobSitemapBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		total int
		query string
		want  []int64
	}{
		// Every page gets an offset INCLUDING the first, so the sitemap index lists
		// the cursors exactly as they come — no opening cursor to prepend.
		{"exact multiple", 6, "?chunk=2", []int64{0, 2, 4}},
		// A partial trailing page still needs its own file, or its jobs never get crawled.
		{"partial trailing page", 7, "?chunk=2", []int64{0, 2, 4, 6}},
		{"one short page", 1, "?chunk=2", []int64{0}},
		// No documents means no sub-sitemaps — not one empty file.
		{"empty index", 0, "?chunk=2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSitemapTestApp(&stubSitemapIndex{docs: stubDocs(tt.total)})
			status, body := sitemapGet(t, app, "/api/v1/jobs/sitemap/boundaries"+tt.query)
			if status != fiber.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", status, body)
			}
			got := decodeOffsets(t, body)
			if len(got) != len(tt.want) {
				t.Fatalf("offsets = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("offsets = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestJobSitemapPaging(t *testing.T) {
	idx := &stubSitemapIndex{docs: stubDocs(5)}
	app := newSitemapTestApp(idx)

	var d struct {
		Data []struct {
			Slug      string `json:"slug"`
			UpdatedAt string `json:"updated_at"`
		} `json:"data"`
	}
	status, body := sitemapGet(t, app, "/api/v1/jobs/sitemap?offset=2&limit=2")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if idx.lastOffset != 2 || idx.lastLimit != 2 {
		t.Fatalf("index asked for offset=%d limit=%d, want 2/2", idx.lastOffset, idx.lastLimit)
	}
	if len(d.Data) != 2 || d.Data[0].Slug != idx.docs[2].Slug {
		t.Fatalf("page = %+v, want the two documents at offset 2", d.Data)
	}
	if d.Data[0].UpdatedAt == "" {
		t.Fatalf("entry missing updated_at: %+v", d.Data[0])
	}
}

// A crawler holding a stale sitemap index asks for an offset the index has since
// shrunk past. It must get a valid, empty page rather than an error — an error
// there would be reported as a broken sitemap for the whole site.
func TestJobSitemapToleratesStaleAndJunkOffsets(t *testing.T) {
	app := newSitemapTestApp(&stubSitemapIndex{docs: stubDocs(3)})
	for _, url := range []string{
		"/api/v1/jobs/sitemap?offset=9999",
		"/api/v1/jobs/sitemap?offset=not-a-number",
		"/api/v1/jobs/sitemap?offset=-5",
		// Past int32: the overflow pageParamsBounded exists to clamp.
		"/api/v1/jobs/sitemap?offset=3000000000",
	} {
		status, body := sitemapGet(t, app, url)
		if status != fiber.StatusOK {
			t.Fatalf("status = %d for %q, want 200 (body %s)", status, url, body)
		}
	}
}

// The limit is clamped to the sitemap protocol's per-file cap however it is asked
// for, so an untrusted ?limit= can never make the server build an invalid file.
func TestJobSitemapClampsLimit(t *testing.T) {
	idx := &stubSitemapIndex{docs: stubDocs(1)}
	app := newSitemapTestApp(idx)
	if status, body := sitemapGet(t, app, "/api/v1/jobs/sitemap?limit=999999"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}
	if idx.lastLimit != sitemapMaxURLs {
		t.Fatalf("limit = %d, want it clamped to %d", idx.lastLimit, sitemapMaxURLs)
	}
}

// With no search engine configured the job sitemap has no source at all, so it must
// say so rather than serve an empty sitemap that reads as "this site has no jobs".
func TestJobSitemapWithoutSearchEngine(t *testing.T) {
	app := newSitemapTestApp(nil)
	for _, url := range []string{"/api/v1/jobs/sitemap", "/api/v1/jobs/sitemap/boundaries"} {
		if status, _ := sitemapGet(t, app, url); status != fiber.StatusServiceUnavailable {
			t.Fatalf("status = %d for %q, want 503", status, url)
		}
	}
}

// A company indexed before updated_at joined the document shape has no lastmod.
// The field must be ABSENT from the response, not present as the year-1 zero
// instant — the SPA only omits <lastmod> for a falsy value, so a zero time would
// ship as <lastmod>0001-01-01T00:00:00Z</lastmod> to every crawler.
func TestSitemapOmitsAnAbsentLastmod(t *testing.T) {
	app := newSitemapTestApp(&stubSitemapIndex{docs: []search.SitemapDocument{{Slug: "acme"}}})
	_, body := sitemapGet(t, app, "/api/v1/jobs/sitemap")
	if strings.Contains(string(body), "updated_at") {
		t.Fatalf("response carries an updated_at for a document that has none: %s", body)
	}
	if !strings.Contains(string(body), "acme") {
		t.Fatalf("the entry itself must still be served: %s", body)
	}
}

func TestJobSitemapPropagatesIndexFailure(t *testing.T) {
	app := newSitemapTestApp(&stubSitemapIndex{err: errors.New("engine down")})
	if status, _ := sitemapGet(t, app, "/api/v1/jobs/sitemap"); status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

// The two sitemaps read DIFFERENT indexes, and nothing in their shared shape would
// catch them being wired to the same one — the endpoints would answer, just with the
// wrong catalogue. Distinct stubs are what makes that visible.
func TestSitemapHalvesReadTheirOwnIndex(t *testing.T) {
	jobs := &stubSitemapIndex{docs: stubDocs(4)}
	companies := &stubSitemapIndex{docs: stubDocs(9)}
	app := newSitemapTestAppWith(jobs, companies)

	// Chunk sizes are the endpoints' own defaults, so the offsets also prove each
	// half applied its own constant rather than the other's.
	_, body := sitemapGet(t, app, "/api/v1/jobs/sitemap/boundaries?chunk=2")
	if got := decodeOffsets(t, body); len(got) != 2 {
		t.Fatalf("job offsets = %v, want 2 for the 4-document jobs index", got)
	}
	_, body = sitemapGet(t, app, "/api/v1/companies/sitemap/boundaries?chunk=2")
	if got := decodeOffsets(t, body); len(got) != 5 {
		t.Fatalf("company offsets = %v, want 5 for the 9-document companies index", got)
	}

	if _, body = sitemapGet(t, app, "/api/v1/companies/sitemap?offset=8"); true {
		var d struct {
			Data []struct {
				Slug string `json:"slug"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &d); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		if len(d.Data) != 1 {
			t.Fatalf("company tail page = %+v, want the single 9th document", d.Data)
		}
	}
}

// Both sitemap paths sit under a segment that also has a :slug catch-all. If the
// catch-all were registered first, "/jobs/sitemap" would resolve as the job with
// slug "sitemap" and the whole sitemap would 404 — which is why register() puts the
// literals ahead of it, and why this asserts through register() rather than mounting
// the handlers itself.
func TestSitemapRoutesAreNotShadowedBySlug(t *testing.T) {
	h := &sitemapHandlers{jobs: &stubSitemapIndex{docs: stubDocs(3)}, companies: &stubSitemapIndex{docs: stubDocs(3)}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h.register(api)
	// The catch-alls production registers after the sitemap literals. Answering 418
	// makes "the slug route swallowed it" unmistakable in a failure.
	catchAll := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusTeapot) }
	api.Get("/jobs/:slug", catchAll)
	api.Get("/companies/:slug", catchAll)

	for _, url := range []string{
		"/api/v1/jobs/sitemap",
		"/api/v1/jobs/sitemap/boundaries",
		"/api/v1/companies/sitemap",
		"/api/v1/companies/sitemap/boundaries",
	} {
		if status, body := sitemapGet(t, app, url); status != fiber.StatusOK {
			t.Fatalf("status = %d for %q, want 200 — the :slug route swallowed it (body %s)", status, url, body)
		}
	}
}
