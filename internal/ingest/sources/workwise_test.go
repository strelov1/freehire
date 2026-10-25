package sources

import (
	"context"
	"os"
	"testing"

	"golang.org/x/time/rate"
)

// workwiseFixture reads what www.workwise.io served on 2026-10-25: the sitemap index verbatim,
// the posting sitemap trimmed to four entries, and three posting pages trimmed to their <title>
// and JobPosting script. The closed posting's page carried no JobPosting at all; its fixture
// keeps the notice it showed instead.
func workwiseFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/workwise/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func newTestWorkwise(t *testing.T) (workwise, *routedHTTP) {
	fake := (&routedHTTP{}).
		route("workwise.io/sitemap.xml", workwiseFixture(t, "sitemap.xml")).
		route("/enquiryFocusPagesSitemap.xml", workwiseFixture(t, "focus_sitemap.xml")).
		route("/job/6952-", workwiseFixture(t, "detail_6952.html")).
		route("/job/129779-", workwiseFixture(t, "detail_129779.html")).
		route("/job/125872-", workwiseFixture(t, "detail_remote.html")).
		route("/job/129778-", workwiseFixture(t, "detail_closed.html"))
	s := NewWorkwise(fake).(workwise)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestWorkwiseProvider(t *testing.T) {
	s, ok := All(nil)["workwise"]
	if !ok || s.Provider() != "workwise" {
		t.Fatal("All() must register workwise")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("workwise must be a HydratingSource: the body costs a page request")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("workwise must be boardless: one sitemap names every posting")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("workwise must be an aggregator: the employer is read per posting")
	}
}

func byExternalID(jobs []Job) map[string]Job {
	out := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestWorkwiseHydratesUnseenPostings(t *testing.T) {
	s, _ := newTestWorkwise(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "workwise"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want the 3 open postings (the closed one carries no JobPosting)", len(jobs))
	}
	if _, ok := got["129778"]; ok {
		t.Error("a closed posting must be dropped")
	}

	sec := got["6952"]
	if sec.Company != "aramido GmbH" || sec.Location != "Karlsruhe" || sec.URL != "https://www.workwise.io/job/6952-it-security-consultant-mit-schwerpunkt-it-infrastruktur-m-w-x" {
		t.Errorf("6952 = %q %q %q", sec.Company, sec.Location, sec.URL)
	}
	if sec.WorkMode != "hybrid" || sec.EmploymentType != "full_time" || sec.SalaryMin != nil {
		t.Errorf("6952 work mode/type/salary = %q %q %v, want hybrid full_time none", sec.WorkMode, sec.EmploymentType, sec.SalaryMin)
	}
	if len(sec.Countries) != 1 || sec.Countries[0] != "de" || sec.PostedAt == nil {
		t.Errorf("6952 countries/posted = %v %v", sec.Countries, sec.PostedAt)
	}

	intern := got["129779"]
	if intern.EmploymentType != "internship" {
		t.Errorf("129779 EmploymentType = %q, want internship for FULL_TIME + INTERN", intern.EmploymentType)
	}
	if intern.SalaryMin == nil || *intern.SalaryMin != 1250 || intern.SalaryMax == nil || *intern.SalaryMax != 1650 ||
		intern.SalaryPeriod != "month" || intern.SalaryCurrency != "EUR" {
		t.Errorf("129779 salary = %v-%v %s/%s", intern.SalaryMin, intern.SalaryMax, intern.SalaryCurrency, intern.SalaryPeriod)
	}
	if intern.WorkMode != "" {
		t.Errorf("129779 WorkMode = %q: \"Kein Homeoffice\" must not be read as onsite", intern.WorkMode)
	}

	if remote := got["125872"]; remote.WorkMode != "remote" || len(remote.Countries) != 1 || remote.Countries[0] != "de" {
		t.Errorf("125872 work mode/countries = %q %v, want remote in de (from applicantLocationRequirements)", remote.WorkMode, remote.Countries)
	}
}

func TestWorkwiseRefreshesSeenPostingsWithoutFetchingThem(t *testing.T) {
	s, fake := newTestWorkwise(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "workwise"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 || fake.calls != 2 {
		t.Fatalf("jobs/requests = %d/%d, want 4 refreshes from the two sitemap requests", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("job %s must be a SeenRefresh", j.ExternalID)
		}
	}
}

func TestWorkwiseBoundsHydrationNewestFirst(t *testing.T) {
	s, _ := newTestWorkwise(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "workwise"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "129779" {
		t.Fatalf("jobs = %+v, want only the newest posting, 129779", jobs)
	}
}

func TestWorkwiseSitemapFailureFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).route("workwise.io/sitemap.xml", workwiseFixture(t, "sitemap.xml"))
	s := NewWorkwise(fake).(workwise)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "workwise"}); err == nil {
		t.Fatal("a posting sitemap that cannot be read must fail the crawl")
	}
}

func TestWorkwisePostingID(t *testing.T) {
	for raw, want := range map[string]int64{
		"https://www.workwise.io/job/6952-it-security-consultant": 6952,
		"https://www.workwise.io/job/125872":                      125872,
		"https://www.workwise.io/jobs/it":                         0,
		"https://www.workwise.io/s/28yBpwZ-vermessungstechniker":  0,
	} {
		if got := workwisePostingID(raw); got != want {
			t.Errorf("workwisePostingID(%q) = %d, want %d", raw, got, want)
		}
	}
}
