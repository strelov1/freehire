package sources

import (
	"context"
	"slices"
	"testing"
)

// The fixtures are trimmed from live jobgether.com responses fetched 2026-11-22: the offers
// sitemap index and two shards cut to a few entries (one shard repeats a posting, one entry is not
// a posting) and posting pages cut to their JobPosting block and the body's data-offer attribute.

const (
	jobgetherUSURL     = "https://jobgether.com/offer/6b02a8451870ab7f4cb42437-servicenow-developer"
	jobgetherGoneURL   = "https://jobgether.com/offer/6ac46bb38a27695aea2f0e8f-director-partnership-operations"
	jobgetherBRURL     = "https://jobgether.com/offer/6abefb618a27695aea18f543-engenheiro-dados"
	jobgetherLapsedURL = "https://jobgether.com/offer/6aaaaaaa8a27695aea000001-lapsed-posting"
)

func newJobgetherFake(t *testing.T) *sitemapBoardFake {
	return &sitemapBoardFake{t: t, dir: "jobgether", files: map[string]string{
		jobgetherSitemapURL:                          "index-offers.xml",
		"https://jobgether.com/sitemap/offers-1.xml": "offers-1.xml",
		"https://jobgether.com/sitemap/offers-2.xml": "offers-2.xml",
		jobgetherUSURL:                               "posting-us.html",
		jobgetherBRURL:                               "posting-br.html",
		jobgetherLapsedURL:                           "posting-lapsed.html",
	}}
}

func fastJobgether(f *sitemapBoardFake) jobgether {
	s := NewJobgether(f).(jobgether)
	s.limiter = noWait{}
	return s
}

func TestJobgetherID(t *testing.T) {
	for loc, want := range map[string]string{
		jobgetherUSURL: "6b02a8451870ab7f4cb42437",
		"https://jobgether.com/remote-jobs/united-states": "",
		"https://jobgether.com/offer/{id}-slug":           "",
		jobgetherUSURL + "?utm=x":                         "",
		"https://jobgether.com/offer/short-slug":          "",
	} {
		if got := jobgetherID(loc); got != want {
			t.Errorf("jobgetherID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestJobgetherFetchReadsEveryShardAndSkipsWhatIsNotAPosting(t *testing.T) {
	f := newJobgetherFake(t)
	jobs, err := fastJobgether(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	// The US and BR postings; the gone page and the lapsed one yield nothing.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	if f.fetched("https://jobgether.com/remote-jobs/united-states") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
	n := 0
	for _, u := range f.got {
		if u == jobgetherGoneURL {
			n++
		}
	}
	if n != 1 {
		t.Errorf("a posting named by two shards was fetched %d times, want once", n)
	}
}

func TestJobgetherMapsAPosting(t *testing.T) {
	jobs, err := fastJobgether(newJobgetherFake(t)).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	var us, br *Job
	for i := range jobs {
		switch jobs[i].ExternalID {
		case "6b02a8451870ab7f4cb42437":
			us = &jobs[i]
		case "6abefb618a27695aea18f543":
			br = &jobs[i]
		}
	}
	if us == nil || br == nil {
		t.Fatalf("missing a posting: %+v", jobs)
	}
	if us.Title != "ServiceNow Developer" || us.Company != "General Dynamics Information Technology" || us.URL != jobgetherUSURL {
		t.Errorf("identity = %q / %q / %q", us.Title, us.Company, us.URL)
	}
	if !us.Remote || us.WorkMode != "remote" || us.EmploymentType != "full_time" {
		t.Errorf("remote/workmode/type = %v %q %q", us.Remote, us.WorkMode, us.EmploymentType)
	}
	if !slices.Equal(us.Countries, []string{"us"}) {
		t.Errorf("countries = %v", us.Countries)
	}
	if us.PostedAt == nil || us.PostedAt.Format("2006-01-02") != "2026-11-22" {
		t.Errorf("posted at = %v", us.PostedAt)
	}
	if us.SalaryMin == nil || *us.SalaryMin != 97750 || us.SalaryMax == nil || *us.SalaryMax != 132250 || us.SalaryCurrency != "USD" {
		t.Errorf("salary = %v-%v %s", us.SalaryMin, us.SalaryMax, us.SalaryCurrency)
	}
	if us.Description == "" {
		t.Error("no description")
	}
	wantApply := "https://gdit.wd5.myworkdayjobs.com/External_Career_Site/job/Any-Location--Remote/ServiceNow-Developer_RQ232563-1?source=jobgether&utm_campaign=search&utm_medium=website"
	if us.ApplyMethod != ApplyExternal || us.ExternalApplyURL != wantApply {
		t.Errorf("apply = %q %q", us.ApplyMethod, us.ExternalApplyURL)
	}
	if !slices.Equal(br.Countries, []string{"br", "pt"}) || br.EmploymentType != "contract" {
		t.Errorf("br countries/type = %v %q", br.Countries, br.EmploymentType)
	}
	// An easy-apply posting is applied for on Jobgether: there is no employer destination.
	if br.ApplyMethod != "" || br.ExternalApplyURL != "" {
		t.Errorf("easy-apply posting recorded an external destination: %q %q", br.ApplyMethod, br.ExternalApplyURL)
	}
}

func TestJobgetherRefreshesStoredPostingsWithoutAPageRequest(t *testing.T) {
	f := newJobgetherFake(t)
	seen := func(id string) bool { return id == "6b02a8451870ab7f4cb42437" }
	jobs, err := fastJobgether(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if f.fetched(jobgetherUSURL) {
		t.Error("a stored posting's page was requested")
	}
	var refreshed bool
	for _, j := range jobs {
		if j.ExternalID == "6b02a8451870ab7f4cb42437" {
			refreshed = j.SeenRefresh
		}
	}
	if !refreshed {
		t.Errorf("stored posting not yielded as a liveness refresh: %+v", jobs)
	}
}

func TestJobgetherBoundsNewPagesPerRun(t *testing.T) {
	f := newJobgetherFake(t)
	s := fastJobgether(f)
	s.maxNew = 1
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, u := range f.got {
		if jobgetherID(u) != "" {
			pages++
		}
	}
	if pages != 1 {
		t.Errorf("fetched %d posting pages, want the bound of 1", pages)
	}
}

func TestJobgetherFailsWhenNoShardReadable(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "jobgether", files: map[string]string{jobgetherSitemapURL: "index-offers.xml"}}
	if _, err := fastJobgether(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable shard must fail the run, not yield a shorter catalogue")
	}
}

func TestJobgetherIsABoardlessAggregator(t *testing.T) {
	var s Source = NewJobgether(newJobgetherFake(t))
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("not boardless")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("not an aggregator")
	}
}
