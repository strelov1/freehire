package sources

import (
	"context"
	"slices"
	"testing"
)

// Fixtures are trimmed from live remotejobsfinder.co responses fetched 2026-11-24: the flat
// listings sitemap cut to four entries (a hybrid posting, two remote ones, a category page) and
// two posting pages cut to their JobPosting block.

const (
	rjfRemoteURL = "https://remotejobsfinder.co/en/remote-jobs/usa/accounts-payable-specialist_a3a7e08c-5500-4a7b-acdf-156b2f9c2e65"
	rjfPlainURL  = "https://remotejobsfinder.co/en/remote-jobs/usa/senior-technical-support-specialist_ab716a4f-7ab5-456d-92cc-8b1eacd42d32"
	rjfGoneURL   = "https://remotejobsfinder.co/en/hybrid-jobs/usa/chief-information-officer-cio_e8c07fd6-9ee7-4427-8842-a18f9180605c"
)

func newRJFFake(t *testing.T) *sitemapBoardFake {
	t.Helper()
	return &sitemapBoardFake{t: t, dir: "remotejobsfinder", files: map[string]string{
		remotejobsfinderSitemapURL: "sitemap.xml",
		rjfRemoteURL:               "posting-remote.html",
		rjfPlainURL:                "posting-nolocationtype.html",
	}}
}

func TestRemoteJobsFinderID(t *testing.T) {
	for loc, want := range map[string]string{
		rjfRemoteURL: "a3a7e08c-5500-4a7b-acdf-156b2f9c2e65",
		rjfGoneURL:   "e8c07fd6-9ee7-4427-8842-a18f9180605c",
		"https://remotejobsfinder.co/en/remote-jobs":                                                 "",
		"https://remotejobsfinder.co/de/remote-jobs/usa/x_" + "a3a7e08c-5500-4a7b-acdf-156b2f9c2e65": "",
	} {
		if got := remotejobsfinderID(loc); got != want {
			t.Errorf("remotejobsfinderID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestRemoteJobsFinderMapsPostings(t *testing.T) {
	f := newRJFFake(t)
	s := NewRemoteJobsFinder(f).(remotejobsfinder)
	s.limiter = noWait{}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	// The hybrid posting 404s (a stale sitemap entry) and the category page is not a posting.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	var ap, plain *Job
	for i := range jobs {
		switch jobs[i].URL {
		case rjfRemoteURL:
			ap = &jobs[i]
		case rjfPlainURL:
			plain = &jobs[i]
		}
	}
	if ap == nil || plain == nil {
		t.Fatalf("missing a posting: %+v", jobs)
	}
	if ap.ExternalID != "a3a7e08c-5500-4a7b-acdf-156b2f9c2e65" || ap.Title != "Accounts payable specialist" || ap.Company != "DV Trading" {
		t.Errorf("identity = %q / %q / %q", ap.ExternalID, ap.Title, ap.Company)
	}
	if !ap.Remote || ap.EmploymentType != "full_time" || !slices.Equal(ap.FallbackCountries, []string{"us"}) {
		t.Errorf("remote/type/countries = %v %q %v", ap.Remote, ap.EmploymentType, ap.FallbackCountries)
	}
	if ap.PostedAt == nil || ap.PostedAt.Format("2006-01-02") != "2026-11-19" {
		t.Errorf("posted at = %v", ap.PostedAt)
	}
	if ap.SalaryMin == nil || ap.SalaryMax == nil || ap.SalaryCurrency != "USD" {
		t.Errorf("salary = %v-%v %s", ap.SalaryMin, ap.SalaryMax, ap.SalaryCurrency)
	}
	if ap.Description == "" {
		t.Error("no description")
	}
	if plain.Remote {
		t.Error("a posting without jobLocationType TELECOMMUTE was marked remote")
	}
	if f.fetched("https://remotejobsfinder.co/en/remote-jobs") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
}

func TestRemoteJobsFinderIsABoardlessAggregator(t *testing.T) {
	s := NewRemoteJobsFinder(newRJFFake(t))
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("not boardless")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("not an aggregator")
	}
}
