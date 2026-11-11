package handler

import (
	"context"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/companywatch"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// watchQueries stands in for the watchlist's SQL: one known company, "acme", and a list that
// can be made full.
type watchQueries struct {
	full    bool
	removed db.RemoveFromWatchlistParams
	list    db.ListWatchlistParams
}

func (f *watchQueries) ListWatchlist(_ context.Context, arg db.ListWatchlistParams) ([]db.ListWatchlistRow, error) {
	f.list = arg
	at := pgtype.Timestamptz{Time: time.Unix(100, 0).UTC(), Valid: true}
	return []db.ListWatchlistRow{{Slug: "acme", Name: "Acme", AddedAt: at, Total: 1}}, nil
}

func (f *watchQueries) ListWatchlistSlugs(context.Context, int64) ([]string, error) {
	return []string{"acme"}, nil
}

func (f *watchQueries) IsWatchingCompany(_ context.Context, arg db.IsWatchingCompanyParams) (bool, error) {
	return arg.Slug == "acme", nil
}

func (f *watchQueries) ResolveWatchableCompany(_ context.Context, slug string) (db.ResolveWatchableCompanyRow, error) {
	if slug != "acme" {
		return db.ResolveWatchableCompanyRow{}, pgx.ErrNoRows
	}
	return db.ResolveWatchableCompanyRow{Slug: "acme", Name: "Acme"}, nil
}

func (f *watchQueries) AddToWatchlist(context.Context, db.AddToWatchlistParams) (pgtype.Timestamptz, error) {
	if f.full {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	return pgtype.Timestamptz{Time: time.Unix(200, 0).UTC(), Valid: true}, nil
}

func (f *watchQueries) AddManyToWatchlist(_ context.Context, arg db.AddManyToWatchlistParams) (db.AddManyToWatchlistRow, error) {
	return db.AddManyToWatchlistRow{Fresh: int64(len(arg.Slugs)), Inserted: int64(len(arg.Slugs))}, nil
}

func (f *watchQueries) RemoveFromWatchlist(_ context.Context, arg db.RemoveFromWatchlistParams) error {
	f.removed = arg
	return nil
}

func (f *watchQueries) ResolveCompanySlugAliases(context.Context, []string) ([]db.ResolveCompanySlugAliasesRow, error) {
	return nil, nil
}

func (f *watchQueries) ListWatchableCompaniesByFoldedSlug(_ context.Context, arg db.ListWatchableCompaniesByFoldedSlugParams) ([]db.ListWatchableCompaniesByFoldedSlugRow, error) {
	for _, k := range arg.FoldedKeys {
		if k == "acme" {
			return []db.ListWatchableCompaniesByFoldedSlugRow{{Slug: "acme", Name: "Acme", JobCount: 3}}, nil
		}
	}
	return nil, nil
}

// watchApp mounts the real route table; a signed-in caller is faked by a pass-through
// gate, or an unauthenticated one by userID 0.
func watchApp(q *watchQueries, userID int64) *fiber.App {
	h := &companyWatchHandlers{watch: companywatch.New(q)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: func(c *fiber.Ctx) error {
		if userID == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		c.Locals(auth.LocalsUserID, userID)
		return c.Next()
	}}, newContract(contractHandlers{companyWatchHandlers: h}))
	return app
}

func doWatch(t *testing.T, app *fiber.App, method, target, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, "/api/v1"+target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var out map[string]any
	if resp.StatusCode == fiber.StatusNoContent {
		return resp.StatusCode, nil
	}
	return decodeJSON(t, resp, &out), out
}

func TestCompanyWatch_ListShapesTheContract(t *testing.T) {
	q := &watchQueries{}
	status, body := doWatch(t, watchApp(q, 7), fiber.MethodGet, "/me/watchlist?q=%20ac%20&limit=25&offset=5", "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if q.list.UserID != 7 || q.list.Q != "ac" || q.list.PageLimit != 25 || q.list.PageOffset != 5 {
		t.Errorf("list args = %+v", q.list)
	}
	meta := body["meta"].(map[string]any)
	if meta["total"] != float64(1) || meta["limit"] != float64(25) || meta["offset"] != float64(5) || meta["max"] != float64(5000) {
		t.Errorf("meta = %v", meta)
	}
	row := body["data"].([]any)[0].(map[string]any)
	if row["slug"] != "acme" || row["name"] != "Acme" || row["added_at"] == nil {
		t.Errorf("row = %v", row)
	}
}

func TestCompanyWatch_AddRemoveAndMembership(t *testing.T) {
	q := &watchQueries{}
	app := watchApp(q, 7)

	status, body := doWatch(t, app, fiber.MethodPut, "/me/watchlist/acme", "")
	if status != fiber.StatusOK || body["data"].(map[string]any)["slug"] != "acme" {
		t.Fatalf("add = %d %v", status, body)
	}
	status, body = doWatch(t, app, fiber.MethodGet, "/me/watchlist/acme", "")
	if status != fiber.StatusOK || body["data"].(map[string]any)["watched"] != true {
		t.Fatalf("watching = %d %v", status, body)
	}
	status, _ = doWatch(t, app, fiber.MethodDelete, "/me/watchlist/acme", "")
	if status != fiber.StatusNoContent || q.removed.UserID != 7 || q.removed.Slug != "acme" {
		t.Fatalf("remove = %d %+v", status, q.removed)
	}
}

func TestCompanyWatch_Refusals(t *testing.T) {
	cases := []struct {
		name         string
		q            *watchQueries
		method, path string
		body         string
		want         int
	}{
		{"unknown company", &watchQueries{}, fiber.MethodPut, "/me/watchlist/nobody", "", fiber.StatusNotFound},
		{"full list", &watchQueries{full: true}, fiber.MethodPut, "/me/watchlist/acme", "", fiber.StatusBadRequest},
		{"empty import", &watchQueries{}, fiber.MethodPost, "/me/watchlist/import", `{"names":[" ",""]}`, fiber.StatusBadRequest},
		{"malformed import", &watchQueries{}, fiber.MethodPost, "/me/watchlist/import", `{`, fiber.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := doWatch(t, watchApp(tc.q, 7), tc.method, tc.path, tc.body)
			if status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			if body["error"] == nil {
				t.Errorf("no error message in %v", body)
			}
		})
	}
}

