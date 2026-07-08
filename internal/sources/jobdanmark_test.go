package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// jobdanmarkHTTP is a route-aware test fake for the jobdanmarkClient roles. The list is a POST
// that paginates on the page in the URL (/api/jobsearch/search/{page}); the detail is a GetHTML
// keyed by the slug at the end of the job URL. It records the pages and detail slugs requested so
// a test can assert pagination stops at totalPages and a detail is fetched per item.
type jobdanmarkHTTP struct {
	pages      map[int]string    // POST list body keyed by page
	details    map[string]string // detail HTML keyed by slug
	failPage   map[int]bool      // a specific page's POST fails
	failDetail map[string]bool   // a specific slug's detail GET fails
	gotPages   []int
	gotDetails []string
}

var jobdanmarkPageRE = regexp.MustCompile(`/search/(\d+)`)

func (f *jobdanmarkHTTP) PostJSON(_ context.Context, url string, _ any, v any) error {
	page := 1
	if m := jobdanmarkPageRE.FindStringSubmatch(url); m != nil {
		page, _ = strconv.Atoi(m[1])
	}
	f.gotPages = append(f.gotPages, page)
	if f.failPage[page] {
		return errors.New("jobdanmarkHTTP: list boom")
	}
	raw, ok := f.pages[page]
	if !ok {
		raw = `{"items":[],"totalPages":0}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *jobdanmarkHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	slug := url[strings.LastIndex(url, "/")+1:]
	f.gotDetails = append(f.gotDetails, slug)
	if f.failDetail[slug] {
		return nil, errors.New("jobdanmarkHTTP: detail boom")
	}
	raw, ok := f.details[slug]
	if !ok {
		raw = "<html></html>"
	}
	return html.Parse(strings.NewReader(raw))
}

// jobdanmarkDetailHTML wraps a JSON-LD JobPosting exactly as the live page serves it — the
// script type is HTML-entity-encoded ("application/ld&#x2B;json"), which the html parser decodes
// back to "application/ld+json" so ldJobPosting matches.
func jobdanmarkDetailHTML(datePosted, description string) string {
	ld := `{"@type":"JobPosting","datePosted":"` + datePosted + `","description":` + strconv.Quote(description) + `}`
	return `<html><head><script type="application/ld&#x2B;json">` + ld + `</script></head><body></body></html>`
}

func TestJobdanmarkProvider(t *testing.T) {
	if got := NewJobdanmark(nil).Provider(); got != "jobdanmark" {
		t.Errorf("Provider() = %q, want %q", got, "jobdanmark")
	}
}

func TestJobdanmarkFetchMapsJob(t *testing.T) {
	fake := &jobdanmarkHTTP{
		pages: map[int]string{1: `{"totalPages":1,"items":[
			{"title":"Backend Developer","companyName":"Acme A/S",
			 "companyAddress":"Storegade 25, 6261 Bredebro","url":"/job/acme-dev","publishedDate":"05-07-2026"}
		]}`},
		details: map[string]string{"acme-dev": jobdanmarkDetailHTML("2026-07-07", `<p onclick="x()">Full body here</p>`)},
	}

	jobs, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "acme-dev" {
		t.Errorf("ExternalID = %q, want acme-dev", j.ExternalID)
	}
	if j.URL != "https://jobdanmark.dk/job/acme-dev" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Backend Developer" || j.Company != "Acme A/S" {
		t.Errorf("Title/Company = %q / %q", j.Title, j.Company)
	}
	// Description comes from the detail JSON-LD as HTML (safe tags kept), run through
	// sanitizeHTML which drops the dangerous on* handler but preserves the text/markup.
	if !strings.Contains(j.Description, "Full body here") || strings.Contains(j.Description, "onclick") {
		t.Errorf("Description not extracted/sanitized: %q", j.Description)
	}
	// The portal omits the country from the address; the adapter appends it so the geo
	// dictionary resolves Denmark.
	if j.Location != "Storegade 25, 6261 Bredebro, Danmark" {
		t.Errorf("Location = %q", j.Location)
	}
	// The detail JSON-LD datePosted (ISO) is preferred over the list's DD-MM-YYYY.
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-07" {
		t.Errorf("PostedAt = %v, want 2026-07-07 from detail", j.PostedAt)
	}
	if len(fake.gotDetails) != 1 || fake.gotDetails[0] != "acme-dev" {
		t.Errorf("detail fetches = %v, want [acme-dev]", fake.gotDetails)
	}
}

// When the detail fetch fails or carries no JobPosting, the job is still emitted (the list
// already has title/company/url/date) with an empty description and the list's date.
func TestJobdanmarkFetchKeepsJobWhenDetailMissing(t *testing.T) {
	fake := &jobdanmarkHTTP{
		pages: map[int]string{1: `{"totalPages":1,"items":[
			{"title":"Dev","companyName":"Acme","companyAddress":"København","url":"/job/nodetail","publishedDate":"05-07-2026"}
		]}`},
		failDetail: map[string]bool{"nodetail": true},
	}
	jobs, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty on missing detail", jobs[0].Description)
	}
	// Falls back to the list publishedDate (DD-MM-YYYY -> 2026-07-05).
	if jobs[0].PostedAt == nil || jobs[0].PostedAt.Format("2006-01-02") != "2026-07-05" {
		t.Errorf("PostedAt = %v, want 2026-07-05 fallback", jobs[0].PostedAt)
	}
}

// An item with no title, employer, or url to key on is dropped rather than emitted.
func TestJobdanmarkFetchDropsIncompleteItems(t *testing.T) {
	fake := &jobdanmarkHTTP{pages: map[int]string{1: `{"totalPages":1,"items":[
		{"title":"","companyName":"Acme","url":"/job/a"},
		{"title":"No company","companyName":"","url":"/job/b"},
		{"title":"No url","companyName":"Acme","url":""},
		{"title":"Good","companyName":"Acme","url":"/job/ok"}
	]}`}}
	jobs, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "ok" {
		t.Fatalf("got %v, want only the complete item", jobs)
	}
}

// A trailing slash or tracking query on the item URL must not defeat the id or leak into the
// stored URL: the slug is the last real path segment and the URL is stripped of query/fragment.
func TestJobdanmarkFetchNormalizesURL(t *testing.T) {
	fake := &jobdanmarkHTTP{pages: map[int]string{1: `{"totalPages":1,"items":[
		{"title":"A","companyName":"Acme","url":"/job/acme-dev/"},
		{"title":"B","companyName":"Acme","url":"/job/other-role?utm=x#frag"}
	]}`}}
	jobs, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]string{}
	for _, j := range jobs {
		byID[j.ExternalID] = j.URL
	}
	if got := byID["acme-dev"]; got != "https://jobdanmark.dk/job/acme-dev" {
		t.Errorf("trailing-slash URL: id=acme-dev URL=%q", got)
	}
	if got := byID["other-role"]; got != "https://jobdanmark.dk/job/other-role" {
		t.Errorf("query URL: id=other-role URL=%q (query/fragment must be stripped)", got)
	}
}

func TestJobdanmarkFetchPaginates(t *testing.T) {
	fake := &jobdanmarkHTTP{pages: map[int]string{
		1: `{"totalPages":9999,"items":[{"title":"A","companyName":"Acme","url":"/job/a"}]}`,
	}}
	if _, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.gotPages) != 2 || fake.gotPages[0] != 1 || fake.gotPages[1] != 2 {
		t.Errorf("requested pages = %v, want [1 2] (page 2 empty terminates)", fake.gotPages)
	}
}

func TestJobdanmarkFetchStopsAtTotalPages(t *testing.T) {
	fake := &jobdanmarkHTTP{pages: map[int]string{
		1: `{"totalPages":1,"items":[{"title":"A","companyName":"Acme","url":"/job/a"}]}`,
	}}
	if _, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.gotPages) != 1 {
		t.Errorf("requested pages = %v, want only [1]", fake.gotPages)
	}
}

func TestJobdanmarkFetchFirstPageErrorFails(t *testing.T) {
	fake := &jobdanmarkHTTP{failPage: map[int]bool{1: true}}
	if _, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"}); err == nil {
		t.Fatal("Fetch: want error when the first page fails")
	}
}

func TestJobdanmarkFetchLaterPageErrorKeepsJobs(t *testing.T) {
	fake := &jobdanmarkHTTP{
		pages:    map[int]string{1: `{"totalPages":9999,"items":[{"title":"A","companyName":"Acme","url":"/job/a"}]}`},
		failPage: map[int]bool{2: true},
	}
	jobs, err := NewJobdanmark(fake).Fetch(context.Background(), CompanyEntry{Company: "JobiDanmark", Provider: "jobdanmark"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want the 1 gathered before the failing page", len(jobs))
	}
}

var _ jobdanmarkClient = (*jobdanmarkHTTP)(nil)
