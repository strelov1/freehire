package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestPowerToFlyProvider(t *testing.T) {
	if got := NewPowerToFly(nil).Provider(); got != "powertofly" {
		t.Errorf("Provider() = %q, want powertofly", got)
	}
}

func TestPowerToFlyIsBoardlessAggregator(t *testing.T) {
	s := NewPowerToFly(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("powertofly should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("powertofly should implement the aggregator marker")
	}
}

func TestPowerToFlyRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["powertofly"]; !ok {
		t.Error("All() should register provider powertofly")
	}
	if !slices.Contains(FilterableProviders(), "powertofly") {
		t.Error("FilterableProviders() should include powertofly")
	}
}

func TestPowerToFlyFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"id":1691729,"title":"Senior Go Engineer","description":"<p>Build &amp; ship.</p>","location":"Remote","location_regions":["USA"],"employment_type":"Full Time","company":{"name":"NASCAR"},"country":{"title":"United States"},"state":{"title":"Alabama"},"city":{"title":"Talladega"}},
{"id":0,"title":"skip me","company":{"name":"NoID"}}
],"meta":{"next_page":2}}`
	page2 := `{"data":[
{"id":42,"title":"Ops Lead","description":"x","location":"Onsite","location_regions":["USA"],"employment_type":"Per Project","company":{"name":"Acme"},"country":{"title":"United States"}}
],"meta":{"next_page":null}}`
	// page=2 routed first so the more specific match wins over the base ?page=1 route.
	fake := (&routedHTTP{}).route("page=2", page2).route("page=1", page1)

	jobs, err := NewPowerToFly(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the id=0 posting dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "1691729" || j.Company != "NASCAR" || j.Title != "Senior Go Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://powertofly.com/jobs/detail/1691729" {
		t.Errorf("URL = %q, want the powertofly detail URL", j.URL)
	}
	// "location":"Remote" is the work arrangement, not geography — geography comes from city/state/country.
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.Location != "Talladega, Alabama, United States" {
		t.Errorf("Location = %q, want the built city/state/country", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}

	// page 2: onsite arrangement, "Per Project" → contract, geography falls back through the chain.
	j2 := jobs[1]
	if j2.WorkMode != "onsite" || j2.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want onsite/false", j2.WorkMode, j2.Remote)
	}
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j2.EmploymentType)
	}
}

// powertoflyEndlessFeed answers every page with one posting and a next_page: a feed that
// never ends, or one longer than the cap.
type powertoflyEndlessFeed struct{ calls int }

func (f *powertoflyEndlessFeed) GetJSON(_ context.Context, _ string, v any) error {
	f.calls++
	body := fmt.Sprintf(`{"data":[{"id":%d,"title":"t","company":{"name":"Acme"}}],"meta":{"next_page":%d}}`, f.calls, f.calls+1)
	return json.Unmarshal([]byte(body), v)
}

// powertofly is swept by source, so the marker must stay — and it is sound only because every
// way a walk can end short fails the crawl (the tests below).
func TestPowerToFlyIsFullCatalog(t *testing.T) {
	if _, ok := NewPowerToFly(nil).(fullCatalog); !ok {
		t.Error("powertofly should implement the fullCatalog marker")
	}
	if !slices.Contains(FullCatalogProviders(All(nil)), "powertofly") {
		t.Error("FullCatalogProviders() should include powertofly")
	}
}

// Reaching the page cap before the feed's own end is a truncated walk: returned as a success
// it would let the source-scoped sweep close every posting past the cap.
func TestPowerToFlyFetchFailsAtThePageCap(t *testing.T) {
	feed := &powertoflyEndlessFeed{}
	jobs, err := NewPowerToFly(feed).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatalf("got %d jobs and no error; a walk cut short by the cap must fail", len(jobs))
	}
	if feed.calls != powertoflyMaxPages {
		t.Errorf("fetched %d pages, want exactly the cap (%d)", feed.calls, powertoflyMaxPages)
	}
}

func TestPowerToFlyFetchFailsOnALaterPage(t *testing.T) {
	page1 := `{"data":[{"id":1,"title":"t","company":{"name":"Acme"}}],"meta":{"next_page":2}}`
	fake := (&routedHTTP{}).routeErr("page=2", errors.New("timeout")).route("page=1", page1)
	if jobs, err := NewPowerToFly(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatalf("got %d jobs and no error; a failed later page must fail the crawl", len(jobs))
	}
}

func TestPowerToFlyFetchFailsOnAnEmptyPageThatIsNotTheEnd(t *testing.T) {
	page1 := `{"data":[{"id":1,"title":"t","company":{"name":"Acme"}}],"meta":{"next_page":2}}`
	page2 := `{"data":[],"meta":{"next_page":3}}`
	fake := (&routedHTTP{}).route("page=2", page2).route("page=1", page1)
	if jobs, err := NewPowerToFly(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatalf("got %d jobs and no error; an empty page naming a next page is not the end", len(jobs))
	}
}
