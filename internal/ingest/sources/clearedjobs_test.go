package sources

import (
	"context"
	"embed"
	"encoding/xml"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// clearedjobsFixtures holds the trimmed live responses the fake serves.
//
//go:embed testdata/clearedjobs
var clearedjobsFixtures embed.FS

// clearedjobsFake serves the real fixtures (fetched 2026-11-17, trimmed): the sitemap index, two
// job shards that are byte-for-byte copies of each other as the live second shard is, and a
// newest-postings page naming two postings the sitemap lacks. Posting 1541169 (Top Secret / SCI,
// Redstone Arsenal) and 1959852 (Public Trust, country only) have pages; 1965353 answers with
// 1959852's page; 1928706 and 1965357 answer 410, as a taken-down posting does.
type clearedjobsFake struct {
	// refuseFirst answers 403 to the first request of every URL; the retry must clear it.
	refuseFirst bool
	// sitemapErr fails the sitemap index.
	sitemapErr error
	// newestErr fails the newest-postings page.
	newestErr error
	// allChallenged answers every posting page with a challenge.
	allChallenged bool

	mu    sync.Mutex
	asked map[string]int
	pages []string
}

func (f *clearedjobsFake) refused(u string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[u]++
	return f.refuseFirst && f.asked[u] == 1
}

func (f *clearedjobsFake) GetXML(_ context.Context, u string, v any) error {
	if f.refused(u) {
		return &StatusError{Method: "GET", Code: 403, URL: u}
	}
	var path string
	switch {
	case u == clearedjobsSitemapURL:
		if f.sitemapErr != nil {
			return f.sitemapErr
		}
		path = "testdata/clearedjobs/sitemap_index.xml"
	case strings.HasSuffix(u, "/sitemap_active_jobs.xml"):
		path = "testdata/clearedjobs/sitemap_active_jobs.xml"
	case strings.HasSuffix(u, "/sitemap_active_jobs_2.xml"):
		path = "testdata/clearedjobs/sitemap_active_jobs_2.xml"
	default:
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := clearedjobsFixtures.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *clearedjobsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.refused(u) {
		return nil, &StatusError{Method: "GET", Code: 403, URL: u}
	}
	var path string
	switch {
	case u == clearedjobsNewestURL:
		if f.newestErr != nil {
			return nil, f.newestErr
		}
		path = "testdata/clearedjobs/newest.html"
	case f.allChallenged:
		return html.Parse(strings.NewReader("<html><body>Just a moment...</body></html>"))
	case strings.HasSuffix(u, "-1541169"):
		path = "testdata/clearedjobs/posting_1541169.html"
	case strings.HasSuffix(u, "-1959852"), strings.HasSuffix(u, "-1965353"):
		path = "testdata/clearedjobs/posting_1959852.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 410, URL: u}
	}
	raw, err := clearedjobsFixtures.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestClearedJobs(f *clearedjobsFake) clearedjobs {
	return clearedjobs{
		http:           f,
		limiter:        noWait{},
		freshPerRun:    clearedjobsFreshPerRun,
		backlogPerRun:  clearedjobsBacklogPerRun,
		deadlineMargin: clearedjobsDeadlineMargin,
		retryBackoff:   time.Millisecond,
		now:            time.Now,
	}
}

func jobsByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestClearedJobsHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &clearedjobsFake{}
	jobs, err := newTestClearedJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "1541169" })
	if err != nil {
		t.Fatal(err)
	}
	byID := jobsByID(jobs)
	// 1541169 seen, 1959852 and 1965353 hydrated; 1928706 and 1965357 are gone (410).
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want one refreshed and two hydrated: %+v", len(jobs), jobs)
	}
	if r := byID["1541169"]; !r.SeenRefresh || r.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	for _, id := range []string{"1928706", "1965357"} {
		if _, ok := byID[id]; ok {
			t.Errorf("posting %s, which the site answers 410, was yielded", id)
		}
	}
	if _, ok := byID["1965353"]; !ok {
		t.Error("a posting only the newest-postings page names was not read")
	}
	j := byID["1959852"]
	if j.Title != "Okta IAM / Identity Integration Engineer" || j.Company != "Peraton" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != clearedjobsBaseURL+"/job/okta-iam-identity-integration-engineer-1959852" {
		t.Errorf("url = %q", j.URL)
	}
	if j.Location != "United States" || !reflect.DeepEqual(j.Countries, []string{"us"}) {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02") != "2026-10-27" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if levels := location.ClearanceLevelsFromDescription(j.Description); !reflect.DeepEqual(levels, []string{"public_trust"}) {
		t.Errorf("clearance levels = %v, want [public_trust]", levels)
	}
}

