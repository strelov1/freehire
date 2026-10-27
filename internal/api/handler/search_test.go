package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

type fakeSearcher struct {
	got search.SearchParams
	res search.SearchResult
	err error
}

func (f *fakeSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.got = p
	return f.res, f.err
}

func searchApp(s searcher) *fiber.App {
	h := &searchHandlers{search: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", h.SearchJobs)
	return app
}

func doGet(t *testing.T, app *fiber.App, target string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var body map[string]any
	return decodeJSON(t, resp, &body), body
}

func TestSearchJobs_DisabledReturns503(t *testing.T) {
	app := searchApp(nil) // search not configured
	status, _ := doGet(t, app, "/jobs/search?q=go")
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestSearchJobs_PassesParamsAndShapesResponse(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "go-dev-acme-x", Title: "Go Dev"}}},
		Total: 5,
	}}
	app := searchApp(fake)

	status, body := doGet(t, app, "/jobs/search?q=golang&limit=10&offset=20&seniority=senior&regions=latam")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// Params mapped onto SearchParams.
	if fake.got.Query != "golang" {
		t.Errorf("Query = %q, want golang", fake.got.Query)
	}
	if fake.got.Limit != 10 || fake.got.Offset != 20 {
		t.Errorf("limit/offset = %d/%d, want 10/20", fake.got.Limit, fake.got.Offset)
	}
	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `enrichment.seniority = "senior"`) || !filterHas(groups, `regions = "latam"`) {
		t.Errorf("Filter missing facets: %#v", groups)
	}

	// Response envelope: data carries the public view (no id), meta has totals.
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}
	first, _ := data[0].(map[string]any)
	if first["public_slug"] != "go-dev-acme-x" {
		t.Errorf("public_slug = %v", first["public_slug"])
	}
	if _, leaked := first["id"]; leaked {
		t.Errorf("internal id leaked in search result: %v", first)
	}
	meta, _ := body["meta"].(map[string]any)
	if meta["total"].(float64) != 5 || meta["limit"].(float64) != 10 || meta["offset"].(float64) != 20 {
		t.Errorf("meta = %v", meta)
	}
}

func TestSearchJobs_DeepPaginationRejected(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// offset+limit past the window must be refused before the backend is touched.
	status, body := doGet(t, app, "/jobs/search?offset=10000&limit=100")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searcher was called despite deep pagination: %#v", fake.got)
	}
	if body["error"] == nil {
		t.Errorf("expected an error message, got %v", body)
	}
}

func TestSearchJobs_PaginationAtWindowBoundaryAllowed(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// offset+limit exactly at the window is still allowed and reaches the backend.
	status, _ := doGet(t, app, "/jobs/search?offset=9900&limit=100")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Offset != 9900 || fake.got.Limit != 100 {
		t.Errorf("offset/limit = %d/%d, want 9900/100 (within window)", fake.got.Offset, fake.got.Limit)
	}
}

func TestSearchJobs_BackendErrorMaps500(t *testing.T) {
	fake := &fakeSearcher{err: context.DeadlineExceeded}
	app := searchApp(fake)
	status, _ := doGet(t, app, "/jobs/search?q=go")
	if status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

func TestSearchJobs_ExcludeAndAndMode(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	doGet(t, app, "/jobs/search?seniority=senior&seniority_exclude=junior&skills=go&skills=react&skills_mode=and")

	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Error("missing include seniority=senior")
	}
	if !filterHas(groups, `enrichment.seniority != "junior"`) {
		t.Error("missing exclude seniority!=junior")
	}
	// AND mode: each skill must be its own single-element group (ANDed), not one
	// OR group of both.
	for _, skill := range []string{"go", "react"} {
		expr := `skills = "` + skill + `"`
		found := false
		for _, g := range groups {
			if len(g) == 1 && g[0] == expr {
				found = true
			}
		}
		if !found {
			t.Errorf("skills AND mode: %q not in its own group; groups=%#v", expr, groups)
		}
	}
}

func filterHas(groups [][]string, expr string) bool {
	for _, g := range groups {
		for _, e := range g {
			if e == expr {
				return true
			}
		}
	}
	return false
}

