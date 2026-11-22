package sources

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/hellowork
var helloworkFixtures embed.FS

// helloworkFake serves trimmed copies of the live pages (2026-11-22). A URL with no fixture
// answers 404, which for a posting is the site's "gone".
type helloworkFake struct {
	pageErr   error
	errByURL  map[string]error
	htmlByURL map[string]string
	finalURL  map[string]string

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
	if err := f.errByURL[u]; err != nil {
		return nil, err
	}
	if body, ok := f.htmlByURL[u]; ok {
		return html.Parse(strings.NewReader(body))
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

func (f *helloworkFake) GetHTMLResolved(ctx context.Context, u string) (*html.Node, string, error) {
	root, err := f.GetHTML(ctx, u)
	return root, firstNonEmpty(f.finalURL[u], u), err
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

func TestHelloWorkFrontierRotatesByRemainingBudget(t *testing.T) {
	for hour, want := range []string{
		helloworkBase + "/fr-fr/emploi/domaine_informatique-ville_lyon-69000.html",
		helloworkBase + "/fr-fr/emploi/domaine_restauration.html",
	} {
		f := &helloworkFake{htmlByURL: map[string]string{
			helloworkHome: `<a href="/fr-fr/emploi/domaine_informatique.html">Informatique</a>`,
		}}
		s := fastHelloWork(f)
		s.maxListPages = 2
		s.now = func() time.Time { return time.Unix(int64(hour)*3600, 0) }
		if _, err := s.list(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(f.pages) != 3 || f.pages[2] != want {
			t.Errorf("hour %d requested %v, want %s after the home page and field", hour, f.pages, want)
		}
	}
}

func TestHelloWorkFollowsOnlyCleanSameOriginLinks(t *testing.T) {
	field := helloworkBase + "/fr-fr/emploi/domaine_informatique.html"
	f := &helloworkFake{finalURL: map[string]string{helloworkHome: helloworkBase + "/fr-fr/accueil/index.html"}, htmlByURL: map[string]string{
		helloworkHome: `<a href="../emploi/domaine_informatique.html">Informatique</a>`,
		field: `<a href="../emplois/85769467.html">SAP</a>
<a href="metier_devops.html">DevOps</a>
<a href="//www.hellowork.com/fr-fr/emplois/85769864.html">Dev</a>
<a href="https://www.hellowork.com/fr-fr/emplois/85769931.html">Server</a>
<a href="//elsewhere.example/fr-fr/emplois/99999999.html">Offsite</a>
<a href="http://www.hellowork.com/fr-fr/emplois/99999998.html">HTTP</a>
<a href="../emplois/99999997.html?">Empty query</a>
<a href="../emplois/99999996.html?p=2">Query</a>
<a href="../alternance/metier_assistant-manager.html">Alternance</a>`,
	}}
	got, err := fastHelloWork(f).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{85769931, 85769864, 85769766, 85769467}
	if !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
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
	if sap.ApplicationDeadline == nil || !sap.ApplicationDeadline.Equal(time.Date(2026, 12, 22, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ApplicationDeadline = %v, want 2026-12-22", sap.ApplicationDeadline)
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

func TestHelloWorkFailedDetailKeepsUnreadableIdentity(t *testing.T) {
	const uri = helloworkBase + "/fr-fr/emplois/85769864.html"
	for _, fixture := range []struct {
		name string
		fake *helloworkFake
	}{
		{"refused", &helloworkFake{errByURL: map[string]error{uri: &StatusError{Code: http.StatusForbidden}}}},
		{"invalid posting", &helloworkFake{htmlByURL: map[string]string{
			uri: `<script type="application/ld+json">{"@type":"JobPosting","title":"Engineer"}</script>`,
		}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			jobs, err := fastHelloWork(fixture.fake).FetchNew(context.Background(), CompanyEntry{Company: "HelloWork"},
				func(id string) bool { return id != "85769864" && id != "85769931" })
			if err != nil {
				t.Fatal(err)
			}
			for _, j := range jobs {
				if j.ExternalID == "85769864" {
					if !j.Unreadable || j.URL != uri || j.Company != "" {
						t.Errorf("unexpected unreadable marker: %+v", j)
					}
					return
				}
			}
			t.Fatal("failed posting did not yield an unreadable marker")
		})
	}
}

func TestHelloWorkInvalidPostingDoesNotCountAsAnAnswer(t *testing.T) {
	const uri = helloworkBase + "/fr-fr/emplois/85769864.html"
	f := &helloworkFake{htmlByURL: map[string]string{
		uri: `<script type="application/ld+json">{"@type":"JobPosting","title":"Engineer"}</script>`,
	}}
	_, err := fastHelloWork(f).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id != "85769864" })
	if err == nil {
		t.Fatal("a crawl whose only attempted posting was invalid must fail")
	}
}

func TestHelloWorkKeepsPostingsWithFormattedSalary(t *testing.T) {
	const uri = helloworkBase + "/fr-fr/emplois/85769864.html"
	f := &helloworkFake{htmlByURL: map[string]string{
		uri: `<script type="application/ld+json">
{"@type":"JobPosting","title":"Engineer","description":"Build web applications",
"hiringOrganization":{"name":"Example"},"baseSalary":{"currency":"EUR","value":"38 000,00"}}
</script>`,
	}}
	job, ok, err := fastHelloWork(f).detail(context.Background(), 85769864)
	if err != nil || !ok {
		t.Fatalf("formatted salary dropped posting: ok=%v, err=%v", ok, err)
	}
	if job.Title != "Engineer" || job.SalaryMin != nil || job.SalaryMax != nil {
		t.Errorf("unexpected job: %+v", job)
	}
}

func TestHelloWorkDeadlinePreservesTheStatedCalendarDay(t *testing.T) {
	const uri = helloworkBase + "/fr-fr/emplois/85769864.html"
	f := &helloworkFake{htmlByURL: map[string]string{
		uri: `<script type="application/ld+json">
{"@type":"JobPosting","title":"Engineer","description":"Build web applications",
"hiringOrganization":{"name":"Example"},"validThrough":"2099-12-22T00:30:00+01:00"}
</script>`,
	}}
	job, ok, err := fastHelloWork(f).detail(context.Background(), 85769864)
	if err != nil || !ok {
		t.Fatalf("detail: ok=%v, err=%v", ok, err)
	}
	want := time.Date(2099, 12, 22, 0, 0, 0, 0, time.UTC)
	if job.ApplicationDeadline == nil || !job.ApplicationDeadline.Equal(want) {
		t.Errorf("ApplicationDeadline = %v, want %v", job.ApplicationDeadline, want)
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

func TestHelloWorkDetailBudgetReachesOlderUnseenPostings(t *testing.T) {
	for hour, want := range []string{"85769766", "85769467"} {
		f := &helloworkFake{}
		s := fastHelloWork(f)
		s.maxNewPerRun = 4
		s.now = func() time.Time { return time.Unix(int64(hour)*3600, 0) }
		jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
		if err != nil {
			t.Fatal(err)
		}
		var detailReads int
		for _, u := range f.pages {
			if strings.Contains(u, "/emplois/") {
				detailReads++
			}
		}
		if detailReads != 4 || len(jobs) != 2 || jobs[0].ExternalID != "85769931" || jobs[1].ExternalID != want {
			t.Errorf("hour %d read %d details, yielded %v, want newest and %s in four requests", hour, detailReads, jobs, want)
		}
	}
}

func TestHelloWorkCrawlThatAnsweredNothingFails(t *testing.T) {
	s := fastHelloWork(&helloworkFake{pageErr: &StatusError{Method: "GET", Code: 403}})
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl whose every request was refused must fail")
	}
}

func TestHelloWorkRegistryPacesEachAttemptAndStopsRedirects(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/retry":
			if attempts.Add(1) <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		case "/redirect":
			http.Redirect(w, r, "/retry?p=2", http.StatusFound)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer srv.Close()

	shared := NewClient()
	shared.httpClient = srv.Client()
	shared.retryDelay = 0
	s := All(shared)["hellowork"].(hellowork)
	if got := s.http.(*Client); got == shared || got.backoff != shared.backoff {
		t.Error("HelloWork must use an isolated client with the shared host backoff")
	}
	gate := &recordingWaiter{}
	s.limiter = gate
	s.retryBackoff = time.Millisecond
	if _, _, err := s.page(context.Background(), srv.URL+"/retry"); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 3 || gate.calls != 3 {
		t.Errorf("attempts=%d admissions=%d, want 3 each", got, gate.calls)
	}
	if _, _, err := s.page(context.Background(), srv.URL+"/redirect"); err == nil || attempts.Load() != 3 {
		t.Errorf("redirect returned err=%v after %d retry-endpoint requests, want a rejected redirect", err, attempts.Load())
	}
	before := attempts.Load()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := shared.httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || attempts.Load() != before+1 {
		t.Error("shared client must still follow redirects")
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
