package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
)

type wantapplyAPIFake struct {
	pages map[string]string
	calls []string
}

func (f *wantapplyAPIFake) GetJSON(_ context.Context, raw string, dst any) error {
	f.calls = append(f.calls, raw)
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	body, ok := f.pages[u.Query().Get("page")]
	if !ok {
		return fmt.Errorf("unavailable page: %s", raw)
	}
	return json.Unmarshal([]byte(body), dst)
}

const wantapplyAPIJobFixture = `{"id":"new-uuid","url":"backend-at-acme","title":"Backend Engineer","companyName":"Acme","description":"<p>Build <b>Go</b>.</p><script>bad()</script>","remote":true,"workplaceTypes":["remote"],"employmentTypes":["fulltime"],"createdAt":"2026-07-15T18:31:30.246Z","publishedAt":"2026-11-15T18:31:30.246Z","expirationDate":"2026-12-15T18:31:30.246Z","status":"published","jobLocations":[{"iso2":"CY","name_en":"Cyprus"},{"iso2":"CY","name_en":"Cyprus"}]}`

func wantapplyAPIPage(postings string, next bool, total int) string {
	return fmt.Sprintf(`{"data":[%s],"hasNextPage":%t,"total":%d}`, postings, next, total)
}

func TestWantapplyAPIReadsCompleteBodiesAndPreservesIdentity(t *testing.T) {
	f := &wantapplyAPIFake{pages: map[string]string{"1": wantapplyAPIPage(wantapplyAPIJobFixture, true, 2), "2": wantapplyAPIPage(`{"url":"frontend-at-other","title":"Frontend Engineer","company":{"name":"Other"},"description":"<p>UI</p>","workplaceTypes":["onsite"],"employmentTypes":["parttime"],"status":"published"}`, false, 2)}}
	jobs, err := NewWantapply(f).Fetch(context.Background(), CompanyEntry{Company: "Wantapply", Provider: "wantapply"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || len(f.calls) != 2 {
		t.Fatalf("jobs=%d calls=%d", len(jobs), len(f.calls))
	}
	j := jobs[0]
	if j.ExternalID != "backend-at-acme" || j.URL != "https://wantapply.com/backend-at-acme" {
		t.Fatalf("published slug identity changed: %+v", j)
	}
	if j.Company != "Acme" || j.Title != "Backend Engineer" || j.Description != "<p>Build <b>Go</b>.</p>" {
		t.Fatalf("posting content: %+v", j)
	}
	if !j.Remote || j.WorkMode != "remote" || j.EmploymentType != "full_time" || j.Location != "Cyprus" || !slices.Equal(j.Countries, []string{"cy"}) {
		t.Fatalf("structured fields: %+v", j)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-15" || j.ApplicationDeadline == nil {
		t.Fatalf("posting dates: %+v", j)
	}
	if jobs[1].Company != "Other" || jobs[1].WorkMode != "onsite" || jobs[1].Remote || jobs[1].EmploymentType != "part_time" {
		t.Fatalf("second posting: %+v", jobs[1])
	}
	for _, raw := range f.calls {
		u, _ := url.Parse(raw)
		if u.Host != "wantapply.com" || u.Path != "/api/jobs" || u.Query().Get("limit") != "50" {
			t.Fatalf("wrong public listing: %s", raw)
		}
	}
}

func TestWantapplyAPIRegionalFilters(t *testing.T) {
	for _, cy := range []bool{false, true} {
		f := &wantapplyAPIFake{pages: map[string]string{"1": wantapplyAPIPage(wantapplyAPIJobFixture, false, 1)}}
		src := NewWantapply(f)
		host, provider := "wantapply.com", "wantapply"
		if cy {
			src = NewWantapplyCy(f)
			host, provider = "wantapply.cy", "wantapply-cy"
		}
		jobs, err := src.Fetch(context.Background(), CompanyEntry{})
		if err != nil {
			t.Fatal(err)
		}
		if src.Provider() != provider || len(jobs) != 1 || !strings.HasPrefix(jobs[0].URL, "https://"+host+"/") {
			t.Fatalf("regional identity: %s %+v", src.Provider(), jobs)
		}
		u, _ := url.Parse(f.calls[0])
		var filters map[string]any
		if err := json.Unmarshal([]byte(u.Query().Get("filters")), &filters); err != nil {
			t.Fatal(err)
		}
		if u.Host != host {
			t.Fatalf("regional host changed: %s", u.Host)
		}
		if cy {
			if filters["domain"] != nil || fmt.Sprint(filters["jobLocations"]) != "[CYP]" {
				t.Fatalf("Cyprus filters: %v", filters)
			}
		} else if filters["domain"] != "tech" {
			t.Fatalf("global filters: %v", filters)
		}
	}
}

func TestWantapplyAPIRejectsPartialOrUnprovenListings(t *testing.T) {
	cases := map[string]map[string]string{
		"later failure":        {"1": wantapplyAPIPage(wantapplyAPIJobFixture, true, 2)},
		"missing collection":   {"1": `{"hasNextPage":false,"total":0}`},
		"missing end evidence": {"1": `{"data":[],"total":0}`},
		"short count":          {"1": wantapplyAPIPage(wantapplyAPIJobFixture, false, 2)},
		"repeated page":        {"1": wantapplyAPIPage(wantapplyAPIJobFixture, true, 2), "2": wantapplyAPIPage(wantapplyAPIJobFixture, false, 2)},
	}
	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			f := &wantapplyAPIFake{pages: pages}
			jobs, err := NewWantapply(f).Fetch(context.Background(), CompanyEntry{})
			if err == nil || len(jobs) != 0 {
				t.Fatalf("partial listing accepted: jobs=%d err=%v", len(jobs), err)
			}
		})
	}
}

func TestWantapplyAPIEmptyIsAnExplicitCollection(t *testing.T) {
	f := &wantapplyAPIFake{pages: map[string]string{"1": wantapplyAPIPage("", false, 0)}}
	jobs, err := NewWantapply(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty listing: jobs=%d err=%v", len(jobs), err)
	}
}

func TestWantapplyAPIRegistryAndCompleteCatalogueCapabilities(t *testing.T) {
	reg := Taxonomy()
	for _, provider := range []string{"wantapply", "wantapply-cy"} {
		src, ok := reg[provider]
		if !ok {
			t.Fatalf("missing %s", provider)
		}
		if !IsAggregator(reg, provider) {
			t.Fatalf("%s must retain original multi-employer postings", provider)
		}
		if _, ok := src.(boardless); !ok {
			t.Fatalf("%s must be boardless", provider)
		}
		if _, ok := src.(fullCatalog); !ok {
			t.Fatalf("%s must close through its verified full listing", provider)
		}
		if _, ok := src.(HydratingSource); ok {
			t.Fatalf("%s must refresh inline bodies on every complete pass", provider)
		}
	}
	if _, ok := proxiedProviders["wantapply-cy"]; !ok {
		t.Fatal("Cyprus must retain proxy egress")
	}
}

func TestWantapplyAPIContractFormats(t *testing.T) {
	for _, format := range []string{"contract", "freelance"} {
		var p wantapplyPosting
		if err := json.Unmarshal([]byte(wantapplyAPIJobFixture), &p); err != nil {
			t.Fatal(err)
		}
		p.EmploymentTypes = []string{format}
		j, ok := p.toJob("wantapply.com")
		if !ok || j.EmploymentType != "contract" {
			t.Fatalf("%s mapped to %q", format, j.EmploymentType)
		}
	}
}
