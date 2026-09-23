package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// meiliSearchProbe stands in for the engine on the /search route and records the one
// request body the reader sent. The sort and the filter are the whole contract of the
// fresh sitemap reader — an assertion on the returned documents would pass against a
// reader that forgot to sort, because a stub can return anything in any order.
type meiliSearchProbe struct {
	srv  *httptest.Server
	body map[string]any
}

func newMeiliSearchProbe(t *testing.T, hits []map[string]any) *meiliSearchProbe {
	t.Helper()
	p := &meiliSearchProbe{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/indexes/jobs/search" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&p.body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits":               hits,
			"estimatedTotalHits": len(hits),
		})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// ListFreshSitemapPage must ask the engine for newest-first order. The spec's
// ordering guarantee lives in this one request field: Meilisearch's /documents route
// cannot sort at all, which is why the paged sub-sitemap is unordered, so a fresh
// reader that lost its sort would silently become a second copy of the broken one.
func TestListFreshSitemapPageSortsNewestFirst(t *testing.T) {
	p := newMeiliSearchProbe(t, nil)
	c := NewClient(p.srv.URL, "key")

	if _, err := c.ListFreshSitemapPage(context.Background(), 0, 10); err != nil {
		t.Fatalf("ListFreshSitemapPage: %v", err)
	}

	sort, ok := p.body["sort"].([]any)
	if !ok || len(sort) != 1 {
		t.Fatalf("sort = %#v, want exactly one sort key", p.body["sort"])
	}
	if got := sort[0]; got != "created_at:desc" {
		t.Errorf("sort[0] = %v, want created_at:desc", got)
	}
}

// The fresh reader must narrow to exactly the population the paged sub-sitemap
// serves. Asserting against jobSitemapFilter itself rather than against a copy of its
// text is deliberate: a literal would keep passing if the shared filter gained a
// predicate and this reader did not, which is the drift that would put a non-tech
// posting in front of a crawler.
func TestListFreshSitemapPageUsesTheSharedFilter(t *testing.T) {
	p := newMeiliSearchProbe(t, nil)
	c := NewClient(p.srv.URL, "key")

	if _, err := c.ListFreshSitemapPage(context.Background(), 0, 10); err != nil {
		t.Fatalf("ListFreshSitemapPage: %v", err)
	}

	if got := p.body["filter"]; got != jobSitemapFilter {
		t.Errorf("filter = %v, want %v", got, jobSitemapFilter)
	}
}

// Offset and limit must reach the engine unchanged: the sitemap index tiles the fresh
// window by offset, so a reader that dropped or rounded one would serve the same first
// page under four different URLs.
func TestListFreshSitemapPagePassesOffsetAndLimit(t *testing.T) {
	p := newMeiliSearchProbe(t, nil)
	c := NewClient(p.srv.URL, "key")

	if _, err := c.ListFreshSitemapPage(context.Background(), 20000, 10000); err != nil {
		t.Fatalf("ListFreshSitemapPage: %v", err)
	}

	if got := p.body["offset"]; got != float64(20000) {
		t.Errorf("offset = %v, want 20000", got)
	}
	if got := p.body["limit"]; got != float64(10000) {
		t.Errorf("limit = %v, want 10000", got)
	}
}

// The projection is the same slim shape the paged reader returns — slug and lastmod,
// nothing wider — and a hit with no lastmod keeps its URL rather than dropping out,
// the rule sitemapPage already follows for a document indexed before updated_at
// joined the shape.
func TestListFreshSitemapPageProjectsSlugAndLastmod(t *testing.T) {
	p := newMeiliSearchProbe(t, []map[string]any{
		{"public_slug": "newest-job", "updated_at": "2026-09-22T10:00:00Z"},
		{"public_slug": "no-lastmod-job"},
	})
	c := NewClient(p.srv.URL, "key")

	docs, err := c.ListFreshSitemapPage(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("ListFreshSitemapPage: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len(docs) = %d, want 2", len(docs))
	}
	if docs[0].Slug != "newest-job" || !docs[0].UpdatedAt.Equal(time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("docs[0] = %+v, want newest-job at 2026-09-22T10:00:00Z", docs[0])
	}
	if docs[1].Slug != "no-lastmod-job" || !docs[1].UpdatedAt.IsZero() {
		t.Errorf("docs[1] = %+v, want no-lastmod-job with a zero lastmod", docs[1])
	}
}

// A hit carrying no slug is skipped rather than shipped as an empty URL — the same
// guard sitemapPage makes, since `/jobs/` is a different page from a job page.
func TestListFreshSitemapPageSkipsASlugLessHit(t *testing.T) {
	p := newMeiliSearchProbe(t, []map[string]any{
		{"updated_at": "2026-09-22T10:00:00Z"},
		{"public_slug": "real-job"},
	})
	c := NewClient(p.srv.URL, "key")

	docs, err := c.ListFreshSitemapPage(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("ListFreshSitemapPage: %v", err)
	}
	if len(docs) != 1 || docs[0].Slug != "real-job" {
		t.Fatalf("docs = %+v, want only real-job", docs)
	}
}
