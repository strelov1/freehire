package sources

import (
	"context"
	"embed"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

//go:embed testdata/awwwards
var awwwardsFixtures embed.FS

// awwwardsFake serves trimmed fixtures in the shape of the live pages (2026-11-22): listing page
// 1 with two postings, page 2 with a third, page 3 a 404 as the site answers past its end, and
// the programming category naming creative-developer-12 alone.
type awwwardsFake struct {
	progErr error
}

func (f awwwardsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	var path string
	switch {
	case u == awwwardsJobsURL:
		path = "testdata/awwwards/list1.html"
	case u == awwwardsJobsURL+"?page=2":
		path = "testdata/awwwards/list2.html"
	case u == awwwardsProgrammingURL:
		if f.progErr != nil {
			return nil, f.progErr
		}
		path = "testdata/awwwards/programming.html"
	case strings.HasPrefix(u, awwwardsJobsURL+"?page="), strings.HasPrefix(u, awwwardsProgrammingURL+"?page="):
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	case strings.HasSuffix(u, "/creative-developer-12.html"):
		path = "testdata/awwwards/detail_creative-developer-12.html"
	case strings.HasSuffix(u, "/art-director-3.html"):
		path = "testdata/awwwards/detail_art-director-3.html"
	case strings.HasSuffix(u, "/manual-qa-tester-london.html"):
		path = "testdata/awwwards/detail_manual-qa-tester-london.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := awwwardsFixtures.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func TestAwwwardsFetch(t *testing.T) {
	s := awwwards{http: awwwardsFake{}}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3: %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	dev := byID["creative-developer-12"]
	if dev.Title != "Creative Developer" || dev.Company != "Frontify" || dev.Location != "United Kingdom" {
		t.Errorf("dev = %+v", dev)
	}
	if dev.PostedAt == nil || dev.PostedAt.Format("2006-01-02") != "2026-11-03" {
		t.Errorf("posted = %v", dev.PostedAt)
	}
	if !dev.IsTechHint {
		t.Error("a posting in the programming category must be hinted technical")
	}
	if dev.URL != "https://www.awwwards.com/jobs/creative-developer-12.html" {
		t.Errorf("url = %q", dev.URL)
	}
	if strings.Contains(dev.Description, "ql-ui") || !strings.Contains(dev.Description, "Interactive experiences") {
		t.Errorf("description = %q", dev.Description)
	}
	for _, id := range []string{"art-director-3", "manual-qa-tester-london"} {
		if byID[id].IsTechHint {
			t.Errorf("%s must not be hinted technical", id)
		}
	}
}

func TestAwwwardsFetchNewRefreshesSeen(t *testing.T) {
	s := awwwards{http: awwwardsFake{}}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != "art-director-3" })
	if err != nil {
		t.Fatal(err)
	}
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		}
	}
	if len(jobs) != 3 || refreshed != 2 {
		t.Fatalf("jobs=%d refreshed=%d, want 3 and 2", len(jobs), refreshed)
	}
}

func TestAwwwardsUnreadableProgrammingCategoryKeepsTheCrawl(t *testing.T) {
	s := awwwards{http: awwwardsFake{progErr: errors.New("boom")}}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for _, j := range jobs {
		if j.IsTechHint {
			t.Errorf("%s hinted technical without the category", j.ExternalID)
		}
	}
}
