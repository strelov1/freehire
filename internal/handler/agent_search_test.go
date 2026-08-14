package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

type fakeDescriptions struct {
	called bool
	gotIDs []int64
	rows   []db.GetJobDescriptionsByIDsRow
	err    error
}

func (f *fakeDescriptions) GetJobDescriptionsByIDs(_ context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error) {
	f.called = true
	f.gotIDs = ids
	return f.rows, f.err
}

func agentSearchApp(s searcher, d jobDescriptions) *fiber.App {
	h := &searchHandlers{search: s, descriptions: d}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/agent/jobs/search", h.AgentSearchJobs)
	return app
}

func TestFormatDescription(t *testing.T) {
	html := "<ul><li>alpha</li><li>beta</li></ul>"
	if got := formatDescription(html, ""); got != html {
		t.Errorf("default: got %q, want verbatim html", got)
	}
	if got := formatDescription(html, "xml"); got != html {
		t.Errorf("unknown format: got %q, want html fallback", got)
	}
	if got := formatDescription(html, "text"); strings.Contains(got, "<li>") {
		t.Errorf("text: tags not stripped: %q", got)
	}
	if got := formatDescription(html, "markdown"); strings.Contains(got, "<li>") || !strings.Contains(got, "alpha") {
		t.Errorf("markdown: got %q", got)
	}
}

func TestAgentSearchJobs_HydratesFullDescriptionByDefault(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 7, Job: jobview.Job{PublicSlug: "go-dev-x", Description: "<p>trunc...</p>"}}},
		Total: 3,
	}}
	desc := &fakeDescriptions{rows: []db.GetJobDescriptionsByIDsRow{{ID: 7, Description: "<p>the full verbatim description</p>"}}}
	app := agentSearchApp(fake, desc)

	status, body := doGet(t, app, "/agent/jobs/search?q=go&seniority=senior&limit=10&offset=20")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	// Parity: the same query/filters/paging reach the search backend as the public
	// endpoint would send — the shared runJobSearch core, verified at the agent layer.
	if fake.got.Query != "go" || fake.got.Limit != 10 || fake.got.Offset != 20 {
		t.Errorf("search params not forwarded: %#v", fake.got)
	}
	if groups, ok := fake.got.Filter.([][]string); !ok || !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("facet filter not forwarded: %#v", fake.got.Filter)
	}
	if !desc.called || len(desc.gotIDs) != 1 || desc.gotIDs[0] != 7 {
		t.Errorf("loader called=%v ids=%v, want [7]", desc.called, desc.gotIDs)
	}
	data, _ := body["data"].([]any)
	first, _ := data[0].(map[string]any)
	if first["description"] != "<p>the full verbatim description</p>" {
		t.Errorf("description = %v, want full verbatim html", first["description"])
	}
	if _, leaked := first["id"]; leaked {
		t.Errorf("internal id leaked: %v", first)
	}
	meta, _ := body["meta"].(map[string]any)
	if meta["total"].(float64) != 3 {
		t.Errorf("meta.total = %v, want 3", meta["total"])
	}
}

func TestAgentSearchJobs_BestEffortKeepsStaleHit(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{
		{ID: 1, Job: jobview.Job{PublicSlug: "a", Description: "full-a-src"}},
		{ID: 2, Job: jobview.Job{PublicSlug: "b", Description: "preview-b"}},
	}}}
	desc := &fakeDescriptions{rows: []db.GetJobDescriptionsByIDsRow{{ID: 1, Description: "FULL-a"}}}
	app := agentSearchApp(fake, desc)

	status, body := doGet(t, app, "/agent/jobs/search")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("data len = %d, want 2 (stale hit not dropped)", len(data))
	}
	stale, _ := data[1].(map[string]any)
	if stale["description"] != "preview-b" {
		t.Errorf("stale hit description = %v, want preview-b (kept)", stale["description"])
	}
}

func TestAgentSearchJobs_FormatApplies(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a", Description: "x"}}},
	}}
	desc := &fakeDescriptions{rows: []db.GetJobDescriptionsByIDsRow{{ID: 1, Description: "<ul><li>alpha</li></ul>"}}}
	app := agentSearchApp(fake, desc)

	status, body := doGet(t, app, "/agent/jobs/search?description_format=text")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].([]any)
	first, _ := data[0].(map[string]any)
	got, _ := first["description"].(string)
	if strings.Contains(got, "<li>") || !strings.Contains(got, "alpha") {
		t.Errorf("text format not applied to hydrated description: %q", got)
	}
}
