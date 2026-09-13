package sources

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// wellfoundFixture wraps a normalized-Apollo-cache "data" object (as it appears at
// props.pageProps.apolloState.data on a real Wellfound role-search page) in the same minimal
// __NEXT_DATA__ script shape the real page carries, then parses it exactly as GetHTML would.
// dataJSON is the literal JSON object body (without the outer braces already implied by the
// wrapper) for props.pageProps.apolloState.data.
func wellfoundFixture(t *testing.T, dataJSON string) *html.Node {
	t.Helper()
	page := `<!DOCTYPE html><html><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"apolloState":{"data":` + dataJSON + `}}}}` +
		`</script></body></html>`
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("wellfoundFixture: parse: %v", err)
	}
	return root
}

// wellfoundUnparseableFixture is a page whose __NEXT_DATA__ script is present but not valid
// JSON at all, for the "unparseable page fails loudly" scenario.
func wellfoundUnparseableFixture(t *testing.T) *html.Node {
	t.Helper()
	page := `<!DOCTYPE html><html><body>` +
		`<script id="__NEXT_DATA__" type="application/json">{not valid json</script>` +
		`</body></html>`
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("wellfoundUnparseableFixture: parse: %v", err)
	}
	return root
}

// wellfoundOnePage is a small but real-shaped page: two startups, three job listings, one of
// which ("Orphan Role" / id 103) is not referenced by any startup's highlightedJobListings at
// all — the genuine "unresolvable company" shape, since a JobListingSearchResult carries no
// forward reference to its hiring company (see design.md's Context section).
const wellfoundOnePage = `{
  "ROOT_QUERY": {
    "__typename": "Query",
    "talent": {
      "__typename": "Talent",
      "seoLandingPageJobSearchResults({\"page\":1,\"remote\":true,\"role\":\"software-engineer\"})": {
        "__typename": "Results",
        "pageCount": 2,
        "totalJobCount": 4,
        "totalStartupCount": 2,
        "perPage": 20,
        "startups": [{"__ref": "StartupResult:1"}, {"__ref": "StartupResult:2"}]
      }
    }
  },
  "StartupResult:1": {
    "__typename": "StartupResult",
    "id": "1",
    "name": "Acme Inc",
    "slug": "acme-inc",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:100"}]
  },
  "StartupResult:2": {
    "__typename": "StartupResult",
    "id": "2",
    "name": "Widget Co",
    "slug": "widget-co",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:101"}]
  },
  "JobListingSearchResult:100": {
    "__typename": "JobListingSearchResult",
    "id": "100",
    "slug": "senior-backend-engineer",
    "title": "Senior Backend Engineer",
    "description": "<p>Build things.</p>",
    "compensation": "$150k – $180k",
    "remote": true,
    "locationNames": ["United States"]
  },
  "JobListingSearchResult:101": {
    "__typename": "JobListingSearchResult",
    "id": "101",
    "slug": "product-designer",
    "title": "Product Designer",
    "description": "<p>Design things.</p>",
    "compensation": "",
    "remote": false,
    "locationNames": ["Berlin"]
  },
  "JobListingSearchResult:103": {
    "__typename": "JobListingSearchResult",
    "id": "103",
    "slug": "orphan-role",
    "title": "Orphan Role",
    "description": "<p>Nobody highlights this one.</p>",
    "compensation": "",
    "remote": true,
    "locationNames": []
  },
  "JobListingSearchResult:104": "not an object at all, so this entry must be dropped"
}`

// --- 2. __NEXT_DATA__ / Apollo cache parsing -------------------------------------------------

func TestParseWellfoundPage_ExtractsApolloState(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	if len(page.Jobs) == 0 {
		t.Fatal("parseWellfoundPage: got no jobs, want at least one")
	}
}

