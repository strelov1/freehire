package sources

import (
	"context"
	"slices"
	"testing"
)

// The fixtures are trimmed from live remote100k.com responses fetched 2026-11-24: the sitemap cut
// to a few entries (two postings, one the site has taken down, three that are not postings) and
// posting pages cut to their JobPosting block (description shortened) and the apply anchor.

const (
	remote100kHumanaURL = "https://remote100k.com/remote-job/humana-medical-director-national-op-ma"
	remote100kCAURL     = "https://remote100k.com/remote-job/instacart-senior-product-manager-retailer-platform"
	remote100kGoneURL   = "https://remote100k.com/remote-job/apollo-account-manager-mid-market"
)

func newRemote100kFake(t *testing.T) *sitemapBoardFake {
	return &sitemapBoardFake{t: t, dir: "remote100k", files: map[string]string{
		remote100kSitemapURL: "sitemap.xml",
		remote100kHumanaURL:  "posting-humana.html",
		remote100kCAURL:      "posting-ca.html",
	}}
}

func fastRemote100k(f *sitemapBoardFake) remote100k {
	s := NewRemote100k(f).(remote100k)
	s.limiter = noWait{}
	return s
}

func TestRemote100kID(t *testing.T) {
	for loc, want := range map[string]string{
		remote100kHumanaURL:                           "humana-medical-director-national-op-ma",
		"https://remote100k.com/remote-jobs/anywhere": "",
		"https://remote100k.com/remote-companies/new": "",
		"https://remote100k.com/remote-job/":          "",
		remote100kHumanaURL + "?x=1":                  "",
	} {
		if got := remote100kID(loc); got != want {
			t.Errorf("remote100kID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestRemote100kMapsPostingsAndDropsAGonePage(t *testing.T) {
	jobs, err := fastRemote100k(newRemote100kFake(t)).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the gone page yields nothing): %+v", len(jobs), jobs)
	}
	var h, c *Job
	for i := range jobs {
		switch jobs[i].URL {
		case remote100kHumanaURL:
			h = &jobs[i]
		case remote100kCAURL:
			c = &jobs[i]
		}
	}
	if h == nil || c == nil {
		t.Fatalf("missing a posting: %+v", jobs)
	}
	if h.ExternalID != "humana-medical-director-national-op-ma" || h.Title != "Medical Director, National OP - MA" || h.Company != "Humana" {
		t.Errorf("identity = %q / %q / %q", h.ExternalID, h.Title, h.Company)
	}
	if !h.Remote || h.WorkMode != "remote" || h.EmploymentType != "full_time" || h.Description == "" {
		t.Errorf("remote/mode/type/body = %v %q %q %d", h.Remote, h.WorkMode, h.EmploymentType, len(h.Description))
	}
	if !slices.Equal(h.Countries, []string{"us"}) || !slices.Equal(c.Countries, []string{"ca"}) {
		t.Errorf("countries = %v / %v", h.Countries, c.Countries)
	}
	if h.PostedAt == nil || h.PostedAt.Format("2006-01-02") != "2026-11-24" {
		t.Errorf("posted at = %v", h.PostedAt)
	}
	if h.SalaryMin == nil || *h.SalaryMin != 223800 || h.SalaryMax == nil || *h.SalaryMax != 313100 || h.SalaryCurrency != "USD" {
		t.Errorf("salary = %v-%v %s", h.SalaryMin, h.SalaryMax, h.SalaryCurrency)
	}
	if h.ApplyMethod != ApplyExternal || h.ExternalApplyURL == "" {
		t.Errorf("apply = %q %q", h.ApplyMethod, h.ExternalApplyURL)
	}
	if f := newRemote100kFake(t); f.fetched("https://remote100k.com/remote-jobs/anywhere") {
		t.Error("a category page was fetched")
	}
}

func TestRemote100kRefreshesStoredPostingsAndBoundsNewPages(t *testing.T) {
	f := newRemote100kFake(t)
	s := fastRemote100k(f)
	s.maxNew = 1
	seen := func(id string) bool { return id == remote100kID(remote100kHumanaURL) }
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if f.fetched(remote100kHumanaURL) {
		t.Error("a stored posting's page was requested")
	}
	pages := 0
	for _, u := range f.got {
		if remote100kID(u) != "" {
			pages++
		}
	}
	if pages != 1 {
		t.Errorf("fetched %d posting pages, want the bound of 1", pages)
	}
	if len(jobs) == 0 || !jobs[0].SeenRefresh {
		t.Errorf("stored posting not refreshed: %+v", jobs)
	}
}

func TestRemote100kIsABoardlessAggregator(t *testing.T) {
	s := NewRemote100k(newRemote100kFake(t))
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("not boardless")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("not an aggregator")
	}
}
