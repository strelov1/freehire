package sources

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/hellowork
var helloworkFixtures embed.FS

// helloworkFake serves trimmed copies of the live pages (2026-11-22). A URL with no fixture
// answers 404, which for a posting is the site's "gone".
type helloworkFake struct {
	pageErr error

	mu    sync.Mutex
	pages []string
}

var helloworkFixtureFor = map[string]string{
	helloworkHome: "home.html",
	helloworkBase + "/fr-fr/emploi/domaine_informatique.html":                  "domaine_informatique.html",
	helloworkBase + "/fr-fr/emploi/domaine_restauration.html":                  "food_service.html",
	helloworkBase + "/fr-fr/emploi/metier_devops.html":                         "metier_devops.html",
	helloworkBase + "/fr-fr/emploi/domaine_informatique-ville_lyon-69000.html": "domaine_informatique-ville_lyon-69000.html",
	fmt.Sprintf(helloworkPostingURL, 85769467):                                 "posting_85769467.html",
	fmt.Sprintf(helloworkPostingURL, 85769931):                                 "posting_85769931.html",
	fmt.Sprintf(helloworkPostingURL, 85769766):                                 "posting_85769766.html",
}

func (f *helloworkFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	name, ok := helloworkFixtureFor[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := helloworkFixtures.ReadFile("testdata/hellowork/" + name)
	if err != nil {
		panic(err)
	}
	return html.Parse(bytes.NewReader(raw))
}

func fastHelloWork(f *helloworkFake) hellowork {
	s := NewHelloWork(f).(hellowork)
	s.limiter = nopWaiter{}
	s.retryBackoff = time.Millisecond
	return s
}

func TestHelloWorkListsPostingsFromLandingPagesNewestFirstOnce(t *testing.T) {
	f := &helloworkFake{}
	ids, err := fastHelloWork(f).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{85769931, 85769864, 85769851, 85769766, 85769467}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	for _, u := range f.pages {
		if strings.Contains(u, "?") {
			t.Errorf("robots.txt disallows every query string, but %s was requested", u)
		}
		if strings.Contains(u, "recherche") || strings.Contains(u, "/alternance/") {
			t.Errorf("requested a page the crawl must not follow: %s", u)
		}
	}
}

func TestHelloWorkReadsAtMostTheListingBudget(t *testing.T) {
	f := &helloworkFake{}
	s := fastHelloWork(f)
	s.maxListPages = 2
	if _, err := s.list(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The home page plus two landing pages.
	if len(f.pages) != 3 {
		t.Errorf("read %d pages, want 3: %v", len(f.pages), f.pages)
	}
}

func TestHelloWorkRotationMovesBetweenRuns(t *testing.T) {
	frontier := []string{"a", "b", "c", "d", "e"}
	t0 := time.Unix(3600*1000, 0)
	a := helloworkRotate(frontier, t0, 2)
	b := helloworkRotate(frontier, t0.Add(time.Hour), 2)
	if a[0] == b[0] {
		t.Errorf("two runs an hour apart start at the same page %q", a[0])
	}
	if len(a) != len(frontier) {
		t.Errorf("rotation lost pages: %v", a)
	}
}

func TestHelloWorkFetchReadsNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &helloworkFake{}
	jobs, err := fastHelloWork(f).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "85769864" || id == "85769851" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// 2 refreshed + 3 read; no posting page was requested for a seen id.
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	for _, id := range []string{"85769864", "85769851"} {
		if j := byID[id]; !j.SeenRefresh || j.Description != "" {
			t.Errorf("seen posting %s must be a body-less refresh: %+v", id, j)
		}
	}
	for _, u := range f.pages {
		if strings.HasSuffix(u, "/85769864.html") || strings.HasSuffix(u, "/85769851.html") {
			t.Errorf("a stored posting's page was requested: %s", u)
		}
	}

	sap := byID["85769467"]
	if sap.Title != "Consultant SAP H/F" || sap.Company != "CGI" ||
		sap.URL != "https://www.hellowork.com/fr-fr/emplois/85769467.html" ||
		sap.Location != "Grenoble, Auvergne-Rhône-Alpes" || sap.EmploymentType != "full_time" ||
		!sap.IsTechHint || sap.SalaryMin != nil || len(sap.FallbackCountries) != 1 || sap.FallbackCountries[0] != "fr" {
		t.Errorf("SAP posting mapped wrong: %+v", sap)
	}
	// jobLocationType TELECOMMUTE marks remote work allowed on this site, not remote-only.
	if sap.Remote || sap.WorkMode != "" {
		t.Errorf("TELECOMMUTE must not read as remote: %+v", sap)
	}
	if sap.PostedAt == nil || !sap.PostedAt.Equal(time.Date(2026, 11, 22, 9, 36, 25, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", sap.PostedAt)
	}

	waiter := byID["85769931"]
	if waiter.EmploymentType != "contract" || waiter.IsTechHint {
		t.Errorf("temporary restaurant posting mapped wrong: %+v", waiter)
	}
	if waiter.SalaryMin == nil || *waiter.SalaryMin != 1865 || waiter.SalaryMax != nil || waiter.SalaryPeriod != "month" || waiter.SalaryCurrency != "EUR" {
		t.Errorf("monthly floor mapped wrong: %+v", waiter)
	}

	devops := byID["85769766"]
	if devops.SalaryMin == nil || *devops.SalaryMin != 55000 || devops.SalaryMax == nil || *devops.SalaryMax != 70000 || devops.SalaryPeriod != "year" {
		t.Errorf("yearly range mapped wrong: %+v", devops)
	}
}

func TestHelloWorkGonePostingIsDroppedNotFailed(t *testing.T) {
	// 85769864 and 85769851 have no page fixture: 404 is the site's "gone".
	jobs, err := fastHelloWork(&helloworkFake{}).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id != "85769864" })
	if err != nil {
		t.Fatalf("a gone posting must not fail the crawl: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "85769864" {
			t.Errorf("a gone posting was yielded: %+v", j)
		}
	}
}

func TestHelloWorkNewPostingsAreBoundedNewestFirst(t *testing.T) {
	f := &helloworkFake{}
	s := fastHelloWork(f)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	var postingReads []string
	for _, u := range f.pages {
		if strings.Contains(u, "/emplois/") {
			postingReads = append(postingReads, u)
		}
	}
	if len(postingReads) != 1 || !strings.HasSuffix(postingReads[0], "/85769931.html") || len(jobs) != 1 {
		t.Errorf("posting reads = %v, jobs = %d; want only the newest id", postingReads, len(jobs))
	}
}

func TestHelloWorkCrawlThatAnsweredNothingFails(t *testing.T) {
	s := fastHelloWork(&helloworkFake{pageErr: &StatusError{Method: "GET", Code: 403}})
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl whose every request was refused must fail")
	}
}

func TestHelloWorkIsBoardlessAggregatorWithAPostingLifetimeGrace(t *testing.T) {
	s := NewHelloWork(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("hellowork must be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("hellowork must be an aggregator")
	}
	if g := s.(sweepGrace).SweepGrace(); g != 30*24*time.Hour {
		t.Errorf("SweepGrace = %v, want the 30-day posting lifetime", g)
	}
}
