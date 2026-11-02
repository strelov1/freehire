package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

type fakeRemovals struct {
	gone []int64
	err  error
	got  []int64
}

func (f *fakeRemovals) ListPendingSearchRemovals(_ context.Context, ids []int64) ([]int64, error) {
	f.got = ids
	return f.gone, f.err
}

func pendingRemovalsApp(s searcher, r pendingRemovals) *fiber.App {
	h := &searchHandlers{search: s, removals: r}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", h.SearchJobs)
	return app
}

func pageOf(ids ...int64) search.SearchResult {
	hits := make([]search.JobDocument, len(ids))
	for i, id := range ids {
		hits[i] = search.JobDocument{ID: id}
	}
	return search.SearchResult{Hits: hits, Total: 100}
}

func TestSearchJobsDropsPostingsWhoseRemovalIsQueued(t *testing.T) {
	removals := &fakeRemovals{gone: []int64{2}}
	app := pendingRemovalsApp(&fakeSearcher{res: pageOf(1, 2, 3)}, removals)

	status, body := doGet(t, app, "/jobs/search")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("hits = %d, want 2 — the closed posting must not be served", len(data))
	}
	meta, _ := body["meta"].(map[string]any)
	if total, _ := meta["total"].(float64); total != 99 {
		t.Errorf("total = %v, want 99 — one fewer, the dropped hit is not a result", total)
	}
	if len(removals.got) != 3 {
		t.Errorf("asked about %v, want the page's three ids", removals.got)
	}
}

func TestSearchJobsKeepsThePageWhenNothingIsQueued(t *testing.T) {
	app := pendingRemovalsApp(&fakeSearcher{res: pageOf(1, 2)}, &fakeRemovals{})

	_, body := doGet(t, app, "/jobs/search")
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("hits = %d, want 2", len(data))
	}
	meta, _ := body["meta"].(map[string]any)
	if total, _ := meta["total"].(float64); total != 100 {
		t.Errorf("total = %v, want 100 unchanged", total)
	}
}

func TestSearchJobsServesTheIndexAsItIsWhenTheLookupFails(t *testing.T) {
	app := pendingRemovalsApp(&fakeSearcher{res: pageOf(1, 2)}, &fakeRemovals{err: errors.New("db down")})

	status, body := doGet(t, app, "/jobs/search")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a failed lookup must not fail the search", status)
	}
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("hits = %d, want 2", len(data))
	}
}
