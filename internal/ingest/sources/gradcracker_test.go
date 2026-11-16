package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// Both fixtures are trimmed from live responses captured 2026-10-18 through the
// Chrome-fingerprint transport, each kept verbatim:
//   - listing.html: the result count, the honeypot card, the first card of each opportunity
//     type plus one more, and the pagination nav of
//     /search/computing-technology/jobs?order=dateAdded (page 1 of 19);
//   - detail.html: the title-to-overview span of PA Consulting's posting 86180, embedded video
//     panel and feedback-pledge badge included. The tests serve it as the page of whichever
//     posting they hydrate.
func gradcrackerFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/gradcracker/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// gradcrackerListing is the listing fixture as a one-page listing: its count lowered to the
// three cards it keeps. The untouched count is what TestGradcrackerRefusesATruncatedWalk uses.
func gradcrackerListing(t *testing.T) string {
	return strings.Replace(gradcrackerFixture(t, "listing.html"), "Show 1455 results", "Show 3 results", 1)
}

// fakeGradcracker answers exact URLs (404 for any other) and records every request in order.
type fakeGradcracker struct {
	mu     sync.Mutex
	bodies map[string]string
	errs   map[string]error
	calls  []string
}

func (f *fakeGradcracker) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.calls = append(f.calls, url)
	f.mu.Unlock()
	if err := f.errs[url]; err != nil {
		return nil, err
	}
	b, ok := f.bodies[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	return html.Parse(strings.NewReader(b))
}

const (
	gcCT1      = "https://www.gradcracker.com/search/computing-technology/jobs?order=dateAdded"
	gcCT2      = "https://www.gradcracker.com/search/computing-technology/jobs?order=dateAdded&page=2"
	gcEE1      = "https://www.gradcracker.com/search/electronic-electrical/jobs?order=dateAdded"
	gcEE2      = "https://www.gradcracker.com/search/electronic-electrical/jobs?order=dateAdded&page=2"
	gcEYTech   = "https://www.gradcracker.com/hub/615/ey/work-placement-internship/86373/technology-summer-internship-july-2027-intake"
	gcNPL      = "https://www.gradcracker.com/hub/616/national-physical-laboratory-and-uk-telecoms-lab/graduate-job/86360/graduate-engineer-it-platforms-and-cyber"
	gcEYFin    = "https://www.gradcracker.com/hub/615/ey/work-placement-internship/86371/financial-restructuring-summer-internship-july-2027-intake"
	gcEmptyDoc = "<html><body><main></main></body></html>"
)

