package sources

import (
	"context"
	"strings"
	"testing"
)

// The fixtures are trimmed from live berlinstartupjobs.com responses fetched 2026-11-03: the
// posting sitemap cut to a few entries (the home page, a posting, a posting whose page is gone and
// a page that is not a posting) and the posting page's JobPosting block verbatim.

const (
	bsjLiveURL = "https://berlinstartupjobs.com/engineering/tech-lead-m-f-d-superchat/"
	bsjGoneURL = "https://berlinstartupjobs.com/internships/some-intern-role/"
)

func fastBSJ(f *sitemapBoardFake) berlinstartupjobs {
	s := NewBerlinStartupJobs(f).(berlinstartupjobs)
	s.limiter = noWait{}
	return s
}

func newBSJFake(t *testing.T) *sitemapBoardFake {
	return &sitemapBoardFake{t: t, dir: "berlinstartupjobs", files: map[string]string{
		berlinstartupjobsSitemapURL: "sitemap.xml",
		bsjLiveURL:                  "posting.html",
	}}
}

func TestBerlinStartupJobsID(t *testing.T) {
	for loc, want := range map[string]string{
		bsjLiveURL: "engineering/tech-lead-m-f-d-superchat",
		"https://berlinstartupjobs.com/engineering/tech-lead-m-f-d-superchat": "engineering/tech-lead-m-f-d-superchat",
		"https://berlinstartupjobs.com/":                                      "",
		"https://berlinstartupjobs.com/about/":                                "",
		"https://berlinstartupjobs.com/a/b/c/":                                "",
		"https://elsewhere.example/engineering/x/":                            "",
	} {
		if got := berlinstartupjobsID(loc); got != want {
			t.Errorf("berlinstartupjobsID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestBerlinStartupJobsHydratesAPostingAndDropsAGonePage(t *testing.T) {
	f := newBSJFake(t)
	jobs, err := fastBSJ(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "engineering/tech-lead-m-f-d-superchat" || j.URL != bsjLiveURL {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Company != "Superchat" || !strings.HasPrefix(j.Title, "Tech Lead") {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-02" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Superchat") {
		t.Errorf("description = %.80q", j.Description)
	}
	if !f.fetched(bsjGoneURL) {
		t.Error("the posting the sitemap names was never tried")
	}
	if f.fetched("https://berlinstartupjobs.com/about/") || f.fetched("https://berlinstartupjobs.com/") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
}

func TestBerlinStartupJobsFailsWhenTheSitemapIsUnreadable(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "berlinstartupjobs", files: map[string]string{}}
	if _, err := fastBSJ(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable sitemap must fail the run, or the sweep closes every posting")
	}
}
