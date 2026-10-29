package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// prospectiveFake serves a real medium listing (UPZ Bern, fetched 2026-10-29) trimmed to two
// jobs plus one hosted elsewhere, its total set to match, and one real posting page trimmed to
// its JobPosting. Any other medium answers 400, as an unknown medium does; any other page 404.
type prospectiveFake struct {
	total *int // overrides the stated total when set

	mu    sync.Mutex
	pages []string
}

const prospectiveFakeMedium = "1000842"

func (f *prospectiveFake) GetJSON(_ context.Context, u string, v any) error {
	if !strings.HasPrefix(u, prospectiveAPI+"/medium/"+prospectiveFakeMedium+"/jobs?") {
		return &StatusError{Method: "GET", Code: 400, URL: u}
	}
	if !strings.Contains(u, "offset=0") {
		total := 3
		if f.total != nil {
			total = *f.total
		}
		return json.Unmarshal([]byte(fmt.Sprintf(`{"total":%d,"jobs":[]}`, total)), v)
	}
	raw, err := os.ReadFile("testdata/prospective/medium.json")
	if err != nil {
		return err
	}
	if f.total != nil {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		m["total"] = *f.total
		raw, _ = json.Marshal(m)
	}
	return json.Unmarshal(raw, v)
}

func (f *prospectiveFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if u != prospectiveAPI+"/jobs/00000000-0000-4000-8000-000000000001" {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/prospective/posting.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestProspective(f *prospectiveFake) prospective {
	s := NewProspective(f, f).(prospective)
	s.limiter = noWait{}
	return s
}

func TestProspectiveHydratesNewJobsAndRefreshesSeen(t *testing.T) {
	f := &prospectiveFake{}
	e := CompanyEntry{Board: prospectiveFakeMedium, Company: "UPZ"}
	jobs, err := newTestProspective(f).FetchNew(context.Background(), e, func(id string) bool { return id == "10240991" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want the refreshed job and the hydrated one (the redirect skipped)", jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if r := byID["10240991"]; !r.SeenRefresh || r.URL != prospectiveAPI+"/jobs/00000000-0000-4000-8000-000000000002" {
		t.Errorf("seen job = %+v, want a SeenRefresh at its directlink", r)
	}
	if _, ok := byID["10239999"]; ok {
		t.Error("a job hosted on another platform was yielded")
	}
	j := byID["10241924"]
	if j.Title != "Sachbearbeitung Frontoffice" || j.Company != "Universitäres Psychiatrisches Zentrum Bern AG" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Bern, Region Bern" || len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("location = %q countries = %v, want Bern, Region Bern / [ch]", j.Location, j.Countries)
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("employment type = %q, want part_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	for _, part := range []string{"Visitenkarte", "Empfang und die Telefonzentrale", "kaufmännische", "Requirements", "About the company"} {
		if !strings.Contains(j.Description, part) {
			t.Errorf("description lacks %q", part)
		}
	}
	for _, u := range f.pages {
		if strings.Contains(u, "000000000002") {
			t.Errorf("fetched the page of a seen job: %q", u)
		}
	}
}

func TestProspectiveShortListingFailsTheBoard(t *testing.T) {
	four := 4
	_, err := newTestProspective(&prospectiveFake{total: &four}).FetchNew(context.Background(),
		CompanyEntry{Board: prospectiveFakeMedium}, func(string) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "read 3 jobs of the 4") {
		t.Errorf("err = %v, want the shortfall reported", err)
	}
}

func TestProspectiveUnknownMediumIsGone(t *testing.T) {
	_, err := newTestProspective(&prospectiveFake{}).FetchNew(context.Background(), CompanyEntry{Board: "999"}, func(string) bool { return false })
	if !errors.Is(err, ErrBoardGone) {
		t.Errorf("err = %v, want ErrBoardGone", err)
	}
}

func TestProspectiveRejectsNonMediumBoard(t *testing.T) {
	if _, err := newTestProspective(&prospectiveFake{}).FetchNew(context.Background(), CompanyEntry{Board: "jobs.coopjobs.ch"}, nil); err == nil {
		t.Error("want an error for a board that is not a medium id")
	}
}

func TestProspectiveUnreadablePageIsAMarker(t *testing.T) {
	s := prospective{pages: emptyPageGetter{}, limiter: noWait{}}
	j, ok := s.detail(context.Background(), CompanyEntry{Company: "UPZ"},
		prospectivePosting{id: "1", url: "https://jobs.example.ch/x", job: prospectiveJob{ViewKey: "00000000-0000-4000-8000-000000000001"}})
	if !ok || !j.Unreadable {
		t.Errorf("job = %+v ok = %v, want an unreadable marker", j, ok)
	}
}

func TestProspectiveTitleDropsMarkup(t *testing.T) {
	got := prospectiveTitle(prospectiveJob{Title: "Fahrer Kat. CE<BR />(16.00 Uhr) &amp; mehr"})
	if want := "Fahrer Kat. CE (16.00 Uhr) & mehr"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}