func mustParse(t *testing.T, doc string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseGradcrackerListing(t *testing.T) {
	root := mustParse(t, gradcrackerFixture(t, "listing.html"))
	cards := parseGradcrackerListing(root)
	var ids []string
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	// The honeypot card (wire:key="hp", a "featured graduate employer" with posting id 1) is
	// not a posting.
	if want := []string{"86373", "86360", "86371"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	c := cards[0]
	if c.URL != gcEYTech || c.Type != "work-placement-internship" {
		t.Errorf("url/type = %q/%q", c.URL, c.Type)
	}
	if c.Title != "Technology Summer Internship – July 2027 intake" || c.Company != "EY" {
		t.Errorf("title/company = %q/%q", c.Title, c.Company)
	}
	if c.Disciplines != "IT, Software, Computer Science, Cyber, FinTech, Analytics, Data" {
		t.Errorf("disciplines = %q", c.Disciplines)
	}
	wantFacts := []gradcrackerFact{
		{"Deadline", "Ongoing"},
		{"Salary", "Competitive"},
		{"Location", "London"},
		{"Degree required", "Bachelor's"}, // entity-encoded in the page
		{"Duration", "8 weeks"},
		{"Starting", "July 2027"},
	}
	if !slices.Equal(c.Facts, wantFacts) {
		t.Errorf("facts = %v, want %v", c.Facts, wantFacts)
	}
	if got := cards[1].Company; got != "National Physical Laboratory and UK Telecoms Lab" {
		t.Errorf("company = %q", got)
	}
	if got := gradcrackerListingTotal(root); got != 1455 {
		t.Errorf("total = %d, want 1455", got)
	}
}

func TestGradcrackerBodyDropsSiteFurniture(t *testing.T) {
	body, ok := gradcrackerBody(mustParse(t, gradcrackerFixture(t, "detail.html")))
	if !ok {
		t.Fatal("no body")
	}
	for _, want := range []string{"Company Description", "The Offer", "Salary potential"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	for _, furniture := range []string{"Gradcracker Insight", "video-js", "feedback pledge", "1e127ede32d8f816eacfb0aed73cee11"} {
		if strings.Contains(body, furniture) {
			t.Errorf("body keeps site furniture %q", furniture)
		}
	}
	if _, ok := gradcrackerBody(mustParse(t, gcEmptyDoc)); ok {
		t.Error("a page without .body-content yielded a body")
	}
}

func TestGradcrackerFetchNewGated(t *testing.T) {
	f := &fakeGradcracker{bodies: map[string]string{
		gcCT1:    gradcrackerListing(t),
		gcEE1:    gradcrackerListing(t), // the same postings under a second discipline are kept once
		gcEYTech: gradcrackerFixture(t, "detail.html"),
		gcEYFin:  gradcrackerFixture(t, "detail.html"),
	}}
	// 86360 is stored; 86371 and 86373 are new.
	seen := func(id string) bool { return id == "86360" }
	gate := DetailGate{}
	jobs, err := NewGradcracker(f, 0).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{}, seen, gate)
	if err != nil {
		t.Fatal(err)
	}
	// Each discipline to the page past its last (a 404), then the new postings' pages.
	if want := []string{gcCT1, gcCT2, gcEE1, gcEE2, gcEYTech, gcEYFin}; !slices.Equal(f.calls, want) {
		t.Fatalf("calls = %v\nwant    %v", f.calls, want)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 3 || len(jobs) != 3 {
		t.Fatalf("jobs = %+v, want 3 distinct", jobs)
	}
	if j := byID["86360"]; !j.SeenRefresh || j.Description != "" || j.URL != gcNPL || j.Title != "Graduate Engineer – IT Platforms and Cyber" {
		t.Errorf("stored posting = %+v, want a titled, body-less SeenRefresh", j)
	}
	if j := byID["86371"]; j.SeenRefresh || j.Description == "" || j.URL != gcEYFin {
		t.Errorf("new posting = %+v, want it hydrated", j)
	}
	j := byID["86373"]
	if j.SeenRefresh || j.EmploymentType != "internship" || j.Location != "London" || j.Company != "EY" {
		t.Errorf("new posting = %+v", j)
	}
	for _, want := range []string{"Starting:", "July 2027", "Disciplines:", "Company Description"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description lacks %q", want)
		}
	}
	if strings.Contains(j.Description, "video-js") {
		t.Error("description keeps the embedded video panel")
	}
}

func TestGradcrackerGraduateJobStatesNoEmploymentType(t *testing.T) {
	cards := parseGradcrackerListing(mustParse(t, gradcrackerFixture(t, "listing.html")))
	if got := cards[1].job().EmploymentType; got != "" {
		t.Errorf("graduate job employment type = %q, want none", got)
	}
}

func TestGradcrackerBudgetDefersTheRest(t *testing.T) {
	f := &fakeGradcracker{bodies: map[string]string{
		gcCT1:    gradcrackerListing(t),
		gcEE1:    gradcrackerListing(t),
		gcEYTech: gradcrackerFixture(t, "detail.html"),
	}}
	jobs, err := NewGradcracker(f, 1).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "86373" {
		t.Errorf("jobs = %+v, want only the newest posting", jobs)
	}
}

func TestGradcrackerRefusalStopsHydration(t *testing.T) {
	f := &fakeGradcracker{
		bodies: map[string]string{gcCT1: gradcrackerListing(t), gcEE1: gradcrackerListing(t)},
		errs:   map[string]error{gcEYTech: &StatusError{Method: "GET", Code: 403, URL: gcEYTech}},
	}
	_, err := NewGradcracker(f, 0).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	// The refusal blocks the IP: nothing further is asked, and a run that needed bodies and
	// read none is a failure, not a quiet source.
	if err == nil {
		t.Fatal("want an error for a run that read none of the bodies it needed")
	}
	if last := f.calls[len(f.calls)-1]; last != gcEYTech || len(f.calls) != 5 {
		t.Errorf("calls = %v, want the walk and then the one refused posting page", f.calls)
	}
}

func TestGradcrackerDefersAnUnreadablePostingPage(t *testing.T) {
	f := &fakeGradcracker{bodies: map[string]string{
		gcCT1:    gradcrackerListing(t),
		gcEE1:    gradcrackerListing(t),
		gcEYTech: gcEmptyDoc, // a page with no body is deferred, not stored empty
		gcNPL:    gradcrackerFixture(t, "detail.html"),
		gcEYFin:  gradcrackerFixture(t, "detail.html"),
	}}
	jobs, err := NewGradcracker(f, 0).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	if want := []string{"86360", "86371"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v (86373 deferred)", ids, want)
	}
}

// The crawl is a fullCatalog, so a walk that read fewer cards than the listing states — a
// page lost, the site capping a deep page — must fail the run rather than let the
// source-scoped sweep close everything it missed.
func TestGradcrackerRefusesATruncatedWalk(t *testing.T) {
	f := &fakeGradcracker{bodies: map[string]string{gcCT1: gradcrackerFixture(t, "listing.html")}} // "Show 1455 results"
	if _, err := NewGradcracker(f, 0).Fetch(context.Background(), CompanyEntry{}); err == nil || !strings.Contains(err.Error(), "1455") {
		t.Fatalf("err = %v, want a truncated-walk refusal", err)
	}
}

// Without the listing's own count there is no proof the walk was whole: a page that came back
// empty mid-walk would end it looking like the last page.
func TestGradcrackerRefusesAListingWithoutItsCount(t *testing.T) {
	uncounted := strings.Replace(gradcrackerFixture(t, "listing.html"), "Show 1455 results", "", 1)
	f := &fakeGradcracker{bodies: map[string]string{gcCT1: uncounted, gcCT2: gcEmptyDoc, gcEE1: uncounted, gcEE2: gcEmptyDoc}}
	all := func(string) bool { return true } // every posting stored: only the walk can fail
	if _, err := NewGradcracker(f, 0).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, all); err == nil || !strings.Contains(err.Error(), "result count") {
		t.Fatalf("err = %v, want the run to fail when page 1 states no result count", err)
	}
}

// A posting page that answers 404/410 is the posting gone — an answer. Failing the run on it
// would throw away every stored posting's refresh, skip the sweep and, run after run, age the
// whole provider into the chronic-board close.
func TestGradcrackerGonePostingPagesDoNotFailTheRun(t *testing.T) {
	// The new postings' pages are unrouted, so the fake answers 404.
	f := &fakeGradcracker{bodies: map[string]string{gcCT1: gradcrackerListing(t), gcEE1: gradcrackerListing(t)}}
	seen := func(id string) bool { return id == "86360" }
	jobs, err := NewGradcracker(f, 0).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh || jobs[0].ExternalID != "86360" {
		t.Errorf("jobs = %+v, want only the stored posting's refresh", jobs)
	}
}

// A 200 page with no body is NOT an answer: it is the parser failing. When every page that
// came back 200 yielded no body, the posting page's layout has changed, and the run must say
// so rather than go green while ingesting nothing new. One stuck page among good ones is
// TestGradcrackerDefersAnUnreadablePostingPage, and passes.
func TestGradcrackerFailsWhenNoFetchedPageYieldsABody(t *testing.T) {
	f := &fakeGradcracker{bodies: map[string]string{
		gcCT1:    gradcrackerListing(t),
		gcEE1:    gradcrackerListing(t),
		gcEYTech: gcEmptyDoc,
		gcEYFin:  gcEmptyDoc,
		// gcNPL unrouted: a 404 alongside does not rescue the run.
	}}
	_, err := NewGradcracker(f, 0).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("err = %v, want a failure naming the body-less pages", err)
	}
}