func TestParseWellfoundPage_WalksEveryJobListingEntry(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	// Three JobListingSearchResult keys are well-formed objects (100, 101, 103); 104 is not an
	// object and must be silently dropped rather than aborting the page.
	if len(page.Jobs) != 3 {
		t.Fatalf("len(page.Jobs) = %d, want 3 (104 is malformed and must be dropped)", len(page.Jobs))
	}
	var titles []string
	for _, j := range page.Jobs {
		titles = append(titles, j.Entry.Title)
	}
	slices.Sort(titles)
	want := []string{"Orphan Role", "Product Designer", "Senior Backend Engineer"}
	if !slices.Equal(titles, want) {
		t.Errorf("titles = %v, want %v", titles, want)
	}
}

func TestParseWellfoundPage_PopulatesEntryScalarFields(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	var backend *wellfoundResolvedJob
	for i := range page.Jobs {
		if page.Jobs[i].Entry.ID == "100" {
			backend = &page.Jobs[i]
		}
	}
	if backend == nil {
		t.Fatal("job id 100 not found")
	}
	e := backend.Entry
	if e.Slug != "senior-backend-engineer" {
		t.Errorf("Slug = %q, want senior-backend-engineer", e.Slug)
	}
	if e.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Description != "<p>Build things.</p>" {
		t.Errorf("Description = %q", e.Description)
	}
	if e.Compensation != "$150k – $180k" {
		t.Errorf("Compensation = %q", e.Compensation)
	}
	if !e.Remote {
		t.Error("Remote = false, want true")
	}
	if !slices.Equal(e.LocationNames, []string{"United States"}) {
		t.Errorf("LocationNames = %v", e.LocationNames)
	}
}

func TestParseWellfoundPage_ResolvesCompanyViaStartupReverseIndex(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	byID := map[string]wellfoundResolvedJob{}
	for _, j := range page.Jobs {
		byID[j.Entry.ID] = j
	}
	if got := byID["100"].Company; got != "Acme Inc" {
		t.Errorf("job 100 Company = %q, want Acme Inc (via StartupResult:1's highlightedJobListings)", got)
	}
	if got := byID["101"].Company; got != "Widget Co" {
		t.Errorf("job 101 Company = %q, want Widget Co", got)
	}
}

func TestParseWellfoundPage_JobNotReferencedByAnyStartupHasNoCompany(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	for _, j := range page.Jobs {
		if j.Entry.ID == "103" {
			if j.Company != "" {
				t.Errorf("job 103 (orphan) Company = %q, want empty — no StartupResult references it", j.Company)
			}
			return
		}
	}
	t.Fatal("job 103 not found in parsed page")
}

func TestParseWellfoundPage_ReadsPageCountAndTotalJobCount(t *testing.T) {
	root := wellfoundFixture(t, wellfoundOnePage)
	page, err := parseWellfoundPage(root)
	if err != nil {
		t.Fatalf("parseWellfoundPage: %v", err)
	}
	if page.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", page.PageCount)
	}
	if page.TotalJobCount != 4 {
		t.Errorf("TotalJobCount = %d, want 4", page.TotalJobCount)
	}
}

