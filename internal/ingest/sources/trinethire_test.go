package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// trinethireFixture reads a page captured verbatim from app.trinethire.com (2026-10-24), with
// the signed S3 logo URLs and the CSRF/Cloudflare tokens redacted.
func trinethireFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/trinethire/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestTrinetHireProvider(t *testing.T) {
	s, ok := All(nil)["trinethire"]
	if !ok || s.Provider() != "trinethire" {
		t.Fatal("All() must register trinethire")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("trinethire must be a HydratingSource: the body costs a page request")
	}
	if !slices.Contains(FilterableProviders(), "trinethire") {
		t.Error("FilterableProviders() should include trinethire")
	}
}

func TestTrinetHireFetchMapsACompany(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/companies/16150-automox-inc/jobs/26497", trinethireFixture(t, "detail_26497.html")).
		route("/companies/16150-automox-inc/jobs", trinethireFixture(t, "list_automox.html"))
	// The listing's other four postings answer the platform's 404, which drops them.
	for _, id := range []string{"26496", "13870", "23908", "12607"} {
		fake.routeErr("/jobs/"+id+"-", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	}

	jobs, err := NewTrinetHire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Automox", Provider: "trinethire", Board: "16150-automox-inc",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want the one posting whose page answered", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "26497" {
		t.Errorf("ExternalID = %q, want the posting's numeric id", j.ExternalID)
	}
	if j.URL != "https://app.trinethire.com/companies/16150-automox-inc/jobs/26497-senior-software-engineer-backend" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Software Engineer, Backend" || j.Company != "Automox" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	// The label "Remote in the Midwest" contributes the remote fact; the address the place.
	if j.Location != "Remote, Chicago, IL" || !j.Remote {
		t.Errorf("Location/Remote = %q/%v, want the address marked remote by the label", j.Location, j.Remote)
	}
	if j.EmploymentType != "full_time" || j.Seniority != "senior" {
		t.Errorf("EmploymentType/Seniority = %q/%q, want full_time/senior", j.EmploymentType, j.Seniority)
	}
	if j.SalaryMin != nil || j.SalaryCurrency != "" {
		t.Errorf("salary = %v %q, want none: the posting states no salary", j.SalaryMin, j.SalaryCurrency)
	}
	if !strings.Contains(j.Description, "Automox") || strings.Contains(j.Description, "Apply for this job") {
		t.Errorf("Description = %.200q, want the posting body without the apply form", j.Description)
	}
}

func TestTrinetHireReadsAnHourlySalary(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/122235-", trinethireFixture(t, "detail_122235.html")).
		route("/companies/31905-adaptation/jobs", trinethireFixture(t, "list_adaptation.html"))
	for _, id := range []string{"125950", "125775", "125630", "122234", "125048", "122333"} {
		fake.routeErr("/jobs/"+id+"-", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	}

	jobs, err := NewTrinetHire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Adaptation", Provider: "trinethire", Board: "31905-adaptation",
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch = %d jobs, %v; want one", len(jobs), err)
	}
	j := jobs[0]
	if j.SalaryMin == nil || *j.SalaryMin != 45 || j.SalaryMax == nil || *j.SalaryMax != 50 ||
		j.SalaryCurrency != "USD" || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %v-%v %q/%q, want 45-50 USD/hour", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	// The label "Adaptation - Union, NJ" is not remote, so the location is the address alone.
	if j.Location != "Union, NJ" || j.Remote {
		t.Errorf("Location/Remote = %q/%v, want Union, NJ / false", j.Location, j.Remote)
	}
}

func TestTrinetHireFetchNewSkipsSeenPostings(t *testing.T) {
	fake := (&routedHTTP{}).route("/companies/16150-automox-inc/jobs", trinethireFixture(t, "list_automox.html"))
	jobs, err := NewTrinetHire(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Automox", Provider: "trinethire", Board: "16150-automox-inc"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want every listed posting", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
	if fake.calls != 1 {
		t.Errorf("requests = %d, want the listing alone", fake.calls)
	}
}

func TestTrinetHireUnreadableDetailIsAMarker(t *testing.T) {
	fake := (&routedHTTP{}).route("/companies/16150-automox-inc/jobs", trinethireFixture(t, "list_automox.html"))
	fake.routeErr("/jobs/", &StatusError{Method: http.MethodGet, Code: http.StatusForbidden})
	jobs, err := NewTrinetHire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Automox", Provider: "trinethire", Board: "16150-automox-inc",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want a marker per refused posting", len(jobs))
	}
	for _, j := range jobs {
		if !j.Unreadable {
			t.Errorf("job %s: Unreadable = false, want a marker for a 403", j.ExternalID)
		}
	}
}

func TestTrinetHireListingFailureFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("/companies/", errors.New("boom"))
	if _, err := NewTrinetHire(fake).Fetch(context.Background(), CompanyEntry{Board: "16150-automox-inc"}); err == nil {
		t.Error("Fetch succeeded, want the listing failure to fail the board")
	}
	if _, err := NewTrinetHire(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "../admin"}); err == nil {
		t.Error("Fetch accepted a board that is not a company segment")
	}
}

func TestTrinetHireLocation(t *testing.T) {
	cases := []struct{ label, address, want string }{
		{"Remote NY", "*, NY", "Remote, NY"},
		{"Remote", "All, All", "Remote"},
		{"Remote", "Remote, Remote", "Remote, Remote"},
		{"Medacta USA Headquarters", "FRANKLIN, Tennessee", "FRANKLIN, Tennessee"},
		{"Remote - GA", "Atlanta, GA", "Remote, Atlanta, GA"},
		{"Credo UES", "", ""},
	}
	for _, c := range cases {
		if got := trinethireLocation(c.label, c.address); got != c.want {
			t.Errorf("trinethireLocation(%q, %q) = %q, want %q", c.label, c.address, got, c.want)
		}
	}
}

func TestTrinetHireSalary(t *testing.T) {
	cases := []struct {
		line         string
		lo, hi       int
		period       string
		wantNoSalary bool
	}{
		{line: "$90,000 - $120,000", lo: 90000, hi: 120000, period: "year"},
		{line: "$165,000", lo: 165000, hi: 165000, period: "year"},
		{line: "$45.00 - $50.00 per hour", lo: 45, hi: 50, period: "hour"},
		{line: "$23 - $25", wantNoSalary: true}, // an hourly figure typed without the hourly pay type
		{line: "$1,200 - $25", wantNoSalary: true},
		{line: "", wantNoSalary: true},
	}
	for _, c := range cases {
		lo, hi, cur, period := trinethireSalary(c.line)
		if c.wantNoSalary {
			if lo != nil || hi != nil || cur != "" || period != "" {
				t.Errorf("trinethireSalary(%q) = %v-%v %q/%q, want none", c.line, lo, hi, cur, period)
			}
			continue
		}
		if lo == nil || *lo != c.lo || hi == nil || *hi != c.hi || cur != "USD" || period != c.period {
			t.Errorf("trinethireSalary(%q) = %v-%v %q/%q, want %d-%d USD/%s", c.line, lo, hi, cur, period, c.lo, c.hi, c.period)
		}
	}
}

func TestTrinetHireBoardIdentityFoldsTheSlug(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Provider: "trinethire", Board: "16150-automox-inc"})
	b, _ := BoardDedupeKey(CompanyEntry{Provider: "trinethire", Board: "16150-automox"})
	c, _ := BoardDedupeKey(CompanyEntry{Provider: "trinethire", Board: "16151-automox-inc"})
	if a != b {
		t.Error("two spellings of one company id must be one crawl target")
	}
	if a == c {
		t.Error("different company ids must stay distinct")
	}
}
