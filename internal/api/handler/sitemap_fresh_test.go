package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

// stubFreshSitemapIndex is deliberately a SEPARATE stub from stubSitemapIndex rather
// than a method bolted onto it: the fresh route and the paged route read the same
// index through different engine endpoints, and a shared stub could not tell a
// handler that called the wrong one.
type stubFreshSitemapIndex struct {
	docs                  []search.SitemapDocument
	lastOffset, lastLimit int
	calls                 int
}

func (s *stubFreshSitemapIndex) ListFreshSitemapPage(_ context.Context, offset, limit int) ([]search.SitemapDocument, error) {
	s.calls++
	s.lastOffset, s.lastLimit = offset, limit
	if offset >= len(s.docs) {
		return nil, nil
	}
	return s.docs[offset:min(offset+limit, len(s.docs))], nil
}

func newFreshSitemapTestApp(fresh freshSitemapLister, paged sitemapLister) *fiber.App {
	h := &sitemapHandlers{jobs: paged, companies: paged, freshJobs: fresh}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h.register(api, middleware{})
	return app
}

func freshSitemapEntries(t *testing.T, app *fiber.App, target string) []sitemapEntry {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", target, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Data []sitemapEntry `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v (%s)", target, err, body)
	}
	return out.Data
}

// The fresh route must read the FRESH reader. Wiring it to the paged one would ship a
// sub-sitemap that looks right in every shape assertion and carries the arbitrary
// order this whole change exists to fix.
func TestFreshJobSitemapReadsTheFreshIndex(t *testing.T) {
	fresh := &stubFreshSitemapIndex{docs: stubDocs(3)}
	paged := &stubSitemapIndex{docs: stubDocs(3)}

	if got := freshSitemapEntries(t, newFreshSitemapTestApp(fresh, paged), "/api/v1/jobs/sitemap/fresh"); len(got) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(got))
	}
	if fresh.calls != 1 {
		t.Errorf("fresh reader called %d times, want 1", fresh.calls)
	}
	if paged.lastLimit != 0 {
		t.Errorf("paged reader was called (lastLimit=%d), want untouched", paged.lastLimit)
	}
}

// The handler must preserve the reader's order exactly. A handler that sorted,
// grouped or reversed would be a second place the ordering guarantee could be lost,
// after the reader has already secured it from the engine.
func TestFreshJobSitemapPreservesOrder(t *testing.T) {
	docs := stubDocs(4)
	fresh := &stubFreshSitemapIndex{docs: docs}

	got := freshSitemapEntries(t, newFreshSitemapTestApp(fresh, &stubSitemapIndex{}), "/api/v1/jobs/sitemap/fresh")
	if len(got) != len(docs) {
		t.Fatalf("len(entries) = %d, want %d", len(got), len(docs))
	}
	for i := range docs {
		if got[i].Slug != docs[i].Slug {
			t.Fatalf("entry %d = %q, want %q — order not preserved", i, got[i].Slug, docs[i].Slug)
		}
	}
}

// An offset past the fresh window's bound is an empty page, not an error: a crawler
// holding a stale sitemap index must never be answered with a failure, and the bound
// is also what keeps a hand-written offset off an unmeasured depth of the index.
func TestFreshJobSitemapRefusesToPageBeyondTheWindow(t *testing.T) {
	fresh := &stubFreshSitemapIndex{docs: stubDocs(5)}
	app := newFreshSitemapTestApp(fresh, &stubSitemapIndex{})

	if got := freshSitemapEntries(t, app, "/api/v1/jobs/sitemap/fresh?offset=500000"); len(got) != 0 {
		t.Fatalf("len(entries) = %d, want 0 past the window", len(got))
	}
	if fresh.calls != 0 {
		t.Errorf("fresh reader called %d times for an out-of-window offset, want 0", fresh.calls)
	}
}

// The bound must hold against the LIMIT too, not only the offset. pageParamsBounded
// clamps ?limit= to sitemapMaxURLs (50,000), so a guard that reads only the offset
// leaves the depth wide open through the front door: ?offset=30000&limit=50000 asks
// the engine for hit 80,000 of a sorted read — 2.7x past the depth the bound exists
// to stop at, on a public route, well inside the shared 600/min budget.
func TestFreshJobSitemapBoundsTheLimitToo(t *testing.T) {
	fresh := &stubFreshSitemapIndex{docs: stubDocs(5)}
	app := newFreshSitemapTestApp(fresh, &stubSitemapIndex{})

	_ = freshSitemapEntries(t, app, "/api/v1/jobs/sitemap/fresh?offset=0&limit=50000")
	if fresh.lastLimit > jobSitemapChunk {
		t.Errorf("limit reached the reader as %d, want at most %d", fresh.lastLimit, jobSitemapChunk)
	}
}

// The deepest a fresh read may ever reach is the bound plus one page. Asserting the
// REACH rather than the two parameters separately is what makes the guarantee
// checkable as one number, which is how the comment beside the constant states it.
func TestFreshJobSitemapNeverReachesPastTheWindow(t *testing.T) {
	fresh := &stubFreshSitemapIndex{docs: stubDocs(5)}
	app := newFreshSitemapTestApp(fresh, &stubSitemapIndex{})

	for _, target := range []string{
		"/api/v1/jobs/sitemap/fresh?offset=30000&limit=50000",
		"/api/v1/jobs/sitemap/fresh?offset=30000",
		"/api/v1/jobs/sitemap/fresh?offset=29999&limit=50000",
	} {
		fresh.lastOffset, fresh.lastLimit = 0, 0
		_ = freshSitemapEntries(t, app, target)
		if reach := fresh.lastOffset + fresh.lastLimit; reach > freshSitemapMaxOffset+jobSitemapChunk {
			t.Errorf("%s reached hit %d, want at most %d", target, reach, freshSitemapMaxOffset+jobSitemapChunk)
		}
	}
}

// The last in-window offset must still be served. An off-by-one here silently drops
// the oldest quarter of the fresh window, which no shape assertion would catch.
func TestFreshJobSitemapServesTheLastInWindowOffset(t *testing.T) {
	fresh := &stubFreshSitemapIndex{docs: stubDocs(3)}
	app := newFreshSitemapTestApp(fresh, &stubSitemapIndex{})

	target := "/api/v1/jobs/sitemap/fresh?offset=" + strconv.Itoa(freshSitemapMaxOffset)
	_ = freshSitemapEntries(t, app, target)
	if fresh.calls != 1 {
		t.Fatalf("fresh reader called %d times at the bound, want 1", fresh.calls)
	}
	if fresh.lastOffset != freshSitemapMaxOffset {
		t.Errorf("offset reached the reader as %d, want %d", fresh.lastOffset, freshSitemapMaxOffset)
	}
	if fresh.lastLimit != jobSitemapChunk {
		t.Errorf("limit reached the reader as %d, want %d", fresh.lastLimit, jobSitemapChunk)
	}
}