// TestParseWellfoundPage_CompanyResolutionIsDeterministic guards against a real hazard in
// wellfoundCompanyIndex: it is built by ranging a Go map (StartupResult entries), whose
// iteration order is randomized per run. If a job were ever referenced by more than one
// startup's highlightedJobListings (not expected in real Wellfound data, but not something the
// JSON schema forbids either), an index built by unconditional last-write-wins would resolve
// that job's Company to a different name on different runs of the SAME input — silently
// churning content_hash and misattributing the employer at random. This asserts the resolution
// is stable across repeated parses of an input that deliberately has two startups both
// referencing the same job id.
func TestParseWellfoundPage_CompanyResolutionIsDeterministic(t *testing.T) {
	const conflicted = `{
  "ROOT_QUERY": {
    "__typename": "Query",
    "talent": {
      "__typename": "Talent",
      "seoLandingPageJobSearchResults({\"page\":1,\"remote\":true,\"role\":\"software-engineer\"})": {
        "__typename": "Results",
        "pageCount": 1,
        "totalJobCount": 1,
        "totalStartupCount": 2,
        "perPage": 20,
        "startups": [{"__ref": "StartupResult:1"}, {"__ref": "StartupResult:2"}]
      }
    }
  },
  "StartupResult:1": {
    "__typename": "StartupResult",
    "id": "1",
    "name": "Zeta Corp",
    "slug": "zeta-corp",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:200"}]
  },
  "StartupResult:2": {
    "__typename": "StartupResult",
    "id": "2",
    "name": "Acme Inc",
    "slug": "acme-inc",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:200"}]
  },
  "JobListingSearchResult:200": {
    "__typename": "JobListingSearchResult",
    "id": "200",
    "slug": "conflicted-job",
    "title": "Conflicted Job",
    "description": "<p>Body.</p>",
    "compensation": "",
    "remote": true,
    "locationNames": []
  }
}`
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		root := wellfoundFixture(t, conflicted)
		page, err := parseWellfoundPage(root)
		if err != nil {
			t.Fatalf("parseWellfoundPage: %v", err)
		}
		for _, j := range page.Jobs {
			if j.Entry.ID == "200" {
				seen[j.Company] = true
			}
		}
	}
	if len(seen) != 1 {
		t.Errorf("job 200 resolved to %d distinct companies across repeated parses (%v), want exactly 1 — resolution must be deterministic", len(seen), seen)
	}
}

func TestParseWellfoundPage_UnparseablePageIsAnError(t *testing.T) {
	root := wellfoundUnparseableFixture(t)
	_, err := parseWellfoundPage(root)
	if err == nil {
		t.Fatal("parseWellfoundPage: got nil error for an unparseable page, want an error")
	}
}

