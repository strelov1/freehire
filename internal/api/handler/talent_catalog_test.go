package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/platform/db"
)

const catalogBackendCV = `{"full_name":"Ada Lovelace","email":"ada@example.com",
  "summary":"Led the platform team at Analytical Engines",
  "total_years":8,"skills":["Go","PostgreSQL"],
  "experience":[{"title":"Senior Backend Engineer","company":"Analytical Engines",
    "current":true,"stack":["Go"],"highlights":["Cut Analytical Engines spend"]}]}`

const catalogFrontendCV = `{"total_years":3,"skills":["React"],
  "experience":[{"title":"Frontend Developer","company":"Acme","current":true}]}`

type fakeTalentCatalogStore struct {
	rows []db.ListTalentNetworkMembersRow
	one  *db.GetTalentNetworkMemberByHandleRow
}

func (f *fakeTalentCatalogStore) ListTalentNetworkMembers(context.Context) ([]db.ListTalentNetworkMembersRow, error) {
	return f.rows, nil
}

func (f *fakeTalentCatalogStore) GetTalentNetworkMemberByHandle(_ context.Context, handle string) (db.GetTalentNetworkMemberByHandleRow, error) {
	if f.one == nil || f.one.TalentHandle.String != handle {
		return db.GetTalentNetworkMemberByHandleRow{}, pgx.ErrNoRows
	}
	return *f.one, nil
}

func catalogRow(handle, cv string, fresh time.Time) db.ListTalentNetworkMembersRow {
	return db.ListTalentNetworkMembersRow{
		TalentHandle:               pgtype.Text{String: handle, Valid: true},
		Timezone:                   pgtype.Text{String: "Europe/Berlin", Valid: true},
		Cities:                     []string{"berlin"},
		ResumeStructured:           []byte(cv),
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: fresh, Valid: true},
		Specializations:            []string{},
	}
}

// talentCatalogApp mounts both public routes with NO auth and no limiter, so the tests
// exercise the handlers rather than the middleware. That the real routes carry a limiter
// is asserted separately, against the real router.
func talentCatalogApp(store talentnetwork.Store) *fiber.App {
	h := newTalentCatalogHandlers(talentnetwork.NewCatalogue(store, time.Minute, time.Now))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/talent", h.List)
	// Registered before the parametrised route, exactly as `register` does — a test that
	// mounted them the other way round would pass while the real router 404s.
	app.Get("/talent/facets", h.Facets)
	app.Get("/talent/:handle", h.Get)
	return app
}

// readBody and forbidSubstrings moved here from talent_network_profile_test.go when that
// route was retired. forbidSubstrings is the shape most of these assertions take: the
// interesting claim about a public response is what is ABSENT from it.
func talentNetworkReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func forbidSubstrings(t *testing.T, body string, forbidden ...string) {
	t.Helper()
	for _, s := range forbidden {
		if strings.Contains(body, s) {
			t.Errorf("body must not contain %q: %s", s, body)
		}
	}
}

func doTalent(t *testing.T, app *fiber.App, target string) *http.Response {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("request %s: %v", target, err)
	}
	return resp
}