func TestCompanyWatch_ImportIsNotShadowedByTheSlugRoute(t *testing.T) {
	status, body := doWatch(t, watchApp(&watchQueries{}, 7), fiber.MethodPost, "/me/watchlist/import", `{"names":["Acme","Nowhere Ltd"]}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	data := body["data"].(map[string]any)
	unmatched := data["unmatched"].([]any)
	if data["added"] != float64(1) || len(unmatched) != 1 || unmatched[0] != "Nowhere Ltd" {
		t.Errorf("import = %v", data)
	}
}

func TestCompanyWatch_NeedsASignedInCaller(t *testing.T) {
	status, _ := doWatch(t, watchApp(&watchQueries{}, 0), fiber.MethodGet, "/me/watchlist", "")
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

// The page params are strings in the contract so a bad one falls back, as Fiber's QueryInt
// did, instead of being refused; the clamp still bounds both.
func TestCompanyWatch_PageParamsFallBackAndClamp(t *testing.T) {
	for target, want := range map[string][2]int32{
		"/me/watchlist?limit=abc&offset=xyz":     {50, 0},
		"/me/watchlist?limit=1000&offset=-3":     {100, 0},
		"/me/watchlist?offset=3000000000":        {50, math.MaxInt32},
		"/me/watchlist?limit=7&limit=9&offset=2": {7, 2},
	} {
		q := &watchQueries{}
		status, _ := doWatch(t, watchApp(q, 7), fiber.MethodGet, target, "")
		if status != fiber.StatusOK || q.list.PageLimit != want[0] || q.list.PageOffset != want[1] {
			t.Errorf("%s = %d limit %d offset %d, want %v", target, status, q.list.PageLimit, q.list.PageOffset, want)
		}
	}
}
