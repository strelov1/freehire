package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// swissdevjobsFake serves the feed and posting pages from fixtures; an unknown URL answers 404.
type swissdevjobsFake struct {
	t     *testing.T
	pages map[string]string
	fail  bool
	mu    sync.Mutex
	gets  []string
}

func (f *swissdevjobsFake) record(u string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, u)
}

func (f *swissdevjobsFake) GetJSON(_ context.Context, u string, v any) error {
	f.record(u)
	if u != swissdevjobsListURL {
		return &StatusError{Code: 404}
	}
	b, err := os.ReadFile("testdata/swissdevjobs/jobsLight.json")
	if err != nil {
		f.t.Fatal(err)
	}
	return json.Unmarshal(b, v)
}

func (f *swissdevjobsFake) GetText(_ context.Context, u string) (string, error) {
	f.record(u)
	if f.fail {
		return "", &StatusError{Code: 503}
	}
	body, ok := f.pages[u]
	if !ok {
		return "", &StatusError{Code: 404}
	}
	return body, nil
}

const (
	sdjIsolutions = "6a99830638d909e9f7b1ec5a"
	sdjRemote     = "6a96c67b385f9a8c8271196a"
	sdjBachmann   = "6313076be3f58101d34f7316"
)

func newSwissdevjobsFake(t *testing.T) *swissdevjobsFake {
	b, err := os.ReadFile("testdata/swissdevjobs/posting.html")
	if err != nil {
		t.Fatal(err)
	}
	return &swissdevjobsFake{t: t, pages: map[string]string{
		swissdevjobsBase + "/jobs/isolutions-Senior-Software-Engineer-NET--Angular": string(b),
	}}
}

func newTestSwissdevjobs(f *swissdevjobsFake) swissdevjobs {
	return swissdevjobs{get: f, limiter: noWait{}, maxNewPerRun: swissdevjobsMaxNewPerRun, deadlineMargin: swissdevjobsDeadlineMargin}
}

func TestSwissDevJobsHydratesNewRefreshesSeenAndGatesCovered(t *testing.T) {
	f := newSwissdevjobsFake(t)
	gate := DetailGate{Covered: func([]string) map[string]bool { return map[string]bool{"BACHMANN people consulting GmbH": true} }}
	jobs, err := newTestSwissdevjobs(f).FetchNewGated(context.Background(), CompanyEntry{}, func(id string) bool { return id == sdjRemote }, gate)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Job{}
	for _, j := range jobs {
		got[j.ExternalID] = j
	}
	if !got[sdjRemote].SeenRefresh {
		t.Errorf("seen posting = %+v; want a refresh", got[sdjRemote])
	}
	for _, u := range f.gets {
		if strings.Contains(u, "OnlineDoctor") || strings.Contains(u, "BACHMANN") {
			t.Errorf("requested %s; seen and covered postings must not be bought", u)
		}
	}
	if j := got[sdjBachmann]; j.Description != "" || j.Company == "" {
		t.Errorf("covered posting = %+v; want body-less", j)
	}
	j := got[sdjIsolutions]
	if j.Title != "Senior Software Engineer .NET & Angular" || j.Company != "isolutions" || j.Location != "Zürich" {
		t.Errorf("job = %q / %q / %q", j.Title, j.Company, j.Location)
	}
	if j.URL != "https://swissdevjobs.ch/jobs/isolutions-Senior-Software-Engineer-NET--Angular" {
		t.Errorf("url = %q", j.URL)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 120000 || *j.SalaryMax != 160000 || j.SalaryCurrency != "CHF" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s %s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.WorkMode != "hybrid" || j.Seniority != "senior" || j.EmploymentType != "full_time" || !j.IsTechHint {
		t.Errorf("facets = %q %q %q tech=%v", j.WorkMode, j.Seniority, j.EmploymentType, j.IsTechHint)
	}
	if strings.Join(j.FallbackCountries, ",") != "ch" {
		t.Errorf("fallback countries = %v", j.FallbackCountries)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2026-10-28T15:51:39Z" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	for _, want := range []string{"<h3>Responsibilities</h3>", "<h3>Requirements</h3>", "<li>", "Angular"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description lacks %q: %.300q", want, j.Description)
		}
	}
}

func TestSwissDevJobsRemoteFromTheFeed(t *testing.T) {
	f := newSwissdevjobsFake(t)
	rows, err := newTestSwissdevjobs(f).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == sdjRemote {
			j := swissdevjobsBaseJob(r)
			if j.WorkMode != "remote" || !j.Remote || j.Seniority != "middle" {
				t.Errorf("remote posting = %q remote=%v seniority %q", j.WorkMode, j.Remote, j.Seniority)
			}
			return
		}
	}
	t.Fatal("remote posting not listed")
}

func TestSwissDevJobsNoPageAnsweredFailsTheCrawl(t *testing.T) {
	f := newSwissdevjobsFake(t)
	f.fail = true
	if _, err := newTestSwissdevjobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when no posting page answered")
	}
}

func TestSwissDevJobsGonePostingIsNotYielded(t *testing.T) {
	f := newSwissdevjobsFake(t)
	jobs, err := newTestSwissdevjobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID == sdjRemote || j.ExternalID == sdjBachmann {
			t.Errorf("posting %s answered 404 and must not be yielded", j.ExternalID)
		}
	}
}

func TestSwissDevJobsPlaceholderPageIsRetriedUnderTheNextAddress(t *testing.T) {
	f := newSwissdevjobsFake(t)
	const slug = "isolutions-Senior-Software-Engineer-NET--Angular"
	f.pages[swissdevjobsBase+"/de/jobs/"+slug] = f.pages[swissdevjobsBase+"/jobs/"+slug]
	f.pages[swissdevjobsBase+"/jobs/"+slug] = `<html><head><script>window.__detailedJob = "JOB_PLACEHOLDER"</script></head></html>`
	r := swissdevjobsJob{ID: sdjIsolutions, Slug: slug, Name: "x", Company: "isolutions"}
	j, ok, err := newTestSwissdevjobs(f).detail(context.Background(), r)
	if err != nil || !ok || j.Description == "" {
		t.Fatalf("detail = ok %v, err %v; want the posting from /de/jobs/", ok, err)
	}
	if j.URL != swissdevjobsBase+"/jobs/"+slug {
		t.Errorf("url = %q; want the canonical /jobs/ address", j.URL)
	}

	// A localized address that answers 404 after a placeholder is passed over, not read as gone.
	f.pages[swissdevjobsBase+"/fr/jobs/"+slug] = f.pages[swissdevjobsBase+"/de/jobs/"+slug]
	delete(f.pages, swissdevjobsBase+"/de/jobs/"+slug)
	if _, ok, err := newTestSwissdevjobs(f).detail(context.Background(), r); err != nil || !ok {
		t.Fatalf("detail = ok %v, err %v; want the posting from /fr/jobs/ past a 404 on /de/jobs/", ok, err)
	}

	// A placeholder under every address defers the posting rather than reading it as gone.
	for _, p := range swissdevjobsDetailPaths {
		f.pages[swissdevjobsBase+p+slug] = `<script>window.__detailedJob = "JOB_PLACEHOLDER"</script>`
	}
	if _, _, err := newTestSwissdevjobs(f).detail(context.Background(), r); err == nil {
		t.Fatal("want an error when every address serves the placeholder")
	}
}