func decodeTalentList(t *testing.T, resp *http.Response) struct {
	Data []talentnetwork.CatalogueMember `json:"data"`
	Meta struct {
		Total         int `json:"total"`
		Limit         int `json:"limit"`
		Offset        int `json:"offset"`
		IgnoredParams []struct {
			Param string `json:"param"`
		} `json:"ignored_params"`
	} `json:"meta"`
} {
	t.Helper()
	var out struct {
		Data []talentnetwork.CatalogueMember `json:"data"`
		Meta struct {
			Total         int `json:"total"`
			Limit         int `json:"limit"`
			Offset        int `json:"offset"`
			IgnoredParams []struct {
				Param string `json:"param"`
			} `json:"ignored_params"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out
}

func twoMemberStore() *fakeTalentCatalogStore {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return &fakeTalentCatalogStore{rows: []db.ListTalentNetworkMembersRow{
		catalogRow("backend-aaaa", catalogBackendCV, base),
		catalogRow("frontend-bbbb", catalogFrontendCV, base.Add(-time.Hour)),
	}}
}

func TestTalentCatalogList_IsOpenToAnonymousVisitors(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeTalentList(t, resp)
	if got.Meta.Total != 2 || len(got.Data) != 2 {
		t.Errorf("total = %d, data = %d; want 2 and 2", got.Meta.Total, len(got.Data))
	}
}

// The listing must carry no more than a card does. This is the same invariant the
// projection's own tests assert, checked once more at the wire — a handler that added a
// field for convenience would pass those and fail this.
func TestTalentCatalogList_LeaksNothingFromTheCV(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent")
	defer resp.Body.Close()
	body := talentNetworkReadBody(t, resp)
	forbidSubstrings(t, body, "Ada Lovelace", "ada@example.com", "Analytical Engines", "Acme")
}

func TestTalentCatalogList_FiltersNarrowTheTotal(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent?categories=backend")
	defer resp.Body.Close()
	got := decodeTalentList(t, resp)
	if got.Meta.Total != 1 {
		t.Errorf("total = %d for categories=backend, want 1", got.Meta.Total)
	}
	if len(got.Data) != 1 || got.Data[0].Handle != "backend-aaaa" {
		t.Errorf("data = %+v, want only the backend member", got.Data)
	}
}

// meta.total must count the FILTERED set, not the catalogue. A total that reports the
// whole membership behind a narrowed page is how a UI builds pages that do not exist.
func TestTalentCatalogList_TotalIsBehindTheSameFilter(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent?categories=backend&limit=1")
	defer resp.Body.Close()
	if got := decodeTalentList(t, resp); got.Meta.Total != 1 {
		t.Errorf("total = %d, want 1 — the count must share the page's predicate", got.Meta.Total)
	}
}

func TestTalentCatalogList_ReportsParamsItDidNotRead(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	cases := map[string]string{
		"/talent?countries=de":     "countries", // a jobs facet, meaningless here
		"/talent?min_years=lots":   "min_years", // recognised, unreadable
		"/talent?limit=1000":       "limit",     // recognised, out of range
		"/talent?category=backend": "category",  // the singular of a real filter
	}
	for target, want := range cases {
		t.Run(target, func(t *testing.T) {
			resp := doTalent(t, app, target)
			defer resp.Body.Close()
			got := decodeTalentList(t, resp)
			if len(got.Meta.IgnoredParams) != 1 || got.Meta.IgnoredParams[0].Param != want {
				t.Errorf("ignored_params = %+v, want [%s]", got.Meta.IgnoredParams, want)
			}
		})
	}
}

// The key is omitted entirely when nothing was ignored. An always-present empty array is
// a field every reader learns to skip, and then the one response that carries a warning
// gets skipped too.
func TestTalentCatalogList_OmitsIgnoredParamsWhenThereAreNone(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent?categories=backend&limit=5")
	defer resp.Body.Close()
	body := talentNetworkReadBody(t, resp)
	forbidSubstrings(t, body, "ignored_params")
}

func TestTalentCatalogGet_ServesAMemberByHandle(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := twoMemberStore()
	store.one = &db.GetTalentNetworkMemberByHandleRow{
		TalentHandle:               pgtype.Text{String: "backend-aaaa", Valid: true},
		Timezone:                   pgtype.Text{String: "Europe/Berlin", Valid: true},
		Cities:                     []string{"berlin"},
		ResumeStructured:           []byte(catalogBackendCV),
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: base, Valid: true},
		Specializations:            []string{},
	}
	app := talentCatalogApp(store)

	resp := doTalent(t, app, "/talent/backend-aaaa")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Robots-Tag"); got != "noindex" {
		t.Errorf("X-Robots-Tag = %q, want noindex — a card is a person, and a cached one outlives their leaving", got)
	}
	// `private` and not `public`: a shared cache holding a departed member's card is
	// this route's own promise — 404 on the next request — broken by an intermediary.
	if got := resp.Header.Get("Cache-Control"); !strings.HasPrefix(got, "private") {
		t.Errorf("Cache-Control = %q, want it to start with private", got)
	}
	forbidSubstrings(t, talentNetworkReadBody(t, resp), "Ada Lovelace", "Analytical Engines")
}

// Every way of not being in the catalogue answers identically, so the route cannot be
// used to ask whether an account exists.
func TestTalentCatalogGet_AbsentMalformedAndUnknownAnswerTheSame(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	var bodies []string
	for _, target := range []string{
		"/talent/backend-zzzz", // well-formed, nobody holds it
		"/talent/NotAHandle",   // uppercase: cannot be one
		"/talent/backend",      // no suffix: cannot be one
		"/talent/ivan-strelov", // shaped like an account username
	} {
		resp := doTalent(t, app, target)
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("%s status = %d, want 404", target, resp.StatusCode)
		}
		bodies = append(bodies, talentNetworkReadBody(t, resp))
		resp.Body.Close()
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("404 bodies differ:\n%s\n%s", bodies[0], bodies[i])
		}
	}
}

func decodeFacets(t *testing.T, resp *http.Response) talentnetwork.Counts {
	t.Helper()
	var out struct {
		Data talentnetwork.Counts `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode facets: %v", err)
	}
	return out.Data
}

func TestTalentFacets_ServesCountsToAnonymousVisitors(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent/facets")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeFacets(t, resp)
	if got.Total != 2 {
		t.Errorf("total = %d, want 2", got.Total)
	}
	if _, ok := got.Facets["categories"]; !ok {
		t.Error("no category counts")
	}
}

// The route order is the trap: `/talent/:handle` registered first would swallow this and
// answer an honest 404, which reads as "facets are broken" rather than "the route was
// shadowed".
func TestTalentFacets_IsNotSwallowedByTheHandleRoute(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent/facets")
	defer resp.Body.Close()
	if resp.StatusCode == fiber.StatusNotFound {
		t.Fatal("/talent/facets answered 404 — the parametrised handle route shadowed it")
	}
}

// Counts carry no more than a card does: they are numbers over dictionary values, and the
// employer names seeded into the fixture must not reach them either.
func TestTalentFacets_LeakNothingFromTheCV(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent/facets")
	defer resp.Body.Close()
	forbidSubstrings(t, talentNetworkReadBody(t, resp),
		"Ada Lovelace", "ada@example.com", "Analytical Engines", "Acme")
}

func TestTalentFacets_ReportsParamsItDidNotRead(t *testing.T) {
	app := talentCatalogApp(twoMemberStore())

	resp := doTalent(t, app, "/talent/facets?countries=de")
	defer resp.Body.Close()
	var out struct {
		Meta struct {
			IgnoredParams []struct {
				Param string `json:"param"`
			} `json:"ignored_params"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Meta.IgnoredParams) != 1 || out.Meta.IgnoredParams[0].Param != "countries" {
		t.Errorf("ignored_params = %+v, want [countries]", out.Meta.IgnoredParams)
	}
}
