package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// tmtFake is a bespoke test client for the Job Market Finland adapter. The search URL is constant
// (region and page live in the POST body), so it decodes the body to serve the right region's
// page, and it serves details by the id in the GET URL. Unset regions/ids return an empty page /
// a not-found error, exercising the shard-dedup and detail-fallback paths.
type tmtFake struct {
	// regions maps a region code to its postings (a single page; page>0 returns empty).
	regions map[string][]tmtListItem
	// details maps a posting id to its detail JSON; a missing id fails the GET.
	details map[string]string
}

func (f *tmtFake) PostJSON(_ context.Context, _ string, body, v any) error {
	raw, _ := json.Marshal(body)
	var req tmtSearchBody
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	resp := tmtSearchResponse{LastPage: 0}
	if req.Paging.PageNumber == 0 && len(req.Filters.Regions) == 1 {
		resp.Content = f.regions[req.Filters.Regions[0]]
	}
	out, _ := json.Marshal(resp)
	return json.Unmarshal(out, v)
}

func (f *tmtFake) GetJSON(_ context.Context, url string, v any) error {
	id := url[strings.LastIndex(url, "/")+1:]
	body, ok := f.details[id]
	if !ok {
		return fmt.Errorf("tmtFake: no detail for %s", id)
	}
	return json.Unmarshal([]byte(body), v)
}

func listItem(id, region, titleFI, company string) tmtListItem {
	it := tmtListItem{ID: id, PublishDate: "2026-07-15T17:55:01.597Z"}
	it.Title = tmtLang{"fi": titleFI}
	it.Employer.OwnerName = tmtLang{"fi": company}
	it.Location.Municipalities = []struct {
		Label tmtLang `json:"label"`
	}{{Label: tmtLang{"fi": "Helsinki", "en": "Helsinki"}}}
	_ = region
	return it
}

func detailJSON(titleEN, company, contentType, body string) string {
	return fmt.Sprintf(`{
		"descriptionsContentType": %q,
		"position": {"title": {"en": %q}, "jobDescription": {"en": %q}},
		"owner": {"company": {"en": %q}},
		"application": {"published": "2026-07-10T09:00:00Z"}
	}`, contentType, titleEN, body, company)
}

func TestTyomarkkinatoriProvider(t *testing.T) {
	if got := NewTyomarkkinatori(nil).Provider(); got != "tyomarkkinatori" {
		t.Errorf("Provider() = %q, want %q", got, "tyomarkkinatori")
	}
}

func TestTyomarkkinatoriBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/tyomarkkinatori.yml")
	if err != nil {
		t.Fatalf("LoadConfig(sources/tyomarkkinatori.yml): %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/tyomarkkinatori.yml fails registry validation: %v", err)
	}
}

func TestTyomarkkinatoriIsBoardlessAggregator(t *testing.T) {
	s := NewTyomarkkinatori(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("tyomarkkinatori must be boardless (no per-tenant board id)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("tyomarkkinatori must be an aggregator (stays in the source facet)")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("tyomarkkinatori must be a HydratingSource (detail only for new postings)")
	}
}

func TestTyomarkkinatoriRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["tyomarkkinatori"]
	if !ok {
		t.Fatal("All() missing provider tyomarkkinatori")
	}
	if s.Provider() != "tyomarkkinatori" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	// Multi-company aggregator: filtering by it is meaningful, so it stays in the source facet.
	if !slices.Contains(FilterableProviders(), "tyomarkkinatori") {
		t.Error("FilterableProviders() should include the aggregator tyomarkkinatori")
	}
}

func TestTyomarkkinatoriLangPick(t *testing.T) {
	if got := (tmtLang{"fi": "Kokki", "en": "Cook"}).pick(); got != "Cook" {
		t.Errorf("pick() = %q, want English preferred", got)
	}
	if got := (tmtLang{"sv": "Kock", "fi": "Kokki"}).pick(); got != "Kokki" {
		t.Errorf("pick() = %q, want Finnish over Swedish", got)
	}
	if got := (tmtLang{}).pick(); got != "" {
		t.Errorf("pick() = %q, want empty", got)
	}
	if got := (tmtLang{"fi": "x"}).lang(); got != "fi" {
		t.Errorf("lang() = %q, want fi", got)
	}
	if got := (tmtLang{"en": "x", "fi": "y"}).lang(); got != "en" {
		t.Errorf("lang() = %q, want en", got)
	}
}

