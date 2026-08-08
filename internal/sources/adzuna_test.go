package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"
)

// adzunaHTTP is a paging-aware test JSONGetter, mirroring echojobsHTTP: it serves pages[i] for a
// request whose URL path ends in /search/<i+1> (Adzuna encodes the page number in the path, not a
// query param) and records every URL requested so a test can assert credentials and routing.
type adzunaHTTP struct {
	pages    []string
	failPage int // 0 = never fail
	gotURLs  []string
}

func (f *adzunaHTTP) GetJSON(_ context.Context, u string, v any) error {
	f.gotURLs = append(f.gotURLs, u)
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(path.Base(parsed.Path))
	if f.failPage != 0 && page == f.failPage {
		return errors.New("adzunaHTTP: boom")
	}
	if page < 1 || page > len(f.pages) {
		return fmt.Errorf("adzunaHTTP: no page %d", page)
	}
	return json.Unmarshal([]byte(f.pages[page-1]), v)
}

func adzunaJobJSON(id, title string) string {
	return fmt.Sprintf(`{
		"id":%q,"title":%q,"description":"Great role.","created":"2026-08-07T13:10:31Z",
		"redirect_url":"https://www.adzuna.co.uk/jobs/land/ad/%s",
		"company":{"display_name":"Acme"},
		"location":{"display_name":"Leeds, West Yorkshire"}
	}`, id, title, id)
}

func adzunaPageJSON(jobs string) string {
	return fmt.Sprintf(`{"count":999,"results":[%s]}`, jobs)
}

func TestAdzunaFetchMapsFields(t *testing.T) {
	http := &adzunaHTTP{pages: []string{
		adzunaPageJSON(adzunaJobJSON("111", "Backend Engineer")),
		adzunaPageJSON(""),
	}}
	adz := adzuna{http: http, appID: "id1", appKey: "key1"}

	jobs, err := adz.Fetch(context.Background(), CompanyEntry{Region: "gb", Board: "it-jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	if job.ExternalID != "111" || job.Title != "Backend Engineer" || job.Company != "Acme" {
		t.Fatalf("identity mismatch: %+v", job)
	}
	if job.URL != "https://www.adzuna.co.uk/jobs/land/ad/111" {
		t.Fatalf("URL: %q", job.URL)
	}
	if job.Location != "Leeds, West Yorkshire" {
		t.Fatalf("Location: %q", job.Location)
	}
	if job.PostedAt == nil {
		t.Fatalf("PostedAt not parsed")
	}
	if job.Description != "Great role." {
		t.Fatalf("Description: %q", job.Description)
	}
}

// Adzuna emits id as a JSON string for most postings but as a bare JSON number for some
// (observed live 2026-08-08, gb/it-jobs page 7) — an upstream inconsistency, not a documented
// alternate shape. A page hitting it must not fail the whole page's decode.
func TestAdzunaFetchAcceptsNumericID(t *testing.T) {
	numericIDJob := `{
		"id":123456789,"title":"Backend Engineer","description":"Great role.",
		"created":"2026-08-07T13:10:31Z",
		"redirect_url":"https://www.adzuna.co.uk/jobs/land/ad/123456789",
		"company":{"display_name":"Acme"},"location":{"display_name":"Leeds, West Yorkshire"}
	}`
	http := &adzunaHTTP{pages: []string{adzunaPageJSON(numericIDJob), adzunaPageJSON("")}}

	jobs, err := (adzuna{http: http, appID: "id", appKey: "key"}).Fetch(context.Background(), CompanyEntry{Region: "gb", Board: "it-jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "123456789" {
		t.Fatalf("want ExternalID \"123456789\", got %+v", jobs)
	}
}

// The credential and the board's country/category must reach the request.
func TestAdzunaFetchRequestsCorrectURL(t *testing.T) {
	http := &adzunaHTTP{pages: []string{adzunaPageJSON("")}}
	adz := adzuna{http: http, appID: "myid", appKey: "mykey"}

	if _, err := adz.Fetch(context.Background(), CompanyEntry{Region: "us", Board: "it-jobs"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(http.gotURLs) != 1 {
		t.Fatalf("want 1 request, got %d", len(http.gotURLs))
	}
	got := http.gotURLs[0]
	for _, want := range []string{"/jobs/us/search/1", "app_id=myid", "app_key=mykey", "category=it-jobs"} {
		if !strings.Contains(got, want) {
			t.Fatalf("request URL %q missing %q", got, want)
		}
	}
}

// A blank country or category is refused rather than crawled (an empty country would 404, an
// empty category would fetch Adzuna's whole unfiltered country feed).
func TestAdzunaFetchRejectsBlankBoard(t *testing.T) {
	adz := adzuna{http: &adzunaHTTP{}, appID: "id", appKey: "key"}
	if _, err := adz.Fetch(context.Background(), CompanyEntry{Region: "", Board: "it-jobs"}); err == nil {
		t.Fatal("want error on blank country")
	}
	if _, err := adz.Fetch(context.Background(), CompanyEntry{Region: "gb", Board: ""}); err == nil {
		t.Fatal("want error on blank category")
	}
}

// A failed first page is a board-level error.
func TestAdzunaFetchFirstPageFailure(t *testing.T) {
	http := &adzunaHTTP{failPage: 1, pages: []string{adzunaPageJSON("")}}
	if _, err := (adzuna{http: http, appID: "id", appKey: "key"}).Fetch(context.Background(), CompanyEntry{Region: "gb", Board: "it-jobs"}); err == nil {
		t.Fatal("want error on first-page failure")
	}
}

// A later page failing ends the walk with what was gathered so far.
func TestAdzunaFetchLaterPageFailureReturnsPartial(t *testing.T) {
	http := &adzunaHTTP{
		pages:    []string{adzunaPageJSON(adzunaJobJSON("1", "A")), adzunaPageJSON(adzunaJobJSON("2", "B"))},
		failPage: 2,
	}
	jobs, err := (adzuna{http: http, appID: "id", appKey: "key"}).Fetch(context.Background(), CompanyEntry{Region: "gb", Board: "it-jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("want partial result [job 1], got %+v", jobs)
	}
}
