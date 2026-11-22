package sources

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The fixtures are trimmed from live ictjobs.ch responses fetched 2026-11-22: the posting
// sitemap cut to a few entries (the home page, a posting, a posting whose page is gone and a
// deeper path that is not a posting) and a posting page's JobPosting block verbatim.

const (
	ictjobsLiveURL = "https://ictjobs.ch/it-security-cyber-security/security-engineer-a-80-100/"
	ictjobsGoneURL = "https://ictjobs.ch/software-entwicklung/gone-role/"
)

func fastICTJobs(f *sitemapBoardFake) ictjobs {
	s := NewICTJobs(f).(ictjobs)
	s.limiter = noWait{}
	return s
}

func TestICTJobsID(t *testing.T) {
	for loc, want := range map[string]string{
		ictjobsLiveURL: "it-security-cyber-security/security-engineer-a-80-100",
		"https://ictjobs.ch/it-security-cyber-security/security-engineer-a-80-100": "it-security-cyber-security/security-engineer-a-80-100",
		"https://ictjobs.ch/":                            "",
		"https://ictjobs.ch/a/b/c/":                      "",
		"https://www.ictjobs.ch/software-entwicklung/x/": "",
	} {
		if got := ictjobsID(loc); got != want {
			t.Errorf("ictjobsID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestICTJobsHydratesAPostingAndDropsAGonePage(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "ictjobs", files: map[string]string{
		ictjobsSitemapURL: "sitemap.xml",
		ictjobsLiveURL:    "posting.html",
	}}
	jobs, err := fastICTJobs(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "it-security-cyber-security/security-engineer-a-80-100" || j.URL != ictjobsLiveURL {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Company != "Helsana Versicherungen AG" || !strings.HasPrefix(j.Title, "Security Engineer") {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if !j.IsTechHint {
		t.Error("an IT-only board's posting must carry the technical hint")
	}
	if j.Remote || j.WorkMode == "remote" {
		t.Errorf("the TELECOMMUTE flag must not make a posting remote: remote=%v mode=%q", j.Remote, j.WorkMode)
	}
	if j.Location != "Dübendorf-Stettbach" || len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("location = %q %v", j.Location, j.Countries)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 114300 || j.SalaryMax == nil || *j.SalaryMax != 139700 || j.SalaryCurrency != "CHF" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-20" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if want := time.Date(2026, time.December, 19, 0, 0, 0, 0, time.UTC); j.ApplicationDeadline == nil || !j.ApplicationDeadline.Equal(want) {
		t.Errorf("application deadline = %v, want %v", j.ApplicationDeadline, want)
	}
	if !strings.Contains(j.Description, "Fortinet") {
		t.Errorf("description = %.80q", j.Description)
	}
	if !f.fetched(ictjobsGoneURL) {
		t.Error("the posting the sitemap names was never tried")
	}
	if f.fetched("https://ictjobs.ch/") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
}

func TestICTJobsFailsWhenTheSitemapIsUnreadable(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "ictjobs", files: map[string]string{}}
	if _, err := fastICTJobs(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable sitemap must fail the run, or the sweep closes every posting")
	}
}

func TestICTJobsStoredPostingDoesNotHideFreshDetailFailure(t *testing.T) {
	f := &sitemapBoardFake{
		t: t, dir: "ictjobs", files: map[string]string{ictjobsSitemapURL: "sitemap.xml"},
		missingDetailStatus: http.StatusServiceUnavailable,
	}
	seen := func(id string) bool { return id == "it-security-cyber-security/security-engineer-a-80-100" }
	jobs, err := fastICTJobs(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err == nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs and %v, want a failed crawl when the only new detail is unreadable", len(jobs), err)
	}
	if !f.fetched(ictjobsGoneURL) || f.fetched(ictjobsLiveURL) {
		t.Fatalf("requested %v, want a detail request only for the unseen posting", f.got)
	}
}

func TestICTJobsPreservesUnreadablePostingIdentityOnPartialFailure(t *testing.T) {
	f := &sitemapBoardFake{
		t: t, dir: "ictjobs", files: map[string]string{
			ictjobsSitemapURL: "sitemap.xml",
			ictjobsLiveURL:    "posting.html",
		},
		missingDetailStatus: http.StatusServiceUnavailable,
	}
	jobs, err := fastICTJobs(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("got %d jobs and %v, want the readable posting and an unreadable marker", len(jobs), err)
	}
	byID := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	live := byID["it-security-cyber-security/security-engineer-a-80-100"]
	if live.URL != ictjobsLiveURL || live.Unreadable || !strings.Contains(live.Description, "Fortinet") {
		t.Errorf("unexpected readable posting: %+v", live)
	}
	unreadable := byID["software-entwicklung/gone-role"]
	if unreadable.URL != ictjobsGoneURL || !unreadable.Unreadable || unreadable.SeenRefresh {
		t.Errorf("unexpected unreadable marker: %+v", unreadable)
	}
}
