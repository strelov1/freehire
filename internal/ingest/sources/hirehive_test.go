package sources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestHireHiveProvider(t *testing.T) {
	if got := NewHireHive(nil).Provider(); got != "hirehive" {
		t.Errorf("Provider() = %q, want hirehive", got)
	}
}

func TestHireHiveRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["hirehive"] {
		t.Error("FullBoardListingProviders(All(nil)) should include hirehive")
	}
}

func TestHireHiveFetchListsPostings(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1&page_size=100", `{
			"meta": {"total_items": 2, "has_next_page": true},
			"items": [{
				"id": "job_iZEZFh",
				"title": "Data Warehouse Engineer",
				"hosted_url": "https://amcsgroup.hirehive.com/data-warehouse-engineer",
				"location": "Cebu City",
				"state_code": null,
				"published_date": "2026-10-14T15:12:09.627Z",
				"description": {"html": "<p>Build the warehouse.</p>"},
				"type": {"type": "FullTime", "name": "Full Time"},
				"experience": {"type": "MidLevel", "name": "Mid Level"},
				"country": {"name": "Philippines", "code": "PH"},
				"compensation_tiers": [{
					"interval": "none", "currency_code": null, "min_value": null, "max_value": null
				}]
			}]
		}`).
		route("page=2&page_size=100", `{
			"meta": {"total_items": 2, "has_next_page": false},
			"items": [{
				"id": "job_7Ep4QB",
				"title": "Sales Executive",
				"hosted_url": "https://amcsgroup.hirehive.com/sales-executive",
				"location": "Remote",
				"published_date": "2026-10-30T18:09:36.457Z",
				"description": {"text": "Sell the platform."},
				"type": {"type": "Contract", "name": "Contract"},
				"experience": {"type": "NA", "name": "NA"},
				"country": {"name": "United States", "code": "US"},
				"compensation_tiers": [{
					"type": "range_salary",
					"interval": "year",
					"currency_code": "USD",
					"min_value": 100000,
					"max_value": 125000
				}]
			}]
		}`)

	jobs, err := NewHireHive(fake).Fetch(context.Background(), CompanyEntry{
		Company: "AMCS", Provider: "hirehive", Board: "AMCSGroup",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	eng := jobs[0]
	if eng.ExternalID != "job_iZEZFh" || eng.Company != "AMCS" || eng.EmploymentType != "full_time" || eng.Seniority != "middle" {
		t.Errorf("engineer = %+v", eng)
	}
	if eng.Location != "Cebu City, Philippines" || len(eng.Countries) != 1 || eng.Countries[0] != "ph" {
		t.Errorf("engineer place = %q %v", eng.Location, eng.Countries)
	}
	if !strings.Contains(eng.Description, "Build the warehouse.") {
		t.Errorf("Description = %q", eng.Description)
	}
	if eng.PostedAt == nil || eng.SalaryMin != nil {
		t.Errorf("engineer posted=%v salary=%v", eng.PostedAt, eng.SalaryMin)
	}
	sales := jobs[1]
	if sales.EmploymentType != "contract" || sales.Seniority != "" {
		t.Errorf("sales facets = %q %q", sales.EmploymentType, sales.Seniority)
	}
	if sales.SalaryMin == nil || *sales.SalaryMin != 100000 || sales.SalaryMax == nil || *sales.SalaryMax != 125000 || sales.SalaryCurrency != "USD" || sales.SalaryPeriod != "year" {
		t.Errorf("salary = %v %v %s %s", sales.SalaryMin, sales.SalaryMax, sales.SalaryCurrency, sales.SalaryPeriod)
	}
	if sales.Countries[0] != "us" {
		t.Errorf("countries = %v", sales.Countries)
	}
}

func TestHireHiveFetchEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1&page_size=100", `{
		"meta": {"total_items": 0, "has_next_page": false},
		"items": []
	}`)
	jobs, err := NewHireHive(fake).Fetch(context.Background(), CompanyEntry{Board: "emptyco"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%v err=%v, want an empty board", jobs, err)
	}
}

func TestHireHiveFetchGoneBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("hirehive.com", &StatusError{Code: http.StatusNotFound})
	_, err := NewHireHive(fake).Fetch(context.Background(), CompanyEntry{Board: "missing"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestHireHiveFetchRejectsAShortRead(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1&page_size=100", `{
		"meta": {"total_items": 2, "has_next_page": false},
		"items": [{
			"id": "job_one", "title": "Engineer",
			"hosted_url": "https://acme.hirehive.com/engineer",
			"type": {"type": "FullTime"}, "experience": {"type": "Senior"},
			"country": {"code": "IE", "name": "Ireland"}
		}]
	}`)
	if _, err := NewHireHive(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded with fewer postings than the source stated")
	}
}

func TestHireHiveFetchFailsALaterPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1&page_size=100", `{
			"meta": {"total_items": 2, "has_next_page": true},
			"items": [{
				"id": "job_one", "title": "Engineer",
				"hosted_url": "https://acme.hirehive.com/engineer"
			}]
		}`).
		routeErr("page=2", errors.New("timeout"))
	if _, err := NewHireHive(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded after a later page failed")
	}
}

func TestHireHiveFetchRejectsABadLabel(t *testing.T) {
	if _, err := NewHireHive(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Fatal("Fetch accepted a board label that is not a tenant")
	}
}
