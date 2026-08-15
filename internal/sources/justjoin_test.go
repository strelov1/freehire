package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestJustJoinHydrateMapsDetail verifies the per-offer detail fetch + mapping: the sanitized
// body becomes the description and the structured facets justjoin states unambiguously
// (skills, seniority) map into freehire's vocabularies. justjoin's "mid" means "middle".
func TestJustJoinHydrateMapsDetail(t *testing.T) {
	detail := `{
"body":"<p>Build things.</p><script>alert(1)</script>",
"requiredSkills":[{"name":"TypeScript","level":3},{"name":"Node.js","level":3}],
"experienceLevel":{"label":"Mid","value":"mid"}
}`
	fake := (&routedHTTP{}).route("/v1/offers/acme-go-dev", detail)
	s := justjoin{http: fake}

	d, ok := s.detail(context.Background(), "acme-go-dev")
	if !ok {
		t.Fatal("detail() ok=false, want a successful fetch")
	}
	got := d.apply(Job{Title: "Go Developer", Company: "Acme"})

	if !strings.Contains(got.Description, "Build things.") || strings.Contains(got.Description, "<script>") {
		t.Errorf("Description not sanitized/assembled: %q", got.Description)
	}
	if !slices.Contains(got.Skills, "typescript") || !slices.Contains(got.Skills, "nodejs") {
		t.Errorf("Skills = %v, want canonical typescript+nodejs", got.Skills)
	}
	if got.Seniority != "middle" {
		t.Errorf("Seniority = %q, want middle (justjoin mid)", got.Seniority)
	}
}

// justjoin's required_skills entries are already-discrete asserted names, not prose — a lone
// ambiguous word like "React" (which Parse's corroboration rule would drop without a second
// strong tech term in the same text) must still resolve, because the platform is the one
// asserting it, not a sentence justJoinSkills is mining.
func TestJustJoinSkillsResolvesALoneAmbiguousSkill(t *testing.T) {
	got := justJoinSkills([]justJoinSkill{{Name: "React"}})
	if !slices.Contains(got, "react") {
		t.Errorf("justJoinSkills([React]) = %v, want [react] — a platform-asserted skill must not need corroboration", got)
	}
}

func TestJustJoinProvider(t *testing.T) {
	if got := NewJustJoin(nil).Provider(); got != "justjoin" {
		t.Errorf("Provider() = %q, want justjoin", got)
	}
}

func TestJustJoinIsBoardlessAggregator(t *testing.T) {
	s := NewJustJoin(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("justjoin should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("justjoin should implement the aggregator marker")
	}
}

func TestJustJoinIsHydratingSource(t *testing.T) {
	if _, ok := NewJustJoin(nil).(HydratingSource); !ok {
		t.Error("justjoin should implement the HydratingSource marker")
	}
}

func TestJustJoinRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["justjoin"]; !ok {
		t.Error("All() should register provider justjoin")
	}
	if !slices.Contains(FilterableProviders(), "justjoin") {
		t.Error("FilterableProviders() should include justjoin")
	}
}

func TestJustJoinBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/justjoin.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/justjoin.yml fails validation: %v", err)
	}
}

func TestJustJoinFetchNewHydratesOnlyUnseen(t *testing.T) {
	list := `{"data":[
{"guid":"new-1","slug":"acme-go-dev--krakow","title":"Go Developer","companyName":"Acme","city":"Kraków","workplaceType":"remote","publishedAt":"2026-05-18T10:00:00.000Z"},
{"guid":"seen-1","slug":"coder-security--warszawa","title":"Security Engineer","companyName":"Coder","city":"Warszawa","workplaceType":"office","publishedAt":"2026-05-19T00:41:27.040Z"}
],"meta":{"next":null}}`
	detail := `{"body":"<p>Build things.</p>","requiredSkills":[{"name":"Go"}],"experienceLevel":{"value":"senior"}}`
	fake := (&routedHTTP{}).route("/v1/offers/acme-go-dev--krakow", detail).route("by-cursor", list)
	seen := func(id string) bool { return id == "seen-1" }

	jobs, err := justjoin{http: fake}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if got := byID["new-1"]; !strings.Contains(got.Description, "Build things.") || got.Seniority != "senior" {
		t.Errorf("unseen offer not hydrated: desc=%q seniority=%q", got.Description, got.Seniority)
	}
	if got := byID["seen-1"]; got.Description != "" || !got.SeenRefresh {
		t.Errorf("seen offer should be a liveness-refresh with no content, got description=%q SeenRefresh=%v", got.Description, got.SeenRefresh)
	}
	if byID["new-1"].SeenRefresh {
		t.Error("hydrated new offer must not be marked SeenRefresh")
	}
	// One detail request for the single unseen offer (plus the one list page).
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (1 list + 1 detail for the unseen offer)", fake.calls)
	}
}

func TestJustJoinFetchNewIsolatesDetailFailure(t *testing.T) {
	list := `{"data":[
{"guid":"new-1","slug":"acme-go-dev--krakow","title":"Go Developer","companyName":"Acme","city":"Kraków","workplaceType":"remote","publishedAt":"2026-05-18T10:00:00.000Z"}
],"meta":{"next":null}}`
	// No detail route → the detail fetch errors; the offer must still be ingested list-only.
	fake := (&routedHTTP{}).route("by-cursor", list)
	seen := func(string) bool { return false }

	jobs, err := justjoin{http: fake}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew must not abort on a single detail failure: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (list-only fallback)", len(jobs))
	}
	if jobs[0].Description != "" {
		t.Errorf("detail failed, want empty description, got %q", jobs[0].Description)
	}
}

func TestJustJoinFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"guid":"135d4e26","slug":"coder-security-engineer--warszawa","title":"Security Engineer","companyName":"Coder","city":"Warszawa","workplaceType":"remote","publishedAt":"2026-05-19T00:41:27.040Z"},
{"guid":"noslug","slug":"","title":"drop","companyName":"Ghost"}
],"meta":{"next":{"cursor":20}}}`
	page2 := `{"data":[
{"guid":"77aa","slug":"acme-go-dev--krakow","title":"Go Developer","companyName":"Acme","city":"Kraków","workplaceType":"office","publishedAt":"2026-05-18T10:00:00.000Z"}
],"meta":{"next":null}}`
	fake := (&routedHTTP{}).route("from=20", page2).route("by-cursor", page1)
	jobs, err := NewJustJoin(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per cursor page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (empty-slug dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "135d4e26" || j.Company != "Coder" || j.Title != "Security Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://justjoin.it/job-offer/coder-security-engineer--warszawa" {
		t.Errorf("URL = %q, want synthesized job-offer URL", j.URL)
	}
	if j.Location != "Warszawa" {
		t.Errorf("Location = %q, want city", j.Location)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (from workplaceType)", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339 with millis")
	}
	// Second page's "office" workplaceType maps to onsite.
	if jobs[1].WorkMode != "onsite" {
		t.Errorf("jobs[1].WorkMode = %q, want onsite (office)", jobs[1].WorkMode)
	}
}
