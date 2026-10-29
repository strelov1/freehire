package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// swissdevjobsFake serves the feed and posting pages from fixtures; an unknown URL answers 404.
type swissdevjobsFake struct {
	t     *testing.T
	pages map[string]string
	fail  bool
	gets  []string
}

func (f *swissdevjobsFake) GetJSON(_ context.Context, u string, v any) error {
	f.gets = append(f.gets, u)
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
	f.gets = append(f.gets, u)
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
