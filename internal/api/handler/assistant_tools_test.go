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
	// The Claude directory policy keeps cross-tool guidance out of a tool's description.
	for _, other := range mcpKeyTools(a, testMCPOrigin) {
		if other.Name != tool.Name && strings.Contains(tool.Description, other.Name) {
			t.Errorf("description names %s: %q", other.Name, tool.Description)
		}
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
	if got := fmt.Sprint(s.got.Filter); !strings.Contains(got, "remote") || !strings.Contains(got, "senior") {
		t.Errorf("filter = %v, want both work_mode remote and seniority senior applied", s.got.Filter)
	}

	for _, bad := range []string{`42`, `["remote",42]`} {
		_, err := tool.Run(context.Background(), 3, json.RawMessage(`{"filters":{"work_mode":`+bad+`}}`))
		if err == nil || !strings.Contains(err.Error(), "work_mode") || !strings.Contains(err.Error(), bad) {
			t.Errorf("work_mode %s: error = %v, want it refused naming the filter and the value sent", bad, err)
		}
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

// Problem report #17: an agent enumerating employers asked for company_slug and was refused.
func TestMCPFacetsListsTheBusiestCompanies(t *testing.T) {
	fc := &stubFacets{res: search.FacetResult{
		Total: 90,
		Facets: map[string]map[string]int64{
			"company_slug": {"acme-corp": 40, "acme-labs": 10, "globex": 30},
		},
	}}
	a := assistantWith(&searchHandlers{facets: fc}, nil)
	tool := toolByName(t, mcpKeyTools(a, testMCPOrigin), "facets")
	run := func(args string) map[string]any {
		t.Helper()
		out, err := tool.Run(context.Background(), 3, json.RawMessage(args))
		if err != nil {
			t.Fatalf("facets %s: %v", args, err)
		}
		return out.(map[string]any)
	}

	got := run(`{"facet":"company_slug"}`)["companies"].([]companyFacetValue)
	if len(got) != 3 || got[0].Slug != "acme-corp" || got[0].Count != 40 || got[0].Name != "acme-corp" {
		t.Errorf("companies = %+v, want all three busiest first, named by slug without a names source", got)
	}
	if asked := fc.gotP[0].Facets; len(asked) != 1 || asked[0] != "company_slug" {
		t.Errorf("asked the index for %v, want only company_slug", asked)
	}
	// A space in the text matches the slug's hyphen.
	res := run(`{"facet":"company_slug","contains":"Acme Corp"}`)
	if got := res["companies"].([]companyFacetValue); len(got) != 1 || got[0].Slug != "acme-corp" {
		t.Errorf("contains=Acme Corp companies = %+v, want acme-corp", got)
	}
	if res["matching_companies"] != 1 {
		t.Errorf("matching_companies = %v, want 1", res["matching_companies"])
	}
}

// An MCP client keeps every answer in its context, and the whole vocabulary ran to ~125 KB
// (2026-11-05): four long tails of up to 1,200 values each. The MCP facets names the
// filters with their sizes, and lists one filter's values on request.
func TestMCPFacetsNamesTheFiltersAndListsOneOnRequest(t *testing.T) {
	fc := &stubFacets{res: search.FacetResult{
		Total: 120,
		Facets: map[string]map[string]int64{
			"skills":               {"go": 40, "golang-migrate": 3, "python": 30},
			"enrichment.seniority": {"senior": 60},
		},
		Stats: map[string]search.FacetStat{"enrichment.salary_min": {Min: 1000, Max: 9000}},
	}}
	a := assistantWith(&searchHandlers{facets: fc}, nil)
	tool := toolByName(t, mcpKeyTools(a, testMCPOrigin), "facets")
	run := func(args string) map[string]any {
		t.Helper()
		out, err := tool.Run(context.Background(), 3, json.RawMessage(args))
		if err != nil {
			t.Fatalf("facets %s: %v", args, err)
		}
		return out.(map[string]any)
	}

	index := run(`{}`)
	if sizes := index["filters"].(map[string]int); sizes["skills"] != 3 || sizes["seniority"] != 1 {
		t.Errorf("filters = %v, want each public filter name with its number of values", sizes)
	}
	sizes := index["filters"].(map[string]int)
	if n, ok := sizes["metros"]; !ok || n != 0 {
		t.Errorf("filters = %v, want metros named with 0 although no matching posting carries one", sizes)
	}
	if _, ok := sizes["salary_min"]; ok {
		t.Errorf("filters = %v, want numeric facets left to stats", sizes)
	}
	if payload, _ := json.Marshal(index); strings.Contains(string(payload), "python") {
		t.Errorf("index = %s, want no values without a facet", payload)
	}

	fc.gotP = nil
	if got := run(`{"facet":"skills"}`)["values"].(map[string]int64); len(got) != 3 {
		t.Errorf("facet=skills values = %v, want all three", got)
	}
	if asked := fc.gotP[0].Facets; len(asked) != 1 || asked[0] != "skills" {
		t.Errorf("facet=skills asked the index for %v, want only the skills attribute", asked)
	}
	// "golang" is an alias of "go": the alias finds the value the search filters by.
	got := run(`{"facet":"skills","contains":"golang"}`)["values"].(map[string]int64)
	if _, ok := got["go"]; !ok || len(got) != 2 {
		t.Errorf("contains=golang values = %v, want go (by alias) and golang-migrate (by text)", got)
	}

	if got := run(`{"facet":"salary_min"}`)["stats"].(*search.FacetStat); got == nil || got.Max != 9000 {
		t.Errorf("facet=salary_min stats = %+v, want the numeric range", got)
	}
	// No matching posting states a salary_max: still a numeric answer, with no range.
	if got := run(`{"facet":"salary_max"}`); got["stats"].(*search.FacetStat) != nil || got["values"] != nil {
		t.Errorf("facet=salary_max = %v, want stats with no range and no values", got)
	}

	for _, unlisted := range []string{"vibe"} {
		_, err := tool.Run(context.Background(), 3, json.RawMessage(`{"facet":"`+unlisted+`"}`))
		if err == nil || !strings.Contains(err.Error(), unlisted) || !strings.Contains(err.Error(), "salary_min") {
			t.Errorf("facet=%s error = %v, want it refused, naming it and the valid ones", unlisted, err)
		}
	}
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"facet":"vibe"}`)); err == nil ||
		!strings.Contains(err.Error(), "vibe") || !strings.Contains(err.Error(), "skills") {
		t.Errorf("error = %v, want an unknown facet refused, naming it and the valid ones", err)
	}

	// The in-app assistant's own copy still lists every value.
	full, _ := toolByName(t, a.assistantDiscoveryTools(), "facets").Run(context.Background(), 3, json.RawMessage(`{}`))
	if got := len(full.(facetPayload).Facets["skills"]); got != 3 {
		t.Errorf("in-app facets lists %d skills, want all 3", got)
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
