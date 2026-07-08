package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"testing"
)

// jobnetHTTP is a page-aware test HeaderJSONGetter. Jobnet has one endpoint — the paged BFF
// search (/bff/FindJob/Search?PageNumber=) — so this fake routes by the PageNumber query
// parameter and records both the pages requested (to assert pagination stops at
// totalJobAdCount / the first empty page) and the x-csrf header seen (to assert it is sent).
type jobnetHTTP struct {
	pages    map[int]string // search body keyed by requested PageNumber
	failPage map[int]bool   // a specific page's request fails
	gotPages []int
	gotCSRF  string
}

var jobnetPageRE = regexp.MustCompile(`PageNumber=(\d+)`)

func (f *jobnetHTTP) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.gotCSRF = headers["x-csrf"]
	page := 1
	if m := jobnetPageRE.FindStringSubmatch(url); m != nil {
		page, _ = strconv.Atoi(m[1])
	}
	f.gotPages = append(f.gotPages, page)
	if f.failPage[page] {
		return errors.New("jobnetHTTP: boom")
	}
	raw, ok := f.pages[page]
	if !ok {
		raw = `{"jobAds":[],"totalJobAdCount":0}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestJobnetProvider(t *testing.T) {
	if got := NewJobnet(nil).Provider(); got != "jobnet" {
		t.Errorf("Provider() = %q, want %q", got, "jobnet")
	}
}

func TestJobnetFetchMapsAd(t *testing.T) {
	fake := &jobnetHTTP{pages: map[int]string{1: `{"totalJobAdCount":1,"jobAds":[
		{"jobAdId":"c1dc33fa-9c57-4592-a050-36ecda5bdfc1","title":"IT-supportelev",
		 "hiringOrgName":"ipnordic A/S","postalDistrictName":"Gråsten","country":"Danmark",
		 "publicationDate":"2026-07-07T00:00:00+02:00","jobAdUrl":"",
		 "description":"<p>Er du nysgerrig</p><script>x()</script>"}
	]}`}}

	jobs, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "c1dc33fa-9c57-4592-a050-36ecda5bdfc1" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "IT-supportelev" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "ipnordic A/S" {
		t.Errorf("Company = %q", j.Company)
	}
	// jobAdUrl empty → the canonical id-based posting page is the link.
	if want := "https://job.jobnet.dk/CV/FindWork/Details/c1dc33fa-9c57-4592-a050-36ecda5bdfc1"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Gråsten, Danmark" {
		t.Errorf("Location = %q, want %q", j.Location, "Gråsten, Danmark")
	}
	// HTML is sanitized: the <script> is stripped, the text kept.
	if got := j.Description; got == "" || regexp.MustCompile(`(?i)<script`).MatchString(got) {
		t.Errorf("Description not sanitized: %q", got)
	}
	if j.PostedAt == nil {
		t.Fatal("PostedAt = nil, want parsed date")
	}
	if fake.gotCSRF != "1" {
		t.Errorf("x-csrf header = %q, want %q", fake.gotCSRF, "1")
	}
}

// An external ad carries its own destination URL; the adapter must prefer it over the
// synthesized jobnet page so the link points at the real posting.
func TestJobnetFetchPrefersAdURL(t *testing.T) {
	fake := &jobnetHTTP{pages: map[int]string{1: `{"totalJobAdCount":1,"jobAds":[
		{"jobAdId":"abc","title":"Dev","hiringOrgName":"Acme","country":"Danmark",
		 "jobAdUrl":"https://acme.example/jobs/1","description":"<p>x</p>"}
	]}`}}
	jobs, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://acme.example/jobs/1" {
		t.Fatalf("URL = %v, want the ad's own jobAdUrl", jobs)
	}
}

// An ad with no id to key on or no employer name (which would break the company slug) is
// dropped rather than emitted, mirroring the other aggregator adapters.
func TestJobnetFetchDropsIncompleteAds(t *testing.T) {
	fake := &jobnetHTTP{pages: map[int]string{1: `{"totalJobAdCount":3,"jobAds":[
		{"jobAdId":"","title":"No id","hiringOrgName":"Acme","description":"x"},
		{"jobAdId":"has-id","title":"No company","hiringOrgName":"","description":"x"},
		{"jobAdId":"ok","title":"Good","hiringOrgName":"Acme","description":"x"}
	]}`}}
	jobs, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "ok" {
		t.Fatalf("got %v, want only the complete ad", jobs)
	}
}

func TestJobnetFetchPaginates(t *testing.T) {
	// Page 1 reports a total far above one page, so the crawl requests page 2; page 2 is
	// empty (the unknown-page default), which terminates enumeration.
	fake := &jobnetHTTP{pages: map[int]string{
		1: `{"totalJobAdCount":9999,"jobAds":[{"jobAdId":"a","title":"A","hiringOrgName":"Acme","description":"x"}]}`,
	}}
	jobs, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want 1", len(jobs))
	}
	if len(fake.gotPages) != 2 || fake.gotPages[0] != 1 || fake.gotPages[1] != 2 {
		t.Errorf("requested pages = %v, want [1 2]", fake.gotPages)
	}
}

// totalJobAdCount stops pagination without a wasted trailing request: page 1 already covers
// the whole (tiny) catalogue, so page 2 is never requested.
func TestJobnetFetchStopsAtTotal(t *testing.T) {
	fake := &jobnetHTTP{pages: map[int]string{
		1: `{"totalJobAdCount":1,"jobAds":[{"jobAdId":"a","title":"A","hiringOrgName":"Acme","description":"x"}]}`,
	}}
	if _, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.gotPages) != 1 {
		t.Errorf("requested pages = %v, want only [1]", fake.gotPages)
	}
}

func TestJobnetFetchFirstPageErrorFails(t *testing.T) {
	fake := &jobnetHTTP{failPage: map[int]bool{1: true}}
	if _, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"}); err == nil {
		t.Fatal("Fetch: want error when the first page fails")
	}
}

// A later page failing ends enumeration with the jobs gathered so far, rather than failing
// the whole board.
func TestJobnetFetchLaterPageErrorKeepsJobs(t *testing.T) {
	fake := &jobnetHTTP{
		pages:    map[int]string{1: `{"totalJobAdCount":9999,"jobAds":[{"jobAdId":"a","title":"A","hiringOrgName":"Acme","description":"x"}]}`},
		failPage: map[int]bool{2: true},
	}
	jobs, err := NewJobnet(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want the 1 gathered before the failing page", len(jobs))
	}
}

// compile-time guard that the fake satisfies the role the adapter depends on.
var _ HeaderJSONGetter = (*jobnetHTTP)(nil)
