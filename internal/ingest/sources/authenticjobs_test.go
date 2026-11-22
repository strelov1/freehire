package sources

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
)

//go:embed testdata/authenticjobs
var authenticjobsFixtures embed.FS

// authenticjobsFake serves trimmed live fixtures (2026-11-22) by URL and records every URL asked.
type authenticjobsFake struct {
	routes map[string]string // URL -> fixture; a missing route answers 404
	errs   map[string]error
	asked  []string
}

func (f *authenticjobsFake) PostFormJSON(_ context.Context, u string, _ url.Values, v any) error {
	return f.get(u, v)
}

func (f *authenticjobsFake) GetJSON(_ context.Context, u string, v any) error { return f.get(u, v) }

func (f *authenticjobsFake) get(u string, v any) error {
	f.asked = append(f.asked, u)
	if err := f.errs[u]; err != nil {
		return err
	}
	name, ok := f.routes[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := authenticjobsFixtures.ReadFile("testdata/authenticjobs/" + name)
	if err != nil {
		panic(err)
	}
	return json.Unmarshal(raw, v)
}

const (
	ajListing = authenticjobsBase + "/jm-ajax/get_listings/"
	ajREST    = authenticjobsBase + "/wp-json/wp/v2/"
)

func newAuthenticjobsFake() *authenticjobsFake {
	return &authenticjobsFake{routes: map[string]string{
		ajListing:                     "listing.json",
		ajREST + "job-listings/36676": "36676.json",
		ajREST + "job-listings/36673": "36673.json",
		ajREST + "job-listings/36600": "36600.json",
		ajREST + "job-categories/21":  "cat21.json",
		ajREST + "job-categories/23":  "cat23.json",
	}}
}

func fetchAuthenticjobs(f *authenticjobsFake) ([]Job, error) {
	return authenticjobs{http: f, limiter: noWait{}}.Fetch(context.Background(), CompanyEntry{})
}

func TestAuthenticJobsFetch(t *testing.T) {
	f := newAuthenticjobsFake()
	jobs, err := fetchAuthenticjobs(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the filled posting is dropped): %+v", len(jobs), jobs)
	}
	d, r := jobs[0], jobs[1]
	if d.ExternalID != "36676" || d.Title != "Product Designer, Growth" || d.Company != "Discord" ||
		d.Location != "San Francisco Bay Area" || d.EmploymentType != "full_time" || d.IsTechHint || d.Remote || d.WorkMode != "" {
		t.Errorf("designer posting mapped wrong: %+v", d)
	}
	if d.URL != "https://authenticjobs.com/job/36676/discord-product-designer-growth/" || d.PostedAt == nil || d.Description == "" {
		t.Errorf("designer url/date/body wrong: %+v", d)
	}
	if r.Company != "Reddit" || r.EmploymentType != "contract" || !r.IsTechHint || r.WorkMode != "remote" || !r.Remote {
		t.Errorf("engineer posting mapped wrong: %+v", r)
	}
	for _, u := range f.asked {
		if parsed, err := url.Parse(u); err != nil || parsed.RawQuery != "" {
			t.Errorf("requested %s: robots.txt disallows query strings", u)
		}
	}
}

func TestAuthenticJobsEmptyListing(t *testing.T) {
	f := &authenticjobsFake{routes: map[string]string{ajListing: "empty.json"}}
	jobs, err := fetchAuthenticjobs(f)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty listing = %v, %v; want no jobs, no error", jobs, err)
	}
}

func TestAuthenticJobsUnreadableDetailIsAMarker(t *testing.T) {
	f := newAuthenticjobsFake()
	f.errs = map[string]error{ajREST + "job-listings/36673": errors.New("boom")}
	jobs, err := fetchAuthenticjobs(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(unreadableMarkers(jobs)) != 1 || len(readPostings(jobs)) != 1 {
		t.Fatalf("want one marker and one read posting, got %+v", jobs)
	}
}

func TestAuthenticJobsFailsWhenNothingIsRead(t *testing.T) {
	f := newAuthenticjobsFake()
	f.errs = map[string]error{
		ajREST + "job-listings/36676": errors.New("boom"),
		ajREST + "job-listings/36673": errors.New("boom"),
	}
	if _, err := fetchAuthenticjobs(f); err == nil {
		t.Fatal("a crawl that read none of its listed postings must fail")
	}
}

func TestAuthenticJobsMultiPageListingFails(t *testing.T) {
	f := newAuthenticjobsFake()
	f.routes[ajListing] = "multipage.json"
	if _, err := fetchAuthenticjobs(f); err == nil {
		t.Fatal("a listing spanning several pages must fail, not truncate")
	}
}
