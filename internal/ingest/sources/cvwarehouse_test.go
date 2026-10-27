package sources

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// cvwarehouseFixture reads a page captured from jobpage.cvwarehouse.com (2026-10-27), trimmed to
// a few vacancies with scripts and styles removed: kcsit.html is a flat site (KCS IT, Portugal)
// whose page also carries a hidden detail block for an unlisted vacancy (401708); sectioned_* are
// the home page of a sectioned site (De Zilveren Zwaan, Belgium) and its two sections, one empty;
// not_available.html is the platform's answer to an unknown companyGuid.
func cvwarehouseFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/cvwarehouse/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const (
	cvwKCS       = "488b6259-1e95-443a-aefc-c526c05b2d9a"
	cvwSectioned = "0899d23b-6cd3-4e2e-8618-94db8719a1c4"
)

func TestCVWarehouseProvider(t *testing.T) {
	s, ok := All(nil)["cvwarehouse"]
	if !ok || s.Provider() != "cvwarehouse" {
		t.Fatal("All() must register cvwarehouse")
	}
	if !FullBoardListingProviders(All(nil))["cvwarehouse"] {
		t.Error("cvwarehouse reads every page of a site or fails: it must be fullBoardListing")
	}
}

func TestCVWarehouseFlatSite(t *testing.T) {
	fake := (&routedHTTP{}).route("companyGuid="+cvwKCS, cvwarehouseFixture(t, "kcsit.html"))
	jobs, err := NewCVWarehouse(fake).Fetch(context.Background(),
		CompanyEntry{Provider: "cvwarehouse", Board: strings.ToUpper(cvwKCS)})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v, want only the listed vacancy (no open application, no hidden block)", jobs)
	}
	j := jobs[0]
	if j.ExternalID != "400832" || j.Title != ".Net Developer" || j.Company != "KCS IT" {
		t.Errorf("id/title/company = %q/%q/%q", j.ExternalID, j.Title, j.Company)
	}
	if j.Location != "Lisboa" || len(j.Countries) != 1 || j.Countries[0] != "pt" {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if !strings.HasPrefix(j.URL, "https://jobpage.cvwarehouse.com/?companyGuid="+cvwKCS) || !strings.Contains(j.URL, "job=400832") {
		t.Errorf("URL = %q", j.URL)
	}
	if len(j.Description) < 200 || strings.Contains(j.Description, "Submeter candidatura") {
		t.Errorf("Description = %.200q, want the body only", j.Description)
	}
	if fake.calls != 1 {
		t.Errorf("requests = %d, want the one page", fake.calls)
	}
}

func TestCVWarehouseSectionedSite(t *testing.T) {
	fake := (&routedHTTP{}).
		route("section=05c750e4-b056-5260-0d56-9913992e40a0", cvwarehouseFixture(t, "sectioned_05c750e4-b056-5260-0d56-9913992e40a0.html")).
		route("section=dcdb0dba-31c2-cc14-9b97-6b07a0f0f5c8", cvwarehouseFixture(t, "sectioned_dcdb0dba-31c2-cc14-9b97-6b07a0f0f5c8.html")).
		route("companyGuid="+cvwSectioned, cvwarehouseFixture(t, "sectioned_home.html"))
	jobs, err := NewCVWarehouse(fake).Fetch(context.Background(),
		CompanyEntry{Provider: "cvwarehouse", Board: cvwSectioned, Company: "De Zilveren Zwaan"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 3 {
		t.Fatalf("jobs/requests = %d/%d, want the section's 3 vacancies from home + 2 sections", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if j.Company != "De Zilveren Zwaan" || len(j.Countries) != 1 || j.Countries[0] != "be" ||
			j.Location != "Heist-op-den-Berg, Antwerpen" || j.Description == "" {
			t.Errorf("job = %+v", j)
		}
	}
}

func TestCVWarehouseFailures(t *testing.T) {
	bad := (&routedHTTP{}).route("jobpage.cvwarehouse.com", cvwarehouseFixture(t, "not_available.html"))
	if _, err := NewCVWarehouse(bad).Fetch(context.Background(),
		CompanyEntry{Provider: "cvwarehouse", Board: "00000000-1111-2222-3333-444444444444"}); err == nil {
		t.Error("the platform's not-available page must fail, not read as an empty site")
	}
	if _, err := NewCVWarehouse(&routedHTTP{}).Fetch(context.Background(),
		CompanyEntry{Provider: "cvwarehouse", Board: "acme"}); err == nil {
		t.Error("a board that is not a companyGuid must fail")
	}
	sectionDown := (&routedHTTP{}).
		routeErr("section=05c750e4", &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway}).
		route("companyGuid="+cvwSectioned, cvwarehouseFixture(t, "sectioned_home.html"))
	if _, err := NewCVWarehouse(sectionDown).Fetch(context.Background(),
		CompanyEntry{Provider: "cvwarehouse", Board: cvwSectioned}); err == nil {
		t.Error("a section that fails must fail the board: the read must be complete")
	}
}

func TestCVWarehouseEmploymentType(t *testing.T) {
	for _, c := range []struct{ workType, schedules, want string }{
		{"Bediende", `["Voltijds"]`, "full_time"},
		{"Bediende", `["Voltijds","Halftijds"]`, ""},
		{"Outro", `["Tempo parcial"]`, "part_time"},
		{"Freelance", `["Voltijds"]`, "contract"},
		{"", `not json`, ""},
	} {
		if got := cvwarehouseEmploymentType(c.workType, c.schedules); got != c.want {
			t.Errorf("(%q, %q) = %q, want %q", c.workType, c.schedules, got, c.want)
		}
	}
}
