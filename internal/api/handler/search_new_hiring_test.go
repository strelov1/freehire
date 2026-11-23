package handler

import (
	"context"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeNewHiring []string

func (f fakeNewHiring) ListNewHiringCompanySlugs(context.Context) ([]string, error) { return f, nil }

// newHiringApp mounts /jobs/search with the new-hiring lookup and a watchlist. userID 0 is
// an anonymous caller; a nil lookup is left unwired.
func newHiringApp(s searcher, userID int64, newHiring newHiringLookup, watch watchlistLookup) *fiber.App {
	h := &searchHandlers{search: s, watchlist: watch, newHiring: newHiring}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return c.Next()
	}, newContract(contractHandlers{searchHandlers: h}))
	return app
}

func TestNewHiring_AnonymousNarrowsToTheMarkedCompanies(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 3}}
	app := newHiringApp(fake, 0, fakeNewHiring{"acme", "globex"}, nil)

	status, body := doGet(t, app, "/api/v1/jobs/search?new_hiring=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := [][]string{{`enrichment.seniority = "senior"`}, {`company_slug IN ["acme", "globex"]`}}
	if !reflect.DeepEqual(fake.got.Filter, want) {
		t.Errorf("filter = %#v, want %#v", fake.got.Filter, want)
	}
	if names := ignoredParamNames(body); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — the filter was read", names)
	}
}

func TestNewHiring_EmptySetAnswersEmptyNotTheCatalogue(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 999}}
	app := newHiringApp(fake, 0, fakeNewHiring{}, nil)

	if status, _ := doGet(t, app, "/api/v1/jobs/search?new_hiring=true"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried with no marked company; the answer would be the whole catalogue")
	}
}

func TestNewHiring_WithWatchlistIsTheIntersection(t *testing.T) {
	fake := &fakeSearcher{}
	app := newHiringApp(fake, 7, fakeNewHiring{"acme", "globex"}, &fakeWatchlist{slugs: []string{"globex", "initech"}})

	if status, _ := doGet(t, app, "/api/v1/jobs/search?new_hiring=true&watchlist=true"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := [][]string{{`company_slug IN ["globex"]`}}
	if !reflect.DeepEqual(fake.got.Filter, want) {
		t.Errorf("filter = %#v, want %#v", fake.got.Filter, want)
	}
}

func TestNewHiring_UnwiredIsReportedAsIgnored(t *testing.T) {
	fake := &fakeSearcher{}
	app := newHiringApp(fake, 0, nil, nil)

	_, body := doGet(t, app, "/api/v1/jobs/search?new_hiring=true")
	if fake.got.Filter != nil {
		t.Errorf("filter = %#v, want none", fake.got.Filter)
	}
	if names := ignoredParamNames(body); !reflect.DeepEqual(names, []string{newHiringParam}) {
		t.Errorf("ignored_params = %v, want [%s]", names, newHiringParam)
	}
}
