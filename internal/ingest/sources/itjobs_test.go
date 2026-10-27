package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// itjobsFake serves the real fixtures (fetched 2026-10-27): listing page 1 trimmed to four
// postings — two regular cards and two promoted ones, one of them repeated as the site repeats
// it — page 2 as the card-less page the site answers past its end, and two posting pages trimmed
// to their JobPosting (518173 remote, 518547 hybrid with a salary). A posting with no page
// fixture answers 404, as a gone posting does.
type itjobsFake struct {
	listErr error

	mu    sync.Mutex
	pages []string
}

func (f *itjobsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	var path string
	switch {
	case u == itjobsListURL:
		if f.listErr != nil {
			return nil, f.listErr
		}
		path = "testdata/itjobs/list.html"
	case strings.HasPrefix(u, itjobsListURL+"?page="):
		path = "testdata/itjobs/empty.html"
	case strings.Contains(u, "/oferta/518173/"):
		path = "testdata/itjobs/remote.html"
	case strings.Contains(u, "/oferta/518547/"):
		path = "testdata/itjobs/hybrid.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *itjobsFake) postingRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, p := range f.pages {
		if strings.Contains(p, "/oferta/") {
			out = append(out, p)
		}
	}
	return out
}

func newTestITJobs(f *itjobsFake) itjobs {
	s := NewITJobs(f).(itjobs)
	s.limiter = noWait{}
	return s
}

func TestITJobsHydratesNewPostingsNewestFirst(t *testing.T) {
	f := &itjobsFake{}
	jobs, err := newTestITJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "518561" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// 518087 (the promoted posting, listed twice) answers 404 and is not yielded.
	if len(jobs) != 3 {
		t.Fatalf("jobs = %+v, want the refreshed and the two hydrated postings", jobs)
	}
	refreshed := byID["518561"]
	if !refreshed.SeenRefresh || refreshed.Company != "Landskill" || refreshed.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less refresh", refreshed)
	}

	remote := byID["518173"]
	if remote.Title != "DevOps Engineer" || remote.Company != "The Virtual Forge" {
		t.Errorf("remote posting = %q at %q", remote.Title, remote.Company)
	}
	if remote.URL != itjobsBaseURL+"/oferta/518173/devops-engineer" {
		t.Errorf("url = %q", remote.URL)
	}
	if remote.WorkMode != "remote" || !remote.Remote {
		t.Errorf("work mode = %q remote=%v, want remote", remote.WorkMode, remote.Remote)
	}
	if len(remote.Countries) != 1 || remote.Countries[0] != "pt" {
		t.Errorf("countries = %v", remote.Countries)
	}
	if !strings.Contains(remote.Description, "DevOps Engineer") {
		t.Errorf("description = %q", remote.Description)
	}
	if !remote.IsTechHint {
		t.Error("an itjobs posting carries no tech hint")
	}
	if remote.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", remote.EmploymentType)
	}
	if remote.PostedAt == nil || remote.PostedAt.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("posted = %v", remote.PostedAt)
	}

	hybrid := byID["518547"]
	if hybrid.WorkMode != "hybrid" || hybrid.Remote {
		t.Errorf("work mode = %q remote=%v, want hybrid", hybrid.WorkMode, hybrid.Remote)
	}
	if hybrid.Location != "Lisboa" {
		t.Errorf("location = %q", hybrid.Location)
	}
	if hybrid.SalaryMin == nil || *hybrid.SalaryMin != 39000 || hybrid.SalaryMax == nil || *hybrid.SalaryMax != 44000 ||
		hybrid.SalaryCurrency != "EUR" || hybrid.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %q %q", hybrid.SalaryMin, hybrid.SalaryMax, hybrid.SalaryCurrency, hybrid.SalaryPeriod)
	}

	// Each unseen posting is asked once, the promoted one listed twice included; 518561 is seen
	// and never asked.
	got := f.postingRequests()
	if len(got) != 3 || strings.Contains(strings.Join(got, " "), "/518561/") {
		t.Errorf("posting requests = %v", got)
	}
}