func TestSearchJobs_DefaultSortIsPostedAtForEmptyQuery(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// No query text, no sort → freshest by posting date first.
	if status, _ := doGet(t, app, "/jobs/search"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "posted_at:desc" {
		t.Errorf("Sort = %v, want [posted_at:desc] for empty q", fake.got.Sort)
	}

	// Text query, no sort → relevance (nil sort).
	if status, _ := doGet(t, app, "/jobs/search?q=golang"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Sort != nil {
		t.Errorf("Sort = %v, want nil (relevance) for a text query", fake.got.Sort)
	}

	// Explicit sort always wins, q or not — and created_at is still accepted.
	if status, _ := doGet(t, app, "/jobs/search?q=golang&sort=created_at&order=asc"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "created_at:asc" {
		t.Errorf("Sort = %v, want [created_at:asc] for explicit sort", fake.got.Sort)
	}
}

func TestSearchJobs_ReportsIgnoredParams(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// `country` is not the facet name (`countries` is), so it filters nothing.
	// The query still runs — old links and saved searches must keep working —
	// but the response has to say the param was dropped, or an unfiltered
	// result set reads as a real answer about Italy.
	status, body := doGet(t, app, "/jobs/search?country=it")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
	}
	first, _ := ignored[0].(map[string]any)
	if first["param"] != "country" || first["did_you_mean"] != "countries" {
		t.Errorf("ignored_params[0] = %v, want country -> countries", first)
	}
}

func TestSearchJobs_ReportsIgnoredValues(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// `foo` is no work mode, so the filter narrows to nothing and `total: 0` would
	// read as an empty market. The filter still receives it — the report describes
	// the answer, it does not change it.
	status, body := doGet(t, app, "/jobs/search?work_mode=foo")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Filter == nil {
		t.Error("Filter = nil, want the unknown value still applied")
	}
	meta, _ := body["meta"].(map[string]any)
	values, _ := meta["ignored_values"].(map[string]any)
	got, _ := values["work_mode"].([]any)
	if len(got) != 1 || got[0] != "foo" {
		t.Errorf("meta.ignored_values = %v, want {work_mode: [foo]}", meta["ignored_values"])
	}

	_, body = doGet(t, app, "/jobs/search?work_mode=remote")
	meta, _ = body["meta"].(map[string]any)
	if _, present := meta["ignored_values"]; present {
		t.Errorf("meta.ignored_values = %v, want the key absent on a clean request", meta["ignored_values"])
	}
}

func TestSearchJobs_QFieldsRestrictsSearchableAttributes(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	status, body := doGet(t, app, "/jobs/search?q=systems&q_fields=title")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.QFields) != 1 || fake.got.QFields[0] != "title" {
		t.Errorf("QFields = %v, want [title]", fake.got.QFields)
	}
	// q_fields is a param this endpoint reads, so a valid value must not itself
	// be reported as ignored.
	meta, _ := body["meta"].(map[string]any)
	if _, present := meta["ignored_params"]; present {
		t.Errorf("meta.ignored_params = %v, want the key absent for a valid q_fields", meta["ignored_params"])
	}
}

func TestSearchJobs_QFieldsAbsentLeavesAllFieldsSearchable(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	doGet(t, app, "/jobs/search?q=golang")
	if fake.got.QFields != nil {
		t.Errorf("QFields = %v, want nil when q_fields is absent", fake.got.QFields)
	}
}

func TestSearchJobs_UnrecognizedQFieldsIsReportedAndAppliesNoRestriction(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	status, body := doGet(t, app, "/jobs/search?q=systems&q_fields=salary")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.QFields != nil {
		t.Errorf("QFields = %v, want nil — an unrecognized value must not restrict", fake.got.QFields)
	}
	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
	}
	first, _ := ignored[0].(map[string]any)
	if first["param"] != "q_fields" {
		t.Errorf("ignored_params[0] = %v, want q_fields", first)
	}
}

func TestSearchJobs_CleanQueryReportsNothingIgnored(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// Transport params belong to the handler rather than to the filter
	// vocabulary; a well-formed query must not accuse them of anything.
	_, body := doGet(t, app, "/jobs/search?q=go&limit=10&offset=0&sort=posted_at&order=asc&countries=it")

	meta, _ := body["meta"].(map[string]any)
	if _, present := meta["ignored_params"]; present {
		t.Errorf("meta.ignored_params = %v, want the key absent", meta["ignored_params"])
	}
}

// The "Most viewed" ordering. It is a plain attribute sort like the dates, so what is
// worth pinning is that the bare `view_count` name is accepted at all — an unlisted
// value is silently ignored, which would leave the feed date-ordered while the control
// claimed otherwise — and that it defaults to descending, since most-viewed-first is
// the whole point and needing `&order=desc` to reach it would be a trap.
func TestSearchJobs_ViewCountSortDefaultsToDescending(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	if status, _ := doGet(t, app, "/jobs/search?sort=view_count"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "view_count:desc" {
		t.Errorf("Sort = %v, want [view_count:desc] — an ignored sort value would leave the feed date-ordered", fake.got.Sort)
	}

	// Ascending is reachable, so "least viewed first" needs no separate parameter.
	if status, _ := doGet(t, app, "/jobs/search?sort=view_count&order=asc"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "view_count:asc" {
		t.Errorf("Sort = %v, want [view_count:asc]", fake.got.Sort)
	}

	// It ranks by a stored figure, so it survives query text rather than yielding to
	// relevance the way an unrecognised value would.
	if status, _ := doGet(t, app, "/jobs/search?q=golang&sort=view_count"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "view_count:desc" {
		t.Errorf("Sort = %v, want [view_count:desc] under query text", fake.got.Sort)
	}
}

// A title suggestion's click sends its query quoted (see the search-suggestions
// spec). recordQuery must land it on the same demand key an unquoted, typed search
// for the same words would use — otherwise every suggestion click starts a second,
// never-matching demand bucket for a phrase the dictionary already tracks.
func TestDemandKey_StripsMatchingQuotePairBeforeNormalising(t *testing.T) {
	quoted := demandKey(`"Founding Engineer"`)
	typed := demandKey(`Founding Engineer`)
	if quoted != typed {
		t.Fatalf("demandKey(quoted) = %q, demandKey(typed) = %q, want equal", quoted, typed)
	}
	if quoted != suggest.Title("Founding Engineer") {
		t.Fatalf("demandKey(quoted) = %q, want %q", quoted, suggest.Title("Founding Engineer"))
	}
}

// A single leading or trailing quote is not the suggestion-click shape (that always
// wraps both ends), so it is left alone rather than guessed at — stripping only one
// side would rewrite text nobody asked to rewrite.
func TestDemandKey_LeavesUnmatchedQuoteAlone(t *testing.T) {
	leading := demandKey(`"Founding Engineer`)
	if want := suggest.Title(`"Founding Engineer`); leading != want {
		t.Fatalf("demandKey(leading-only) = %q, want %q (untouched)", leading, want)
	}
	trailing := demandKey(`Founding Engineer"`)
	if want := suggest.Title(`Founding Engineer"`); trailing != want {
		t.Fatalf("demandKey(trailing-only) = %q, want %q (untouched)", trailing, want)
	}
}
