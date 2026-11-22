package sources

import (
	"context"
	"strings"
	"testing"
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
