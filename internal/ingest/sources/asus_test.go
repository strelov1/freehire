package sources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// asusSite routes the real pages (2026-10-25): two listing pages of five postings, the empty
// page past the end, and each posting's page trimmed to its <main>.
func asusSite(t *testing.T) *routedHTTP {
	t.Helper()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("testdata/asus", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	r := (&routedHTTP{}).
		route(asusListURL+"1", read("jobs_page1.html")).
		route(asusListURL+"2", read("jobs_page2.html")).
		route(asusListURL+"3", read("jobs_past_end.html"))
	details, err := filepath.Glob("testdata/asus/detail_*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range details {
		sn := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(d), "detail_"), ".html")
		r.route(asusJobURL+sn, read(filepath.Base(d)))
	}
	return r
}

const (
	asusPM       = "14f6ceef-d733-468a-9756-6ac545c1f7cf"
	asusSoftware = "e179d96e-084d-4c03-91ac-111ce59c4ff6"
	asusHardware = "15f2d6a8-2132-4fc2-9b04-c3cdcd1bf417"
	asusCloud    = "0df4e6a3-ad7b-4e04-84a0-31f0a2088cd3"
)

func TestASUSFetchWalksToTheEmptyPage(t *testing.T) {
	r := asusSite(t)
	jobs, err := NewASUS(r).Fetch(context.Background(), CompanyEntry{Company: "ASUS"})
	if err != nil {
		t.Fatal(err)
	}
	// Three listing pages (the third empty) and ten posting pages.
	if len(jobs) != 10 || r.calls != 13 {
		t.Fatalf("got %d jobs over %d requests", len(jobs), r.calls)
	}
	byID := byExternalID(jobs)

	sw := byID[asusSoftware]
	if sw.Title != "2026校園徵才-Windows/MacOS 軟體研發工程師(顯示器)" || sw.Company != "ASUS" ||
		sw.URL != "https://recruit.asus.com/Jobs/Detail?sn="+asusSoftware || sw.Category != "software_engineering" {
		t.Errorf("identity: %q %q %s %q", sw.Title, sw.Company, sw.URL, sw.Category)
	}
	if sw.Location != "Taipei, Taiwan" || len(sw.Countries) != 1 || sw.Countries[0] != "tw" || sw.EmploymentType != "full_time" {
		t.Errorf("facets: %q %v %q", sw.Location, sw.Countries, sw.EmploymentType)
	}
	// The posting page's duties and requirements, headings included.
	if !strings.Contains(sw.Description, "工作說明") || !strings.Contains(sw.Description, "需求條件") {
		t.Errorf("body: %.300s", sw.Description)
	}

	if byID[asusHardware].Category != "hardware" {
		t.Errorf("hardware: %q", byID[asusHardware].Category)
	}
	if cloud := byID[asusCloud]; cloud.Category != "" || !cloud.IsTechHint {
		t.Errorf("e-commerce R&D: %q %v", cloud.Category, cloud.IsTechHint)
	}
	// Market-development PM is left to the dictionaries.
	if pm := byID[asusPM]; pm.Category != "" || pm.IsTechHint {
		t.Errorf("PM: %q %v", pm.Category, pm.IsTechHint)
	}
}

func TestASUSSeenPostingsCostNoDetail(t *testing.T) {
	r := asusSite(t)
	jobs, err := NewASUS(r).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "ASUS"},
		func(id string) bool { return id != asusSoftware })
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 4 || len(jobs) != 10 {
		t.Fatalf("requests %d, jobs %d", r.calls, len(jobs))
	}
	if pm := byExternalID(jobs)[asusPM]; !pm.SeenRefresh || pm.Title != "2026校園徵才-產品經理(PC)" {
		t.Errorf("seen posting: %+v", pm)
	}
}

func TestASUSDefersAFailedDetail(t *testing.T) {
	r := asusSite(t).routeErr("sn="+asusHardware, errors.New("HTTP 500"))
	jobs, err := NewASUS(r).Fetch(context.Background(), CompanyEntry{Company: "ASUS"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byExternalID(jobs)[asusHardware]; ok || len(jobs) != 9 {
		t.Fatalf("a failed detail must be deferred, got %d jobs", len(jobs))
	}
	r = asusSite(t).routeErr("sn="+asusHardware, errors.New("HTTP 500"))
	if _, err := NewASUS(r).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "ASUS"},
		func(id string) bool { return id != asusHardware }); err == nil {
		t.Error("the only new posting failed to read and the crawl still succeeded")
	}
}

func TestASUSFailsOnAnyListingPage(t *testing.T) {
	for _, page := range []string{"1", "2", "3"} {
		r := asusSite(t).routeErr(asusListURL+page, errors.New("HTTP 503"))
		if jobs, err := NewASUS(r).Fetch(context.Background(), CompanyEntry{Company: "ASUS"}); err == nil {
			t.Errorf("page %s failing: got %d jobs and no error", page, len(jobs))
		}
	}
	empty := (&routedHTTP{}).route(asusListURL+"1", `<html><body><table></table></body></html>`)
	if _, err := NewASUS(empty).Fetch(context.Background(), CompanyEntry{Company: "ASUS"}); err == nil {
		t.Error("an empty first page must fail")
	}
}

func TestASUSTitleDropsTheRequisitionCode(t *testing.T) {
	if got := asusTitle("  RD11482 Brand Design Strategy Planning &amp; Management  "); got != "Brand Design Strategy Planning &amp; Management" {
		t.Errorf("got %q", got)
	}
	if got := asusTitle("AI Engineer"); got != "AI Engineer" {
		t.Errorf("a title with no code changed: %q", got)
	}
}

func TestASUSIsBoardlessFullBoardListing(t *testing.T) {
	s := NewASUS(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("asus must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("asus must be fullBoardListing")
	}
}
