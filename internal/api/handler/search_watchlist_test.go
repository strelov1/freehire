package handler

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeWatchlist struct {
	slugs  []string
	err    error
	called bool
	userID int64
}

func (f *fakeWatchlist) Slugs(_ context.Context, userID int64) ([]string, error) {
	f.called, f.userID = true, userID
	return f.slugs, f.err
}

// watchlistApp mounts /jobs/search with both personal company lookups. userID 0 is an
// anonymous caller; a nil lookup is left unwired.
func watchlistApp(s searcher, userID int64, watch watchlistLookup, former formerEmployerLookup) *fiber.App {
	h := &searchHandlers{search: s, watchlist: watch, formerEmployers: former}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return c.Next()
	}, newContract(contractHandlers{searchHandlers: h}))
	return app
}

func TestWatchlist_SignedInNarrowsToTheWatchedCompanies(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 5}}
	lookup := &fakeWatchlist{slugs: []string{"acme", "globex"}}
	app := watchlistApp(fake, 7, lookup, nil)

	status, body := doGet(t, app, "/api/v1/jobs/search?watchlist=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if lookup.userID != 7 {
		t.Errorf("looked up user %d, want the caller (7)", lookup.userID)
	}
	want := [][]string{{`enrichment.seniority = "senior"`}, {`company_slug IN ["acme", "globex"]`}}
	if !reflect.DeepEqual(fake.got.Filter, want) {
		t.Errorf("filter = %#v, want %#v", fake.got.Filter, want)
	}
	if names := ignoredParamNames(body); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — the filter was read", names)
	}
}

func TestWatchlist_EmptyListAnswersEmptyNotTheCatalogue(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 999}}
	app := watchlistApp(fake, 7, &fakeWatchlist{}, nil)

	status, body := doGet(t, app, "/api/v1/jobs/search?watchlist=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried; with nothing watched it must not be, or the answer is strangers")
	}
	if total := body["meta"].(map[string]any)["total"]; total != float64(0) {
		t.Errorf("total = %v, want 0", total)
	}
	if names := ignoredParamNames(body); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — the param was read", names)
	}
}

func TestWatchlist_AnonymousIsReportedAsIgnored(t *testing.T) {
	fake := &fakeSearcher{}
	lookup := &fakeWatchlist{slugs: []string{"acme"}}
	app := watchlistApp(fake, 0, lookup, nil)

	status, body := doGet(t, app, "/api/v1/jobs/search?watchlist=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a shared link must not break for a signed-out visitor", status)
	}
	if lookup.called || fake.got.Filter != nil {
		t.Errorf("an anonymous request read a watchlist (filter %#v)", fake.got.Filter)
	}
	if names := ignoredParamNames(body); !reflect.DeepEqual(names, []string{search.WatchlistParam}) {
		t.Errorf("ignored_params = %v, want [%s]", names, search.WatchlistParam)
	}
}

func TestWatchlist_FalseIsNotAFilter(t *testing.T) {
	fake := &fakeSearcher{}
	lookup := &fakeWatchlist{slugs: []string{"acme"}}
	app := watchlistApp(fake, 7, lookup, nil)

	if status, _ := doGet(t, app, "/api/v1/jobs/search?watchlist=false"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if lookup.called || fake.got.Filter != nil {
		t.Errorf("watchlist=false narrowed the search (filter %#v)", fake.got.Filter)
	}
}

// Only `watchlist=true` is the filter, the spelling search.Narrows and the notify worker read.
// A search that took `watchlist=1` as well would show a saved `watchlist=1&q=go` narrowed to
// the list, while its alert mailed every Go posting in the catalogue.
func TestWatchlist_OnlyTrueIsTheFilter(t *testing.T) {
	fake := &fakeSearcher{}
	lookup := &fakeWatchlist{slugs: []string{"acme"}}
	app := watchlistApp(fake, 7, lookup, nil)

	status, body := doGet(t, app, "/api/v1/jobs/search?watchlist=1")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if lookup.called || fake.got.Filter != nil {
		t.Errorf("watchlist=1 narrowed the search (filter %#v)", fake.got.Filter)
	}
	if names := ignoredParamNames(body); !reflect.DeepEqual(names, []string{search.WatchlistParam}) {
		t.Errorf("ignored_params = %v, want [%s]", names, search.WatchlistParam)
	}
}

func TestWatchlist_LookupFailureFailsTheRequest(t *testing.T) {
	fake := &fakeSearcher{}
	app := watchlistApp(fake, 7, &fakeWatchlist{err: errors.New("db down")}, nil)

	if status, _ := doGet(t, app, "/api/v1/jobs/search?watchlist=true"); status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried after the lookup failed")
	}
}

func TestWatchlist_WithFormerEmployersIsTheIntersection(t *testing.T) {
	fake := &fakeSearcher{}
	app := watchlistApp(fake, 7,
		&fakeWatchlist{slugs: []string{"acme", "globex", "initech"}},
		&fakeFormerEmployers{slugs: []string{"globex", "initech", "umbrella"}})

	if status, _ := doGet(t, app, "/api/v1/jobs/search?watchlist=true&former_employers=true"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := [][]string{{`company_slug IN ["globex", "initech"]`}}
	if !reflect.DeepEqual(fake.got.Filter, want) {
		t.Errorf("filter = %#v, want %#v", fake.got.Filter, want)
	}
}

func TestWatchlist_DisjointFromFormerEmployersAnswersEmpty(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 999}}
	app := watchlistApp(fake, 7, &fakeWatchlist{slugs: []string{"acme"}}, &fakeFormerEmployers{slugs: []string{"globex"}})

	status, body := doGet(t, app, "/api/v1/jobs/search?watchlist=true&former_employers=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried for an empty intersection")
	}
	if names := ignoredParamNames(body); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none", names)
	}
}

func TestWatchlist_SurvivesTheBadFilterDegrade(t *testing.T) {
	fake := &degradingSearcher{}
	app := watchlistApp(fake, 7, &fakeWatchlist{slugs: []string{"acme"}}, nil)

	if status, _ := doGet(t, app, "/api/v1/jobs/search?watchlist=true&seniority=senior"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("searched %d times, want the query and one retry", len(fake.calls))
	}
	want := [][]string{{`company_slug IN ["acme"]`}}
	if !reflect.DeepEqual(fake.calls[1].Filter, want) {
		t.Errorf("retry filter = %#v, want only the watchlist group %#v", fake.calls[1].Filter, want)
	}
}