func TestGradcrackerAnyListingFailureFailsTheRun(t *testing.T) {
	refused := func(u string) error { return &StatusError{Method: "GET", Code: 403, URL: u} }
	for name, f := range map[string]*fakeGradcracker{
		"first page refused":     {errs: map[string]error{gcCT1: refused(gcCT1)}},
		"first page empty":       {bodies: map[string]string{gcCT1: gcEmptyDoc}},
		"later page refused":     {bodies: map[string]string{gcCT1: gradcrackerListing(t)}, errs: map[string]error{gcCT2: refused(gcCT2)}},
		"second discipline gone": {bodies: map[string]string{gcCT1: gradcrackerListing(t)}},
	} {
		if _, err := NewGradcracker(f, 0).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Errorf("%s: want the run to fail", name)
		}
	}
}

func TestGradcrackerRegistered(t *testing.T) {
	src, ok := Taxonomy()["gradcracker"]
	if !ok {
		t.Fatal("gradcracker missing from the taxonomy registry")
	}
	if _, ok := src.(boardless); !ok {
		t.Error("gradcracker must be boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("gradcracker must be an aggregator")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("gradcracker must be a fullCatalog")
	}
	if _, ok := src.(CoverageGated); !ok {
		t.Error("gradcracker must be CoverageGated")
	}
}