func TestITJobsCardFacts(t *testing.T) {
	raw, err := os.ReadFile("testdata/itjobs/list.html")
	if err != nil {
		t.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]itjobsCard{}
	for _, c := range itjobsCards(root) {
		byID[c.id] = c
	}
	outsoft := byID[518087]
	if outsoft.company != "Outsoft" || outsoft.places != "Aveiro, Coimbra, Porto" || outsoft.workModel != "Híbrido" {
		t.Errorf("promoted card = %+v", outsoft)
	}
	if j := itjobsBaseJob(outsoft); j.Location != "Aveiro; Coimbra; Porto" || j.WorkMode != "hybrid" {
		t.Errorf("base job location = %q, work mode = %q", j.Location, j.WorkMode)
	}
	// A remote card names no place; the salary fact is not the work model.
	if forge := byID[518173]; forge.places != "" || forge.workModel != "Remoto" {
		t.Errorf("remote card = %+v", forge)
	}
	if fc := byID[518547]; fc.workModel != "Híbrido" || fc.places != "Lisboa" {
		t.Errorf("salaried card = %+v", fc)
	}
}

func TestITJobsGateSkipsCoveredEmployers(t *testing.T) {
	f := &itjobsFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c] = true
		}
		return out
	}}
	jobs, err := newTestITJobs(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("jobs = %d, want all four yielded body-less", len(jobs))
	}
	for _, j := range jobs {
		if j.Description != "" || j.SeenRefresh {
			t.Errorf("covered posting %s was hydrated or marked seen", j.ExternalID)
		}
	}
	if got := f.postingRequests(); len(got) != 0 {
		t.Errorf("posting requests = %v, want none", got)
	}
}

func TestITJobsFirstListingPageFailingFailsTheCrawl(t *testing.T) {
	f := &itjobsFake{listErr: errors.New("boom")}
	if _, err := newTestITJobs(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestITJobsLeavesUnreadPostingsNearTheDeadline(t *testing.T) {
	f := &itjobsFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestITJobs(f).FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("a run out of time is not a failed crawl: %v", err)
	}
	if len(jobs) != 0 || len(f.postingRequests()) != 0 {
		t.Errorf("jobs = %d, posting requests = %v; want nothing read inside the margin", len(jobs), f.postingRequests())
	}
}

func TestITJobsBoundsNewPostingsPerRun(t *testing.T) {
	f := &itjobsFake{}
	s := newTestITJobs(f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if got := f.postingRequests(); len(got) != 1 || !strings.Contains(got[0], "/518561/") {
		t.Errorf("posting requests = %v, want only the newest", got)
	}
}

func TestITJobsWorkMode(t *testing.T) {
	cases := []struct{ model, locType, want string }{
		{"Remoto", "", "remote"},
		{"Híbrido", "", "hybrid"},
		{"Presencial", "", "onsite"},
		{"", "TELECOMMUTE", "remote"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := itjobsWorkMode(c.model, c.locType); got != c.want {
			t.Errorf("workMode(%q, %q) = %q, want %q", c.model, c.locType, got, c.want)
		}
	}
}

func TestITJobsEmploymentType(t *testing.T) {
	cases := map[string]string{
		`"FULL_TIME"`:                 "full_time",
		`"CONTRACTOR"`:                "contract",
		`["FULL_TIME"]`:               "full_time",
		`["FULL_TIME", "CONTRACTOR"]`: "",
		`["TEMPORARY", "CONTRACTOR"]`: "contract",
		`"OTHER"`:                     "",
		``:                            "",
	}
	for raw, want := range cases {
		if got := itjobsEmploymentType(json.RawMessage(raw)); got != want {
			t.Errorf("employmentType(%s) = %q, want %q", raw, got, want)
		}
	}
}