// The second job shard is a copy of the first on the live site; a posting listed twice is read once.
func TestClearedJobsListsEachPostingOnceNewestFirst(t *testing.T) {
	postings, err := newTestClearedJobs(&clearedjobsFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range postings {
		ids = append(ids, p.id)
	}
	want := []string{"1965357", "1965353", "1959852", "1928706", "1541169"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("listed %v, want %v", ids, want)
	}
}

// The budget goes to the newest postings first.
func TestClearedJobsHydrationBudgetTakesTheNewest(t *testing.T) {
	f := &clearedjobsFake{}
	s := newTestClearedJobs(f)
	s.freshPerRun, s.backlogPerRun = 1, 0
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	var postings []string
	for _, u := range f.pages {
		if strings.Contains(u, "/job/") {
			postings = append(postings, u)
		}
	}
	if len(postings) != 1 || !strings.HasSuffix(postings[0], "-1965357") {
		t.Errorf("posting pages read = %v, want only the newest", postings)
	}
}

func TestClearedJobsClearanceLevelsFromTheLabelledField(t *testing.T) {
	for industry, want := range map[string][]string{
		"Top Secret / SCI":             {"ts_sci"},
		"Top Secret / SCI + Polygraph": {"ts_sci"},
		"Top Secret":                   {"top_secret"},
		"Secret":                       {"secret"},
		"Public Trust":                 {"public_trust"},
	} {
		var ld clearedjobsLD
		ld.Title, ld.HiringOrganization.Name = "Systems Engineer", "Acme"
		ld.Description, ld.Industry = "<p>Build and maintain systems.</p>", industry
		j, ok := clearedjobsJob(ld, clearedjobsPosting{id: "1"})
		if !ok {
			t.Fatalf("%s: posting rejected", industry)
		}
		if got := location.ClearanceLevelsFromDescription(j.Description); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: levels = %v, want %v", industry, got, want)
		}
		if !location.RequiresClearanceFromDescription(j.Description) {
			t.Errorf("%q: clearance not required", industry)
		}
	}
	var ld clearedjobsLD
	ld.Title, ld.HiringOrganization.Name, ld.Description = "Systems Engineer", "Acme", "<p>Build systems.</p>"
	j, _ := clearedjobsJob(ld, clearedjobsPosting{id: "1"})
	if strings.Contains(j.Description, "Security Clearance") {
		t.Errorf("a posting stating no level was given a clearance line: %q", j.Description)
	}
}

func TestClearedJobsRejectsAPostingWithoutTitleEmployerOrBody(t *testing.T) {
	full := clearedjobsLD{Title: "Engineer", Description: "<p>Body.</p>"}
	full.HiringOrganization.Name = "Acme"
	if _, ok := clearedjobsJob(full, clearedjobsPosting{id: "1"}); !ok {
		t.Fatal("a complete posting was rejected")
	}
	for name, mutate := range map[string]func(*clearedjobsLD){
		"title":    func(l *clearedjobsLD) { l.Title = " " },
		"employer": func(l *clearedjobsLD) { l.HiringOrganization.Name = "" },
		"body":     func(l *clearedjobsLD) { l.Description = "" },
	} {
		ld := full
		mutate(&ld)
		if _, ok := clearedjobsJob(ld, clearedjobsPosting{id: "1"}); ok {
			t.Errorf("a posting without a %s was accepted", name)
		}
	}
}

func TestClearedJobsRetriesARefusal(t *testing.T) {
	f := &clearedjobsFake{refuseFirst: true}
	jobs, err := newTestClearedJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	hydrated := 0
	for _, j := range jobs {
		if j.Description != "" {
			hydrated++
		}
	}
	if hydrated != 3 {
		t.Errorf("hydrated %d postings through the refusals, want 3", hydrated)
	}
}

// The newest-postings page only adds to the sitemap's listing, so losing it leaves a stale
// listing, not a failed crawl.
func TestClearedJobsNewestPageFailureIsNotFatal(t *testing.T) {
	f := &clearedjobsFake{newestErr: errors.New("502 bad gateway")}
	postings, err := newTestClearedJobs(f).list(context.Background())
	if err != nil {
		t.Fatalf("a failed newest-postings page failed the crawl: %v", err)
	}
	if len(postings) != 3 {
		t.Errorf("listed %d postings, want the sitemap's 3", len(postings))
	}
}

func TestClearedJobsSitemapFailureFailsTheCrawl(t *testing.T) {
	f := &clearedjobsFake{sitemapErr: errors.New("502 bad gateway")}
	if _, err := newTestClearedJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Error("a failed sitemap did not fail the crawl")
	}
}

// A crawl whose every page is a challenge read nothing: it must fail rather than report a quiet
// board.
func TestClearedJobsAllChallengedFailsTheCrawl(t *testing.T) {
	f := &clearedjobsFake{allChallenged: true}
	if _, err := newTestClearedJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no page did not fail")
	}
}

func TestClearedJobsParsePosting(t *testing.T) {
	for raw, want := range map[string]string{
		"https://clearedjobs.net/job/okta-iam-identity-integration-engineer-1959852":       "1959852",
		"https://clearedjobs.net/job/full-stack-developer-1950486?utm=x":                   "1950486",
		"https://clearedjobs.net/company/peraton-269509":                                   "",
		"https://clearedjobs.net/job/missing-id":                                           "",
		"https://evil.example/job/okta-iam-identity-integration-engineer-1959852":          "",
		"http://clearedjobs.net/job/okta-iam-identity-integration-engineer-1959852":        "",
		"https://clearedjobs.net/c-top-secret-jobs.html":                                   "",
		"https://clearedjobs.net/job/senior-network-systems-engineer-stuttgart-bw-1965285": "1965285",
	} {
		p, ok := clearedjobsParsePosting(raw)
		if ok != (want != "") || p.id != want {
			t.Errorf("clearedjobsParsePosting(%q) = %q %v, want %q", raw, p.id, ok, want)
		}
	}
}

func TestClearedJobsIsAnAggregatorWithAGraceWindow(t *testing.T) {
	s := NewClearedJobs(nil)
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("clearedjobs is not an aggregator")
	}
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("clearedjobs is not boardless")
	}
	if g, ok := s.(sweepGrace); !ok || g.SweepGrace() != clearedjobsSweepGrace {
		t.Error("clearedjobs does not declare its sweep grace")
	}
}
