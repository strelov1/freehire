package sources

import (
	"context"
	"strings"
	"testing"
)

// The fixtures are trimmed from live get-in-it.de responses fetched 2026-11-03: the sitemap cut to
// a few entries (one posting, one the site has since taken down, and three that are not postings)
// and the posting page's JobPosting block verbatim.

const (
	getinitLiveURL = "https://www.get-in-it.de/jobsuche/p312652"
	getinitGoneURL = "https://www.get-in-it.de/jobsuche/p759"
)

func fastGetInIT(f *sitemapBoardFake) getinit {
	s := NewGetInIT(f).(getinit)
	s.limiter = noWait{}
	return s
}

func newGetInITFake(t *testing.T) *sitemapBoardFake {
	return &sitemapBoardFake{t: t, dir: "getinit", files: map[string]string{
		getinitSitemapURL: "sitemap.xml",
		getinitLiveURL:    "posting.html",
	}}
}

func TestGetInITID(t *testing.T) {
	for loc, want := range map[string]string{
		getinitLiveURL:       "312652",
		getinitLiveURL + "/": "312652",
		"https://www.get-in-it.de/jobsuche/feeds/x":     "",
		"https://www.get-in-it.de/jobsuche":             "",
		"https://www.get-in-it.de/it-jobs-bremen":       "",
		"https://www.get-in-it.de/jobsuche/p312652?x=1": "",
	} {
		if got := getinitID(loc); got != want {
			t.Errorf("getinitID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestGetInITHydratesAPostingAndDropsAGonePage(t *testing.T) {
	f := newGetInITFake(t)
	jobs, err := fastGetInIT(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the gone page yields nothing): %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "312652" || j.URL != getinitLiveURL {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Title != "Senior SAP TM Berater (w/m/d)" || j.Company != "Capgemini" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	for _, city := range []string{"Berlin", "Hamburg", "München"} {
		if !strings.Contains(j.Location, city) {
			t.Errorf("location %q lacks %s", j.Location, city)
		}
	}
	if len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("countries = %v", j.Countries)
	}
	if strings.Contains(j.Description, "&uuml;") || !strings.Contains(j.Description, "ü") {
		t.Errorf("description entities not decoded: %.120q", j.Description)
	}
	if f.fetched("https://www.get-in-it.de/jobsuche/feeds/x") || f.fetched("https://www.get-in-it.de/magazin/artikel") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
}

func TestGetInITRefreshesAStoredPostingWithoutFetchingIt(t *testing.T) {
	f := newGetInITFake(t)
	jobs, err := fastGetInIT(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "312652" })
	if err != nil {
		t.Fatal(err)
	}
	var refreshed bool
	for _, j := range jobs {
		if j.ExternalID == "312652" {
			refreshed = j.SeenRefresh
		}
	}
	if !refreshed {
		t.Errorf("stored posting not refreshed by identity: %+v", jobs)
	}
	if f.fetched(getinitLiveURL) {
		t.Error("a stored posting's page was fetched")
	}
}

func TestGetInITFailsWhenTheSitemapNamesNoPosting(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "getinit", files: map[string]string{}}
	if _, err := fastGetInIT(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable sitemap must fail the run, or the sweep closes every posting")
	}
}

func TestGetInITBoundsNewPagesPerRun(t *testing.T) {
	f := newGetInITFake(t)
	s := fastGetInIT(f)
	s.maxNew = 1
	// Which page the bound picks is random, and the gone one picked alone fails the run; the
	// request count is what this test is about.
	_, _ = s.Fetch(context.Background(), CompanyEntry{})
	pages := 0
	for _, g := range f.got {
		if strings.Contains(g, "/jobsuche/p") {
			pages++
		}
	}
	if pages != 1 {
		t.Errorf("fetched %d posting pages, want the bound of 1", pages)
	}
}
