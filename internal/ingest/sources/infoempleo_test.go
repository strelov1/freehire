package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// infoempleoFake serves the real fixtures (fetched 2026-10-27): listing page 1 trimmed to three
// cards plus the "related" rail the walk must not read, page 2 as the empty page the site
// answers past its end, and one posting page trimmed to its JobPosting. A posting with no page
// fixture answers 404, as a gone posting does.
type infoempleoFake struct {
	listErr error

	mu    sync.Mutex
	pages []string
}

const infoempleoPageID = "3206203"

func (f *infoempleoFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	var path string
	switch {
	case u == infoempleoListURL:
		if f.listErr != nil {
			return nil, f.listErr
		}
		path = "testdata/infoempleo/list.html"
	case strings.HasPrefix(u, infoempleoListURL+"pagina_"):
		path = "testdata/infoempleo/empty.html"
	case strings.HasSuffix(u, "/"+infoempleoPageID+"/"):
		path = "testdata/infoempleo/posting.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *infoempleoFake) postingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.pages {
		if strings.Contains(p, "/ofertasdetrabajo/") {
			n++
		}
	}
	return n
}

func newTestInfoempleo(f *infoempleoFake) infoempleo {
	s := NewInfoempleo(f).(infoempleo)
	s.limiter = noWait{}
	return s
}

func TestInfoempleoHydratesNewPostingsNewestFirst(t *testing.T) {
	f := &infoempleoFake{}
	jobs, err := newTestInfoempleo(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "3205964" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The related rail's posting is not a card; 3206515 answers 404 and is not yielded.
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want the refreshed and the hydrated posting", jobs)
	}
	if _, ok := byID["1111111"]; ok {
		t.Error("read a posting from outside the cards")
	}
	refreshed := byID["3205964"]
	if !refreshed.SeenRefresh || refreshed.Company != "MANPOWER ESPAÑA" || refreshed.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less refresh", refreshed)
	}
	j := byID[infoempleoPageID]
	if j.Title != "MLOps Engineer | Azure Databricks | Python (H/M/X) - 100% Remoto" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Company != "MANPOWER ESPAÑA" {
		t.Errorf("company = %q", j.Company)
	}
	if !strings.Contains(j.Description, "Azure Databricks") {
		t.Errorf("description = %q", j.Description)
	}
	if j.URL != infoempleoBaseURL+"/ofertasdetrabajo/mlops-engineer-azure-databricks-python-hmx-100-remoto/espanna/3206203/" {
		t.Errorf("url = %q", j.URL)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("work mode = %q remote=%v, want remote", j.WorkMode, j.Remote)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "es" {
		t.Errorf("countries = %v", j.Countries)
	}
	if !j.IsTechHint {
		t.Error("a posting filed under Tecnología e informática carries no tech hint")
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	// Newest id first: 3206515 (gone) is asked before 3206203; 3205964 is seen and never asked.
	if got := f.postingRequests(); got != 2 {
		t.Errorf("posting requests = %d, want 2", got)
	}
}

func TestInfoempleoGateSkipsCoveredEmployers(t *testing.T) {
	f := &infoempleoFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c] = c == "MANPOWER ESPAÑA"
		}
		return out
	}}
	jobs, err := newTestInfoempleo(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want all three yielded body-less", len(jobs))
	}
	for _, j := range jobs {
		if j.Description != "" || j.SeenRefresh {
			t.Errorf("covered posting %s was hydrated or marked seen", j.ExternalID)
		}
	}
	if got := f.postingRequests(); got != 0 {
		t.Errorf("posting requests = %d, want none", got)
	}
}

func TestInfoempleoFirstListingPageFailingFailsTheCrawl(t *testing.T) {
	f := &infoempleoFake{listErr: errors.New("boom")}
	if _, err := newTestInfoempleo(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestInfoempleoLeavesUnreadPostingsNearTheDeadline(t *testing.T) {
	f := &infoempleoFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestInfoempleo(f).FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("a run out of time is not a failed crawl: %v", err)
	}
	if len(jobs) != 0 || f.postingRequests() != 0 {
		t.Errorf("jobs = %d, posting requests = %d; want nothing read inside the margin", len(jobs), f.postingRequests())
	}
}

func TestInfoempleoBoundsNewPostingsPerRun(t *testing.T) {
	f := &infoempleoFake{}
	s := newTestInfoempleo(f)
	s.maxNewPerRun = 1
	// The newest unseen posting (3206515) answers 404, which is an answer, not a failure.
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if got := f.postingRequests(); got != 1 {
		t.Errorf("posting requests = %d, want 1", got)
	}
}

func TestInfoempleoCardFacts(t *testing.T) {
	cases := []struct {
		locType, remote, want string
	}{
		{"", "Teletrabajo parcial", "hybrid"},
		{"", "Teletrabajo 100%", "remote"},
		{"TELECOMMUTE", "", "remote"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := infoempleoWorkMode(c.locType, c.remote); got != c.want {
			t.Errorf("workMode(%q, %q) = %q, want %q", c.locType, c.remote, got, c.want)
		}
	}
	types := []struct {
		ld, contract, want string
	}{
		{"FULL_TIME", "Prácticas · Jornada completa", "full_time"},
		{"OTHER", "Prácticas · Jornada completa", "internship"},
		{"OTHER", "Autónomo o freelance · Jornada flexible", "contract"},
		{"OTHER", "De duración determinada · Jornada parcial", "part_time"},
		{"OTHER", "Indefinido", ""},
	}
	for _, c := range types {
		if got := infoempleoEmploymentType(c.ld, c.contract); got != c.want {
			t.Errorf("employmentType(%q, %q) = %q, want %q", c.ld, c.contract, got, c.want)
		}
	}
}
