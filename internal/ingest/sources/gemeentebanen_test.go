package sources

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

func newTestGemeentebanen(t *testing.T) (gemeentebanen, *routedHTTP) {
	fake := (&routedHTTP{}).
		route("/sitemap-vacancies.xml", nlFixture(t, "gemeentebanen/sitemap.xml")).
		route("/vacatures/84133/", nlFixture(t, "gemeentebanen/detail_84133.html")).
		route("/vacatures/84135/", nlFixture(t, "gemeentebanen/detail_84135.html")).
		routeRedirect("/vacatures/80001/", "https://www.gemeentebanen.nl/vacatures/")
	s := NewGemeentebanen(fake).(gemeentebanen)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestGemeentebanenProvider(t *testing.T) {
	s, ok := All(nil)["gemeentebanen"]
	if !ok || s.Provider() != "gemeentebanen" {
		t.Fatal("All() must register gemeentebanen")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("gemeentebanen must be a HydratingSource: the body costs a page request")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("gemeentebanen must be boardless: one sitemap names every posting")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("gemeentebanen must be an aggregator: the municipality is read per posting")
	}
}

func TestGemeentebanenHydratesUnseenPostings(t *testing.T) {
	s, _ := newTestGemeentebanen(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "gemeentebanen"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want the 2 open postings (the closed one redirects, /vacatures is the listing)", len(jobs))
	}
	sec := got["84135"]
	if sec.Title != "Directiesecretaresse (32 uur)" || sec.Company != "Gemeente Maashorst" || sec.Location != "Uden" ||
		sec.URL != "https://www.gemeentebanen.nl/vacatures/84135/gemeente-maashorst/directiesecretaresse-32-uur" {
		t.Errorf("84135 = %q %q %q %q", sec.Title, sec.Company, sec.Location, sec.URL)
	}
	if sec.EmploymentType != "" {
		t.Errorf("84135 EmploymentType = %q: FULL_TIME is the form's default, even at 32 hours", sec.EmploymentType)
	}
	if sec.SalaryMin == nil || *sec.SalaryMin != 2875 || sec.SalaryMax == nil || *sec.SalaryMax != 4065 || sec.SalaryPeriod != "month" {
		t.Errorf("84135 salary = %v-%v/%s", sec.SalaryMin, sec.SalaryMax, sec.SalaryPeriod)
	}
	if len(sec.Countries) != 1 || sec.Countries[0] != "nl" || sec.PostedAt == nil || !strings.Contains(sec.Description, "Maashorst") {
		t.Errorf("84135 countries/posted/description = %v %v %d chars", sec.Countries, sec.PostedAt, len(sec.Description))
	}
	if got["84133"].Location != "Volendam" {
		t.Errorf("84133 location = %q", got["84133"].Location)
	}
}

func TestGemeentebanenBoundsHydrationNewestFirst(t *testing.T) {
	s, _ := newTestGemeentebanen(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "gemeentebanen"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "84135" {
		t.Fatalf("jobs = %+v, want only the newest posting, 84135", jobs)
	}
}

func TestGemeentebanenRefreshesSeenPostingsWithoutFetchingThem(t *testing.T) {
	s, fake := newTestGemeentebanen(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "gemeentebanen"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want 3 refreshes from the one sitemap request", len(jobs), fake.calls)
	}
}

func TestGemeentebanenSitemapFailureFailsTheCrawl(t *testing.T) {
	s := NewGemeentebanen(&routedHTTP{}).(gemeentebanen)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "gemeentebanen"}); err == nil {
		t.Fatal("a sitemap that cannot be read must fail the crawl")
	}
}
