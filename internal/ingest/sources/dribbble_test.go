package sources

import (
	"context"
	"embed"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/dribbble
var dribbbleFixtures embed.FS

// dribbbleFake serves trimmed fixtures in the shape of the live pages (2026-11-22): listing page
// 1 with two postings, page 2 with a CLOSED posting, a repeat and a third, page 3 card-less.
// Postings 183719 (on-site) and 296381 (remote) have pages; 222494 answers 404.
type dribbbleFake struct {
	mu    sync.Mutex
	pages []string
}

func (f *dribbbleFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	var path string
	switch {
	case u == dribbbleJobsURL:
		path = "testdata/dribbble/list1.html"
	case u == dribbbleJobsURL+"?page=2":
		path = "testdata/dribbble/list2.html"
	case strings.HasPrefix(u, dribbbleJobsURL+"?page="):
		path = "testdata/dribbble/list_empty.html"
	case strings.Contains(u, "/jobs/183719-"):
		path = "testdata/dribbble/detail_onsite.html"
	case strings.Contains(u, "/jobs/296381-"):
		path = "testdata/dribbble/detail_remote.html"
	case strings.Contains(u, "/jobs/311231-"):
		path = "testdata/dribbble/detail_expired.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := dribbbleFixtures.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *dribbbleFake) postingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.pages {
		if strings.Contains(p, "/jobs/") && !strings.Contains(p, "?page=") && p != dribbbleJobsURL {
			n++
		}
	}
	return n
}

func newTestDribbble(f *dribbbleFake) dribbble {
	return dribbble{http: f, now: func() time.Time { return time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC) }}
}

func TestDribbbleFetch(t *testing.T) {
	f := &dribbbleFake{}
	jobs, err := newTestDribbble(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	// 183719 and 296381 are read; 222494 is a 404 and is deferred; the -CLOSED posting is never
	// requested and the repeated 183719 is read once.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	onsite := byID["183719"]
	if onsite.Title != "Graphic Designer" || onsite.Company != "KAP STRATEGIES" {
		t.Errorf("onsite = %+v", onsite)
	}
	if onsite.Location != "Austin, TX, United States" || onsite.Remote || onsite.WorkMode != "" {
		t.Errorf("onsite location = %q remote=%v mode=%q", onsite.Location, onsite.Remote, onsite.WorkMode)
	}
	if onsite.EmploymentType != "full_time" || onsite.PostedAt == nil || onsite.PostedAt.Year() != 2022 {
		t.Errorf("onsite type/date = %q %v", onsite.EmploymentType, onsite.PostedAt)
	}
	if !strings.Contains(onsite.Description, "graphic designer") || strings.Contains(onsite.Description, "<script") {
		t.Errorf("description = %q", onsite.Description)
	}
	if onsite.URL != "https://dribbble.com/jobs/183719-Graphic-Designer" {
		t.Errorf("url = %q", onsite.URL)
	}
	remote := byID["296381"]
	if !remote.Remote || remote.WorkMode != "remote" || remote.Location != "Remote" {
		t.Errorf("remote = %+v", remote)
	}
	if onsite.IsTechHint || remote.IsTechHint {
		t.Error("no Dribbble posting may be hinted technical")
	}
	if got := f.postingRequests(); got != 3 {
		t.Errorf("posting page requests = %d, want 3 (183719, 296381, 222494)", got)
	}
}

func TestDribbbleFetchNewRefreshesSeen(t *testing.T) {
	f := &dribbbleFake{}
	seen := func(id string) bool { return id == "183719" || id == "222494" }
	jobs, err := newTestDribbble(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.Description != "" {
				t.Errorf("refresh of %s carries a body", j.ExternalID)
			}
		}
	}
	if len(jobs) != 3 || refreshed != 2 {
		t.Fatalf("jobs=%d refreshed=%d, want 3 and 2", len(jobs), refreshed)
	}
	if got := f.postingRequests(); got != 1 {
		t.Errorf("posting page requests = %d, want 1", got)
	}
}

func TestDribbbleExpiredPostingIsSkipped(t *testing.T) {
	s := newTestDribbble(&dribbbleFake{})
	_, ok, err := s.detail(context.Background(), dribbblePosting{id: "311231", url: dribbbleBase + "/jobs/311231-Expired"})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want an expired posting skipped without error", ok, err)
	}
}

func TestDribbbleEmptyListingFails(t *testing.T) {
	f := &emptyListFake{}
	s := dribbble{http: f, now: time.Now}
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a listing with no posting must fail the crawl")
	}
}

// emptyListFake answers every page with the card-less listing.
type emptyListFake struct{}

func (emptyListFake) GetHTML(context.Context, string) (*html.Node, error) {
	raw, err := dribbbleFixtures.ReadFile("testdata/dribbble/list_empty.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func TestDribbbleLocation(t *testing.T) {
	cases := []struct {
		name   string
		places schemaPlaces
		want   string
	}{
		{"anywhere placeholder", schemaPlaces{{Address: schemaAddress{AddressLocality: "Anywhere"}}}, ""},
		{"remote placeholder", schemaPlaces{{Address: schemaAddress{AddressLocality: "Remote"}}}, ""},
		{"city and country", schemaPlaces{{Address: schemaAddress{AddressLocality: "Berlin", AddressCountry: "Germany"}}}, "Berlin, Germany"},
	}
	for _, c := range cases {
		if got := designBoardLocation(c.places); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