func TestParseWellfoundPage_MissingNextDataIsAnError(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<html><body><p>no script here</p></body></html>`))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if _, err := parseWellfoundPage(root); err == nil {
		t.Fatal("parseWellfoundPage: got nil error when __NEXT_DATA__ is entirely absent, want an error")
	}
}

// --- 3. Board URL and pagination --------------------------------------------------------------

func TestWellfoundPageURL(t *testing.T) {
	got := wellfoundPageURL("software-engineer", 1)
	want := "https://wellfound.com/role/r/software-engineer?page=1"
	if got != want {
		t.Errorf("wellfoundPageURL(%q, 1) = %q, want %q", "software-engineer", got, want)
	}
	if got := wellfoundPageURL("data-scientist", 7); got != "https://wellfound.com/role/r/data-scientist?page=7" {
		t.Errorf("wellfoundPageURL(%q, 7) = %q", "data-scientist", got)
	}
}

// wellfoundFakeHTTP is a route-by-URL fake: each configured URL answers either a data payload
// (wrapped into a fixture page) or an error.
type wellfoundFakeHTTP struct {
	pages map[string]string // url -> apolloState "data" JSON
	errs  map[string]error
	t     *testing.T
	got   []string
}

func (f *wellfoundFakeHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.got = append(f.got, url)
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	data, ok := f.pages[url]
	if !ok {
		return nil, fmt.Errorf("wellfoundFakeHTTP: no page configured for %s", url)
	}
	return wellfoundFixture(f.t, data), nil
}

// wellfoundTwoPageData builds a two-startup, one-job page whose stated pageCount/totalJobCount
// can be set independently of the actual number of pages the fake serves, so pagination tests
// can assert the adapter trusts the stated total rather than merely stopping on an empty page.
func wellfoundTwoPageData(page, pageCount, totalJobCount int, jobID, jobTitle string) string {
	return fmt.Sprintf(`{
  "ROOT_QUERY": {
    "__typename": "Query",
    "talent": {
      "__typename": "Talent",
      "seoLandingPageJobSearchResults({\"page\":%d,\"remote\":true,\"role\":\"software-engineer\"})": {
        "__typename": "Results",
        "pageCount": %d,
        "totalJobCount": %d,
        "totalStartupCount": 1,
        "perPage": 20,
        "startups": [{"__ref": "StartupResult:1"}]
      }
    }
  },
  "StartupResult:1": {
    "__typename": "StartupResult",
    "id": "1",
    "name": "Acme Inc",
    "slug": "acme-inc",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:%s"}]
  },
  "JobListingSearchResult:%s": {
    "__typename": "JobListingSearchResult",
    "id": "%s",
    "slug": "job-%s",
    "title": "%s",
    "description": "<p>Body.</p>",
    "compensation": "",
    "remote": true,
    "locationNames": ["United States"]
  }
}`, page, pageCount, totalJobCount, jobID, jobID, jobID, jobID, jobTitle)
}

func TestWellfound_Fetch_PaginatesToTheStatedPageCount(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	url2 := wellfoundPageURL("software-engineer", 2)
	fake := &wellfoundFakeHTTP{
		t: t,
		pages: map[string]string{
			url1: wellfoundTwoPageData(1, 2, 2, "200", "First Page Job"),
			url2: wellfoundTwoPageData(2, 2, 2, "201", "Second Page Job"),
		},
	}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Contains(fake.got, url1) || !slices.Contains(fake.got, url2) {
		t.Errorf("got requests %v, want both page 1 and page 2 fetched", fake.got)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one per page)", len(jobs))
	}
}

func TestWellfound_Fetch_DoesNotFetchPastTheStatedPageCount(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{
		t: t,
		pages: map[string]string{
			// pageCount is 1: a page 2 fetch would fail (unconfigured), proving the adapter
			// never asks for it.
			url1: wellfoundTwoPageData(1, 1, 1, "200", "Only Page Job"),
		},
	}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
}

// --- 4. sources.Job mapping and identity --------------------------------------------------

func TestWellfound_Fetch_MapsAWellFormedEntryToAJob(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, pages: map[string]string{url1: wellfoundOnePageSinglePage()}}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var backend *Job
	for i := range jobs {
		if jobs[i].ExternalID == "100" {
			backend = &jobs[i]
		}
	}
	if backend == nil {
		t.Fatalf("job 100 not found among %d mapped jobs", len(jobs))
	}
	if backend.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", backend.Title)
	}
	if backend.Company != "Acme Inc" {
		t.Errorf("Company = %q, want Acme Inc", backend.Company)
	}
	if backend.URL != "https://wellfound.com/jobs/100-senior-backend-engineer" {
		t.Errorf("URL = %q", backend.URL)
	}
	if !backend.Remote {
		t.Error("Remote = false, want true")
	}
	if backend.Location != "United States" {
		t.Errorf("Location = %q, want United States", backend.Location)
	}
	if !strings.Contains(backend.Description, "Build things.") {
		t.Errorf("Description = %q, want it to contain the body text", backend.Description)
	}
	if !strings.Contains(backend.Description, "150k") {
		t.Errorf("Description = %q, want the free-text compensation folded in (SEEK/Workstream precedent)", backend.Description)
	}
}

// wellfoundOnePageSinglePage is wellfoundOnePage with pageCount forced to 1, so a Fetch test
// exercises exactly one page's worth of mapping without also pulling in pagination.
func wellfoundOnePageSinglePage() string {
	return strings.Replace(wellfoundOnePage, `"pageCount": 2,`, `"pageCount": 1,`, 1)
}

func TestWellfound_Fetch_DropsAJobWithNoResolvableCompany(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, pages: map[string]string{url1: wellfoundOnePageSinglePage()}}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "103" {
			t.Errorf("job 103 (orphan, no resolvable company) was mapped as %+v, want it dropped", j)
		}
	}
	// The other two well-formed, company-resolved postings must be unaffected by the drop.
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2 (100 and 101; 103 dropped, 104 malformed)", len(jobs))
	}
}

func TestWellfound_Fetch_DropsAnEntryWithNoID(t *testing.T) {
	data := `{
  "ROOT_QUERY": {
    "__typename": "Query",
    "talent": {
      "__typename": "Talent",
      "seoLandingPageJobSearchResults({\"page\":1,\"remote\":true,\"role\":\"software-engineer\"})": {
        "__typename": "Results",
        "pageCount": 1,
        "totalJobCount": 1,
        "totalStartupCount": 1,
        "perPage": 20,
        "startups": [{"__ref": "StartupResult:1"}]
      }
    }
  },
  "StartupResult:1": {
    "__typename": "StartupResult",
    "id": "1",
    "name": "Acme Inc",
    "slug": "acme-inc",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:noid"}]
  },
  "JobListingSearchResult:noid": {
    "__typename": "JobListingSearchResult",
    "id": "",
    "slug": "no-id-job",
    "title": "No Id Job",
    "description": "<p>Body.</p>",
    "compensation": "",
    "remote": true,
    "locationNames": []
  }
}`
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, pages: map[string]string{url1: data}}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0 (the only entry has no extractable id)", len(jobs))
	}
}

// --- 5. Adapter wiring -----------------------------------------------------------------------

func TestWellfoundProvider(t *testing.T) {
	if got := NewWellfound(nil).Provider(); got != "wellfound" {
		t.Errorf("Provider() = %q, want wellfound", got)
	}
}

func TestWellfoundIsAggregatorOnly(t *testing.T) {
	s := NewWellfound(nil)
	if _, ok := s.(aggregator); !ok {
		t.Error("wellfound should implement the aggregator marker (each posting names its own hiring startup)")
	}
	if _, ok := s.(boardless); ok {
		t.Error("wellfound should NOT be boardless — the board is a role-taxonomy slug")
	}
	if _, ok := s.(HydratingSource); ok {
		t.Error("wellfound should NOT be a HydratingSource — the listing already carries the full body")
	}
}

func TestWellfound_Fetch_APageThatFailsToFetchIsALoudFailure(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, errs: map[string]error{url1: errors.New("boom")}}
	if _, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"}); err == nil {
		t.Fatal("Fetch: got nil error when the first page fails to fetch, want an error")
	}
}

// --- 6. Registry and hosted-tier integration --------------------------------------------------

func TestWellfoundRegisteredAndFilterable(t *testing.T) {
	src, ok := All(nil)["wellfound"]
	if !ok {
		t.Fatal("wellfound not registered in sources.All")
	}
	if _, isAggregator := src.(aggregator); !isAggregator {
		t.Error("wellfound should be an aggregator")
	}
	if _, isBoardless := src.(boardless); isBoardless {
		t.Error("wellfound should NOT be boardless — the board is a role-taxonomy slug")
	}
	found := false
	for _, p := range FilterableProviders() {
		if p == "wellfound" {
			found = true
		}
	}
	if !found {
		t.Error("wellfound should appear in the source facet")
	}
}

func TestWellfoundInFirecrawlProviders(t *testing.T) {
	if _, ok := firecrawlProviders["wellfound"]; !ok {
		t.Error("wellfound must be in firecrawlProviders — its pages are behind a Cloudflare " +
			"challenge no other transport this repository has can pass")
	}
}

func TestWellfound_Fetch_ALaterUnparseablePageIsALoudFailure(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	url2 := wellfoundPageURL("software-engineer", 2)
	fake := &wellfoundFakeHTTP{
		t: t,
		pages: map[string]string{
			url1: wellfoundTwoPageData(1, 2, 2, "200", "First Page Job"),
		},
	}
	// Page 2 is requested (pageCount says 2) but the fake has no page configured for it, which
	// GetHTML reports as an error — the same shape a real fetch failure or a markup change
	// that leaves __NEXT_DATA__ unparseable would produce.
	_ = url2
	if _, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"}); err == nil {
		t.Fatal("Fetch: got nil error when a later page cannot be fetched/parsed, want an error")
	}
}