// TestTyomarkkinatoriFetchShardsAndMaps verifies the region-shard crawl: a posting returned by two
// region shards is kept once, and its fields come from the list plus the detail (detail title,
// employer, and published date win).
func TestTyomarkkinatoriFetchShardsAndMaps(t *testing.T) {
	shared := listItem("a1", "01", "Ohjelmistokehittäjä", "Bambu Food Oy")
	fake := &tmtFake{
		regions: map[string][]tmtListItem{
			"01": {shared, listItem("a2", "01", "Kokki", "Ravintola Oy")},
			"06": {shared}, // same posting surfaces in a second region — must dedup
		},
		details: map[string]string{
			"a1": detailJSON("Software Developer", "Hyperion Robotics", "plain", "Build things."),
			"a2": detailJSON("Cook", "Ravintola Oy", "plain", "Cook things."),
		},
	}

	jobs, err := NewTyomarkkinatori(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (deduped across shards)", len(jobs))
	}
	a1, ok := jobByID(jobs, "a1")
	if !ok {
		t.Fatal("posting a1 missing")
	}
	if a1.Title != "Software Developer" {
		t.Errorf("Title = %q, want detail title", a1.Title)
	}
	if a1.Company != "Hyperion Robotics" {
		t.Errorf("Company = %q, want detail owner.company", a1.Company)
	}
	if a1.URL != "https://tyomarkkinatori.fi/henkiloasiakkaat/avoimet-tyopaikat/a1/fi" {
		t.Errorf("URL = %q", a1.URL)
	}
	if a1.Location != "Helsinki, Finland" {
		t.Errorf("Location = %q, want municipality + Finland", a1.Location)
	}
	if a1.PostedAt == nil || !a1.PostedAt.Equal(time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want detail published 2026-07-10T09:00:00Z", a1.PostedAt)
	}
}

// TestTyomarkkinatoriFetchNewHydratesOnlyNew verifies the HydratingSource contract: a seen posting
// is refreshed (SeenRefresh, no detail request, empty description), an unseen one is hydrated.
func TestTyomarkkinatoriFetchNewHydratesOnlyNew(t *testing.T) {
	fake := &tmtFake{
		regions: map[string][]tmtListItem{
			"01": {listItem("old", "01", "Vanha", "Acme Oy"), listItem("new", "01", "Uusi", "Beta Oy")},
		},
		details: map[string]string{
			// Only "new" has a detail; "old" must never be fetched (it is seen).
			"new": detailJSON("New Role", "Beta Oy", "plain", "Fresh body."),
		},
	}
	seen := func(id string) bool { return id == "old" }

	jobs, err := NewTyomarkkinatori(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	old, oldOK := jobByID(jobs, "old")
	nu, nuOK := jobByID(jobs, "new")
	if !oldOK || !old.SeenRefresh {
		t.Errorf("seen posting should be SeenRefresh, got %+v (ok=%v)", old, oldOK)
	}
	if !nuOK || nu.SeenRefresh {
		t.Errorf("unseen posting should be hydrated, got %+v (ok=%v)", nu, nuOK)
	}
	if nuOK && nu.Title != "New Role" {
		t.Errorf("unseen Title = %q, want hydrated detail title", nu.Title)
	}
}

// TestTMTDescription pins the contract for the description renderer (the learning contribution).
//
// The body is stored as sanitized HTML. It arrives in two shapes, flagged by contentType:
//   - "markdown": CommonMark — render it (markdownToHTML) so "**bold**" etc. become real tags.
//   - "plain": literal text whose SINGLE newlines are meaningful (postings lay out
//     "Location: …\nWork mode: …\n" line by line). Feeding plain text straight through a
//     CommonMark renderer collapses those single newlines into spaces (soft breaks) and merges the
//     lines — the trade-off to resolve. Preserve the line structure in the rendered HTML.
//
// An empty body renders to "". The output is always sanitized HTML.
func TestTMTDescription(t *testing.T) {
	// Empty stays empty regardless of content type.
	if got := tmtDescription("plain", "  "); got != "" {
		t.Errorf("tmtDescription(plain, blank) = %q, want empty", got)
	}

	// Markdown is rendered: bold becomes a tag, not literal asterisks.
	md := tmtDescription("markdown", "We need a **developer**.")
	if strings.Contains(md, "**") {
		t.Errorf("markdown not rendered: %q", md)
	}
	if !strings.Contains(md, "developer") {
		t.Errorf("markdown lost content: %q", md)
	}

	// Plain text keeps its two lines distinct — they must not be merged into one run of text.
	plain := tmtDescription("plain", "Location: Espoo\nWork mode: Hybrid")
	if !strings.Contains(plain, "Location: Espoo") || !strings.Contains(plain, "Work mode: Hybrid") {
		t.Errorf("plain lost content: %q", plain)
	}
	if strings.Contains(plain, "Location: Espoo Work mode: Hybrid") {
		t.Errorf("plain merged its two lines into one: %q", plain)
	}
}
