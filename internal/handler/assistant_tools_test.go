package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// toolByName finds a registered tool, failing the test when it is absent.
func toolByName(t *testing.T, tools []assistant.Tool, name string) assistant.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q is not registered; got %v", name, toolNames(tools))
	return assistant.Tool{}
}

func toolNames(tools []assistant.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}

// recordingSearcher captures the search it was asked to run and answers with a
// canned result.
type recordingSearcher struct {
	got search.SearchParams
	res search.SearchResult
}

func (s *recordingSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	s.got = p
	return s.res, nil
}

// fixedDescriptions rehydrates every requested id with the same markdown body.
type fixedDescriptions struct{ body string }

func (d fixedDescriptions) GetJobDescriptionsByIDs(_ context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error) {
	out := make([]db.GetJobDescriptionsByIDsRow, len(ids))
	for i, id := range ids {
		out[i] = db.GetJobDescriptionsByIDsRow{ID: id, Description: d.body}
	}
	return out, nil
}

func hitDoc(id int64, slug, title string) search.JobDocument {
	doc := search.JobDocument{}
	doc.ID = id
	doc.PublicSlug = slug
	doc.Title = title
	return doc
}

func TestSearchJobsToolPassesQueryAndFacetFilter(t *testing.T) {
	s := &recordingSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{hitDoc(1, "go-dev-acme", "Go Developer")},
		Total: 1,
	}}
	a := assistantWith(&searchHandlers{search: s, descriptions: fixedDescriptions{body: "<p>Full body</p>"}}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"query":"golang","filters":{"seniority":["senior"],"regions":["eu"]},"limit":5}`))
	if err != nil {
		t.Fatalf("search_jobs: %v", err)
	}
	if s.got.Query != "golang" {
		t.Errorf("query = %q, want golang", s.got.Query)
	}
	if s.got.Limit != 5 {
		t.Errorf("limit = %d, want 5", s.got.Limit)
	}
	if s.got.Filter == nil {
		t.Fatal("filter is nil; the facet arguments were dropped")
	}

	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.Contains(string(payload), "go-dev-acme") {
		t.Errorf("result = %s, want it to carry the public slug the agent links by", payload)
	}
}

func TestSearchJobsToolReturnsFullDescriptionsAsMarkdown(t *testing.T) {
	s := &recordingSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{hitDoc(1, "go-dev-acme", "Go Developer")},
		Total: 1,
	}}
	a := assistantWith(&searchHandlers{search: s, descriptions: fixedDescriptions{body: "<p>Full body</p>"}}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatalf("search_jobs: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "Full body") {
		t.Errorf("result = %s, want the rehydrated full description so one call screens the set", payload)
	}
	if strings.Contains(string(payload), "<p>") {
		t.Errorf("result = %s, want markdown rather than raw HTML", payload)
	}
}

func TestSearchJobsToolRejectsAnUnknownFacet(t *testing.T) {
	a := assistantWith(&searchHandlers{search: &recordingSearcher{}, descriptions: fixedDescriptions{}}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(`{"filters":{"vibe":["chill"]}}`))
	if err == nil {
		t.Fatal("an invented facet must be an error; filtering on it would silently return unfiltered results")
	}
	if !strings.Contains(err.Error(), "vibe") {
		t.Errorf("error = %v, want it to name the invented facet", err)
	}
}

func TestSearchJobsToolCapsTheResultCount(t *testing.T) {
	s := &recordingSearcher{}
	a := assistantWith(&searchHandlers{search: s, descriptions: fixedDescriptions{}}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"limit":500}`)); err != nil {
		t.Fatalf("search_jobs: %v", err)
	}
	if s.got.Limit > assistantMaxSearchLimit {
		t.Errorf("limit = %d, want it clamped to %d — full descriptions blow the context window", s.got.Limit, assistantMaxSearchLimit)
	}
}

func TestSearchJobsToolWithoutSearchConfiguredFails(t *testing.T) {
	a := assistantWith(&searchHandlers{}, nil)
	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`)); err == nil {
		t.Fatal("want an error the model can report when search is unconfigured")
	}
}

func TestFacetsToolReturnsTheVocabulary(t *testing.T) {
	fc := &stubFacets{res: search.FacetResult{
		Total:  120,
		Facets: map[string]map[string]int64{"skills": {"go": 40}, "enrichment.seniority": {"senior": 60}},
	}}
	a := assistantWith(&searchHandlers{facets: fc}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "facets")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("facets: %v", err)
	}
	payload, _ := json.Marshal(out)
	// Distributions must be keyed by the PUBLIC param name the search tool filters
	// by, not by the internal Meili attribute — otherwise the agent reads "seniority"
	// values under a key it cannot filter on.
	if !strings.Contains(string(payload), `"seniority"`) {
		t.Errorf("payload = %s, want public param names", payload)
	}
	if strings.Contains(string(payload), "enrichment.seniority") {
		t.Errorf("payload = %s, want the internal attribute hidden", payload)
	}
}

func TestMarketFitToolRequiresSkills(t *testing.T) {
	a := assistantWith(&searchHandlers{facets: &stubFacets{}}, &resumeHandlers{facets: &stubFacets{}})
	tool := toolByName(t, a.assistantDiscoveryTools(), "market_fit")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"skills":[]}`)); err == nil {
		t.Fatal("market_fit without skills must be an error, not an empty score")
	}
}

// stubFacets answers every facet query with the same canned result.
type stubFacets struct {
	res  search.FacetResult
	gotP []search.FacetParams
}

func (f *stubFacets) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	f.gotP = append(f.gotP, p)
	return f.res, nil
}

func (f *stubFacets) DisjunctiveFacetCounts(context.Context, string, []search.FacetReq, any) (search.FacetResult, error) {
	return f.res, nil
}

// assistantWith builds the agent over just the feature handlers a test needs. The
// assistant is a facade, so its tools are only ever as available as the handlers
// behind them — a nil one is exactly how production behaves when that feature is
// unconfigured.
func assistantWith(search *searchHandlers, resumeH *resumeHandlers) *assistantHandlers {
	if resumeH == nil {
		resumeH = &resumeHandlers{}
	}
	return &assistantHandlers{search: search, resume: resumeH}
}
