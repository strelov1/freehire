package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// turijobsFake serves the real fixtures (fetched 2026-10-27): the active-offers sitemap trimmed
// to three postings and a non-posting URL, and one posting page trimmed to its JobPosting. A
// posting with no page fixture answers 404, as a gone posting does.
type turijobsFake struct {
	pageErr error

	mu    sync.Mutex
	pages []string
}

const turijobsPageID = "358177"

func (f *turijobsFake) GetXML(_ context.Context, u string, v any) error {
	if u != turijobsSitemapURL {
		return fmt.Errorf("turijobsFake: no sitemap %s", u)
	}
	raw, err := os.ReadFile("testdata/turijobs/sitemap.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *turijobsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if !strings.HasSuffix(u, "/"+turijobsPageID) {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/turijobs/posting.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestTurijobs(f *turijobsFake) turijobs {
	s := NewTurijobs(f, f).(turijobs)
	s.limiter = noWait{}
	return s
}

func TestTurijobsHydratesNewPostingsNewestFirst(t *testing.T) {
	f := &turijobsFake{}
	jobs, err := newTestTurijobs(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "357140" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want the refreshed and the hydrated posting", jobs)
	}
	if !jobs[0].SeenRefresh || jobs[0].ExternalID != "357140" {
		t.Errorf("seen posting = %+v", jobs[0])
	}
	j := jobs[1]
	if j.ExternalID != turijobsPageID || j.Company != "Viajes Esport Actiu" {
		t.Errorf("identity = %q %q", j.ExternalID, j.Company)
	}
	if j.Title != "Administrativo o administrativa con experiencia en agencia de viajes y gestión de grupos" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Location != "Náquera, Valencia" || len(j.Countries) != 1 || j.Countries[0] != "es" {
		t.Errorf("place = %q %v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 17200 || *j.SalaryMax != 18200 || j.SalaryPeriod != "year" || j.SalaryCurrency != "EUR" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !strings.Contains(j.Description, "Esport Actiu") {
		t.Errorf("description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-09-27" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	// Newest id first: 358178 (gone) is asked before 358177; 357140 is seen and never asked.
	if len(f.pages) != 2 {
		t.Errorf("pages = %v", f.pages)
	}
}

func TestTurijobsReadsOnlyItsOwnHost(t *testing.T) {
	for _, raw := range []string{
		"https://evil.example/es-es/oferta-trabajo/x/y/1",
		"http://www.turijobs.com/es-es/oferta-trabajo/x/y/1",
	} {
		if _, ok := turijobsParsePosting(raw); ok {
			t.Errorf("read %q as a posting", raw)
		}
	}
}

func TestTurijobsGateSkipsRejectedTitles(t *testing.T) {
	f := &turijobsFake{}
	gate := DetailGate{Rejected: func(j Job) bool { return true }}
	jobs, err := newTestTurijobs(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || len(f.pages) != 0 {
		t.Errorf("jobs = %d, pages = %d; want three body-less postings and no request", len(jobs), len(f.pages))
	}
}

func TestTurijobsNoPageAnsweringFailsTheCrawl(t *testing.T) {
	f := &turijobsFake{pageErr: errors.New("held open")}
	if _, err := newTestTurijobs(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestTurijobsLeavesUnreadPostingsNearTheDeadline(t *testing.T) {
	f := &turijobsFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestTurijobs(f).FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("a run out of time is not a failed crawl: %v", err)
	}
	if len(jobs) != 0 || len(f.pages) != 0 {
		t.Errorf("jobs = %d, pages = %d; want nothing read inside the margin", len(jobs), len(f.pages))
	}
}
