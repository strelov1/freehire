package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
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

// An MCP client keeps every result in its context window: ten full descriptions were a
// ~130 KB answer (measured 2026-11-05), so MCP gets each one's opening and get_job reads.
func TestMCPSearchJobsCarriesOnlyTheOpeningOfEachDescription(t *testing.T) {
	s := &recordingSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{hitDoc(1, "go-dev-acme", "Go Developer")},
		Total: 1,
	}}
	hit := s.res.Hits[0]
	hit.Description = "<p>Opening line.</p><p>" + strings.Repeat("tail ", 200) + "</p>"
	s.res.Hits[0] = hit
	a := assistantWith(&searchHandlers{search: s, descriptions: fixedDescriptions{body: "<p>Full body from Postgres</p>"}}, nil)

	tool := toolByName(t, mcpKeyTools(a, testMCPOrigin), "search_jobs")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatalf("search_jobs: %v", err)
	}
	jobs := out.(map[string]any)["jobs"].([]jobview.Job)
	got := jobs[0].Description
	if !strings.HasPrefix(got, "Opening line.") || strings.Contains(got, "<p>") {
		t.Errorf("description = %q, want the index preview's opening as markdown", got)
	}
	if n := len([]rune(got)); n > mcpSearchExcerptRunes+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("description has %d runes (%q), want at most %d and marked as cut", n, got, mcpSearchExcerptRunes)
	}
	payload, _ := json.Marshal(out)
	if strings.Contains(string(payload), "Postgres") {
		t.Errorf("result = %s, want no full-text rehydration on the MCP listing", payload)
	}
	if !strings.Contains(string(payload), "go-dev-acme") {
		t.Errorf("result = %s, want the slug get_job reads by", payload)
	}
	if !strings.Contains(tool.Description, "get_job") {
		t.Errorf("description = %q, want it to send the agent to get_job for the text", tool.Description)
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

// Models send a one-value filter as a bare string; on the account MCP server that failed
// every such call before the search ran.
func TestSearchJobsToolReadsABareStringFilterAsOneValue(t *testing.T) {
	s := &recordingSearcher{}
	a := assistantWith(&searchHandlers{search: s, descriptions: fixedDescriptions{}}, nil)

	tool := toolByName(t, a.assistantDiscoveryTools(), "search_jobs")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"filters":{"work_mode":"remote","seniority":["senior"],"regions":null}}`)); err != nil {
		t.Fatalf("search_jobs: %v", err)
	}
	if s.got.Filter == nil || !strings.Contains(fmt.Sprint(s.got.Filter), "remote") {
		t.Errorf("filter = %v, want work_mode remote applied", s.got.Filter)
	}

	_, err := tool.Run(context.Background(), 3, json.RawMessage(`{"filters":{"work_mode":42}}`))
	if err == nil || !strings.Contains(err.Error(), "work_mode") {
		t.Errorf("error = %v, want a non-string, non-list value refused by name", err)
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

// The model reads the filter's keys from the schema, so the schema must name every
// facet values() accepts and admit nothing else — an open map is what let it guess
// `specializations` for `category`.
func TestFilterSchemaDeclaresEveryFacetAndNothingElse(t *testing.T) {
	schema := assistantFilterSchema("x")
	if schema["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v, want false", schema["additionalProperties"])
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) != len(search.StringFacets) {
		t.Errorf("schema declares %d facets, search accepts %d", len(props), len(search.StringFacets))
	}
	for name := range search.StringFacets {
		if _, ok := props[name]; !ok {
			t.Errorf("facet %q is accepted by values() but missing from the schema", name)
		}
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

func (f *stubFacets) DistinctCount(context.Context, string, any, string) (int64, error) {
	return 0, nil
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
