package sources

import (
	"context"
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// karriereatFixtures holds the sitemap (trimmed to a few postings plus a company page and an
// off-host URL) and two posting pages trimmed to their JobPosting, all fetched 2026-11-22.
//
//go:embed testdata/karriereat
var karriereatFixtures embed.FS

// karriereatFake serves the fixtures. A posting with no page fixture answers 404, as a removed
// posting does.
type karriereatFake struct {
	pageErr  error
	pageHTML string

	mu    sync.Mutex
	pages []string
}

func (f *karriereatFake) GetXML(_ context.Context, u string, v any) error {
	if u != karriereatSitemapURL {
		return fmt.Errorf("karriereatFake: no sitemap %s", u)
	}
	raw, err := karriereatFixtures.ReadFile("testdata/karriereat/sitemap.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *karriereatFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if f.pageHTML != "" {
		return html.Parse(strings.NewReader(f.pageHTML))
	}
	id := u[strings.LastIndex(u, "/")+1:]
	raw, err := karriereatFixtures.ReadFile("testdata/karriereat/posting_" + id + ".html")
	if err != nil {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestKarriereAt(f *karriereatFake) karriereat {
	s := NewKarriereAt(f, f).(karriereat)
	s.limiter = noWait{}
	return s
}

func TestKarriereAtHydratesNewPostingsAndRefreshesSeen(t *testing.T) {
	f := &karriereatFake{}
	jobs, err := newTestKarriereAt(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "7611468" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %+v, want the refreshed posting and the two hydrated (the 404 is gone)", jobs)
	}
	if r := byID["7611468"]; !r.SeenRefresh || r.URL != "https://www.karriere.at/jobs/7611468" {
		t.Errorf("seen posting = %+v, want a SeenRefresh at its URL", r)
	}
	j := byID["10039924"]
	if j.Title != "IT Service Management - Service Desk Mitarbeiter:in (m/w/d)" || j.Company != "Vereinigung von Ordensschulen Österreichs" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Wien 1. Bezirk (Innere Stadt), Wien" || len(j.Countries) != 1 || j.Countries[0] != "at" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2026-11-20T13:40:36Z" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 2789 || j.SalaryMax != nil || j.SalaryCurrency != "EUR" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s, want a floor of 2789 EUR/month", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !strings.Contains(j.Description, "VOSÖ") {
		t.Errorf("description = %q", j.Description)
	}
	if y := byID["10039643"]; y.SalaryMin == nil || *y.SalaryMin != 48520 || y.SalaryPeriod != "year" {
		t.Errorf("yearly salary = %+v", y)
	}
	for _, u := range f.pages {
		if !strings.HasPrefix(u, "https://www.karriere.at/jobs/") {
			t.Errorf("fetched %q, outside karriere.at/jobs", u)
		}
		if strings.HasSuffix(u, "/7611468") {
			t.Errorf("fetched the page of a seen posting: %q", u)
		}
	}
}

func TestKarriereAtReadsNewestFirst(t *testing.T) {
	postings, err := newTestKarriereAt(&karriereatFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range postings {
		ids = append(ids, p.id)
	}
	want := "10039925,10039924,10039643,7611468"
	if got := strings.Join(ids, ","); got != want {
		t.Errorf("ids = %s, want %s (newest first, company page and off-host entry dropped)", got, want)
	}
}

func TestKarriereAtFailsWhenNoPageAnswers(t *testing.T) {
	f := &karriereatFake{pageErr: errors.New("connection refused")}
	if _, err := newTestKarriereAt(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when every page read failed")
	}
}

func TestKarriereAtFailsWhenEveryPostingLacksABody(t *testing.T) {
	f := &karriereatFake{pageHTML: `<script type="application/ld+json">
	{"@type":"JobPosting","title":"Engineer","hiringOrganization":{"name":"Acme"},"description":""}
	</script>`}
	jobs, err := newTestKarriereAt(f).Fetch(context.Background(), CompanyEntry{})
	if err == nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs and %v, want a failed crawl when every posting is incomplete", len(jobs), err)
	}
}

func TestKarriereAtJobDropsPostingWithoutEmployerOrBody(t *testing.T) {
	p := karriereatPosting{id: "1", url: "https://www.karriere.at/jobs/1"}
	if _, ok := karriereatJob(karriereatLD{Title: "x", Description: "<p>body</p>"}, p); ok {
		t.Error("a posting with no employer was kept")
	}
	ld := karriereatLD{Title: "x"}
	ld.HiringOrganization.Name = "Acme"
	if _, ok := karriereatJob(ld, p); ok {
		t.Error("a posting with no body was kept")
	}
}
