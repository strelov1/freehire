package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// instahyreFake serves the real fixtures (fetched 2026-10-25: three rows of the large-company
// search, and the Roku posting's page trimmed to its JobPosting). A page with no fixture fails as
// a refused read would; searches maps an offset to a response.
type instahyreFake struct {
	searches map[string]string
	pageErr  error

	mu    sync.Mutex
	asked []url.Values
	pages []string
}

func (f *instahyreFake) GetJSON(_ context.Context, u string, v any) error {
	rest, ok := strings.CutPrefix(u, instahyreSearchURL)
	if !ok {
		return fmt.Errorf("instahyreFake: no route for %s", u)
	}
	q, err := url.ParseQuery(rest)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.asked = append(f.asked, q)
	f.mu.Unlock()
	raw, ok := f.searches[q.Get("offset")]
	if !ok {
		return fmt.Errorf("instahyreFake: no search at offset %s", q.Get("offset"))
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *instahyreFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if !strings.HasPrefix(u, instahyreBase+"/job-447488-") {
		return nil, &StatusError{Method: "GET", Code: 403, URL: u}
	}
	raw, err := os.ReadFile("testdata/instahyre/detail_447488.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func instahyreSearchFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/instahyre/search.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestInstahyreFetchReadsNewPostingsFromTheirPages(t *testing.T) {
	f := &instahyreFake{searches: map[string]string{"0": instahyreSearchFixture(t)}}
	jobs, err := NewInstahyre(f, f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "2"},
		func(id string) bool { return id == "331759" })
	if err != nil {
		t.Fatal(err)
	}
	if q := f.asked[0]; q.Get("company_size") != "2" || q.Get("limit") != "35" || q.Get("offset") != "0" {
		t.Errorf("search query = %v", q)
	}
	// 331759 is stored: no page. 447892's page fails and is deferred; 447488's is read.
	if len(f.pages) != 2 {
		t.Errorf("read %d pages, want 2: %v", len(f.pages), f.pages)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 2 || !byID["331759"].SeenRefresh {
		t.Fatalf("jobs = %+v, want the stored posting refreshed and one hydrated", jobs)
	}
	if _, ok := byID["447892"]; ok {
		t.Error("a posting whose page failed was stored body-less")
	}
	j := byID["447488"]
	if j.Title != "Sr. Manager - Software Engineering" || j.Company != "Roku" || j.Location != "Bangalore" {
		t.Errorf("title/company/location = %q / %q / %q", j.Title, j.Company, j.Location)
	}
	if j.URL != "https://www.instahyre.com/job-447488-sr-manager-software-engineering-at-roku-bangalore/" {
		t.Errorf("url = %q", j.URL)
	}
	if j.EmploymentType != "full_time" || j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-20" {
		t.Errorf("employment %q posted %v", j.EmploymentType, j.PostedAt)
	}
	if j.Description == "" || strings.Contains(j.Description, "<body>") {
		t.Errorf("description = %.120q", j.Description)
	}
	if !slices.Equal(j.FallbackCountries, []string{"in"}) {
		t.Errorf("fallback countries = %v", j.FallbackCountries)
	}
}

func TestInstahyreFailsWhenNoPageAnswers(t *testing.T) {
	f := &instahyreFake{searches: map[string]string{"0": instahyreSearchFixture(t)}, pageErr: errors.New("challenge")}
	if _, err := NewInstahyre(f, f).Fetch(context.Background(), CompanyEntry{Board: "2"}); err == nil {
		t.Fatal("a crawl whose every page failed reported success")
	}
}

func TestInstahyreGateSkipsTheRead(t *testing.T) {
	f := &instahyreFake{searches: map[string]string{"0": instahyreSearchFixture(t)}}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		return map[string]bool{"Roku": true, "FourKites": true, "Kotak811": true}
	}}
	jobs, err := NewInstahyre(f, f).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{Board: "2"}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 || len(jobs) != 3 {
		t.Errorf("read %d pages and yielded %d jobs, want 0 and 3", len(f.pages), len(jobs))
	}
}

func TestInstahyreWalksTheBucketToItsTotal(t *testing.T) {
	page := func(from, n int) string {
		var rows []map[string]any
		for i := range n {
			id := from + i
			rows = append(rows, map[string]any{"id": id, "title": "SDE", "locations": "Pune",
				"public_url": fmt.Sprintf("%s/job-%d-sde-at-acme-pune/", instahyreBase, id),
				"employer":   map[string]any{"company_name": "Acme"}})
		}
		out, _ := json.Marshal(map[string]any{"meta": map[string]any{"total_count": 40}, "objects": rows})
		return string(out)
	}
	f := &instahyreFake{searches: map[string]string{"0": page(1, 35), "35": page(36, 5)}}
	jobs, err := NewInstahyre(f, f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "1"}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.asked) != 2 || len(jobs) != 40 {
		t.Errorf("asked %d pages and got %d jobs, want 2 and 40", len(f.asked), len(jobs))
	}
}

func TestInstahyreRejectsAnUnknownBucket(t *testing.T) {
	f := &instahyreFake{}
	if _, err := NewInstahyre(f, f).Fetch(context.Background(), CompanyEntry{Board: "4"}); err == nil {
		t.Fatal("an unknown company size was accepted")
	}
}

func TestInstahyreRemotePlaces(t *testing.T) {
	cases := []struct{ in, loc, mode string }{
		{"Bangalore", "Bangalore", ""},
		{"Work From Home", "Remote", "remote"},
		{"Bangalore,Hyderabad,Work From Home", "Bangalore; Hyderabad; Remote", ""},
	}
	for _, c := range cases {
		r := instahyreListing{ID: 1, Title: "SDE", Locations: c.in, PublicURL: instahyreBase + "/job-1-sde/"}
		r.Employer.CompanyName = "Acme"
		j, ok := r.toJob()
		if !ok || j.Location != c.loc || j.WorkMode != c.mode {
			t.Errorf("%q => %q/%q, want %q/%q", c.in, j.Location, j.WorkMode, c.loc, c.mode)
		}
	}
}
