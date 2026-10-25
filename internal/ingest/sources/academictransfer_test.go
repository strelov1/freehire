package sources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

func newTestAcademicTransfer(t *testing.T) (academicTransfer, *routedHTTP) {
	fake := (&routedHTTP{}).
		route("/sitemap-vacancies.xml", nlFixture(t, "academictransfer/sitemap.xml")).
		route("/jobs/360681/", nlFixture(t, "academictransfer/detail_360681.html")).
		route("/jobs/360740/", nlFixture(t, "academictransfer/detail_360740.html")).
		routeErr("/jobs/350000/", errors.New("unexpected status 404"))
	s := NewAcademicTransfer(fake).(academicTransfer)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestAcademicTransferProvider(t *testing.T) {
	s, ok := All(nil)["academictransfer"]
	if !ok || s.Provider() != "academictransfer" {
		t.Fatal("All() must register academictransfer")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("academictransfer must be a HydratingSource: the body costs a page request")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("academictransfer must be boardless: one sitemap names every posting")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("academictransfer must be an aggregator: the institution is read per posting")
	}
}

func TestAcademicTransferHydratesUnseenPostings(t *testing.T) {
	s, _ := newTestAcademicTransfer(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "academictransfer"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want the 2 open postings (the closed one is a 404)", len(jobs))
	}
	phd := got["360681"]
	if phd.Title != "PhD Position in Cloud-Native and AI-Native 6G Networks" || phd.Company != "University of Amsterdam (UvA)" || phd.Location != "Amsterdam" {
		t.Errorf("360681 = %q %q %q", phd.Title, phd.Company, phd.Location)
	}
	if phd.EmploymentType != "" {
		t.Errorf("360681 EmploymentType = %q: a PhD's TEMPORARY is a fixed-term appointment, not contract work", phd.EmploymentType)
	}
	if phd.SalaryMin == nil || *phd.SalaryMin != 3059 || phd.SalaryMax == nil || *phd.SalaryMax != 3881 || phd.SalaryPeriod != "month" {
		t.Errorf("360681 salary = %v-%v/%s", phd.SalaryMin, phd.SalaryMax, phd.SalaryPeriod)
	}
	if len(phd.Countries) != 1 || phd.Countries[0] != "nl" || phd.PostedAt == nil || !strings.Contains(phd.Description, "Ericsson") {
		t.Errorf("360681 countries/posted/description = %v %v %d chars", phd.Countries, phd.PostedAt, len(phd.Description))
	}
	if nurse := got["360740"]; nurse.EmploymentType != "full_time" || nurse.Company != "Radboudumc" {
		t.Errorf("360740 type/company = %q %q, want full_time Radboudumc", nurse.EmploymentType, nurse.Company)
	}
}

func TestAcademicTransferBoundsHydrationNewestFirst(t *testing.T) {
	s, _ := newTestAcademicTransfer(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "academictransfer"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "360740" {
		t.Fatalf("jobs = %+v, want only the newest posting, 360740", jobs)
	}
}

func TestAcademicTransferSitemapFailureFailsTheCrawl(t *testing.T) {
	s := NewAcademicTransfer(&routedHTTP{}).(academicTransfer)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "academictransfer"}); err == nil {
		t.Fatal("a sitemap that cannot be read must fail the crawl")
	}
}
