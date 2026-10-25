package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
)

// talentstormFixture reads an API answer captured from api.talentstorm.de (2026-10-25), the
// recruiters' names and numbers blanked: advades (four offers, one an open application) and
// stadt-meinerzhagen (five, two with a stated monthly wage).
func talentstormFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/talentstorm/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestTalentstormProvider(t *testing.T) {
	s, ok := All(nil)["talentstorm"]
	if !ok || s.Provider() != "talentstorm" {
		t.Fatal("All() must register talentstorm")
	}
	if !FullBoardListingProviders(All(nil))["talentstorm"] {
		t.Error("pagination=false answers every offer: talentstorm must be fullBoardListing")
	}
}

func TestTalentstormMapsOffers(t *testing.T) {
	fake := (&routedHTTP{}).route("basic/advades?pagination=false", talentstormFixture(t, "list_advades.json"))
	jobs, err := NewTalentstorm(fake).Fetch(context.Background(), CompanyEntry{Company: "advades GmbH", Provider: "talentstorm", Board: "advades"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 || slices.ContainsFunc(jobs, func(j Job) bool { return j.ExternalID == "2547" }) {
		t.Fatalf("jobs = %+v, want the three positions and not the open application", jobs)
	}
	i := slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "26187" })
	if i < 0 {
		t.Fatal("26187 missing")
	}
	j := jobs[i]
	if j.Title != "SAP Development Consultant CAP (m/w/d)" || j.Company != "advades GmbH" ||
		j.URL != "https://advades.talentstorm.de/stellenangebote/sap-development-consultant-cap-m-w-d-26187" {
		t.Errorf("identity = %q/%q/%q", j.Title, j.Company, j.URL)
	}
	if j.Location != "Herborn, Hessen" || !slices.Equal(j.Countries, []string{"de"}) || j.Description == "" {
		t.Errorf("Location/Countries/body = %q/%v/%d", j.Location, j.Countries, len(j.Description))
	}
	if j.EmploymentType != "full_time" || j.WorkMode != "" || j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-04-23" {
		t.Errorf("EmploymentType/WorkMode/PostedAt = %q/%q/%v", j.EmploymentType, j.WorkMode, j.PostedAt)
	}
	intern := jobs[slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "20770" })]
	if intern.EmploymentType != "internship" {
		t.Errorf("Praktikum EmploymentType = %q, want internship", intern.EmploymentType)
	}
}

func TestTalentstormStatedWage(t *testing.T) {
	fake := (&routedHTTP{}).route("stadt-meinerzhagen", talentstormFixture(t, "list_stadt-meinerzhagen.json"))
	jobs, err := NewTalentstorm(fake).Fetch(context.Background(), CompanyEntry{Provider: "talentstorm", Board: "stadt-meinerzhagen"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("len(jobs) = %d, want 4 (the open application dropped)", len(jobs))
	}
	paid := jobs[slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "28029" })]
	if paid.SalaryMin == nil || *paid.SalaryMin != 3240 || paid.SalaryCurrency != "EUR" || paid.SalaryPeriod != "month" {
		t.Errorf("salary = %v/%q/%q, want 3240 EUR a month", paid.SalaryMin, paid.SalaryCurrency, paid.SalaryPeriod)
	}
	unstated := jobs[slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "26448" })]
	if unstated.SalaryMin != nil || unstated.SalaryCurrency != "" {
		t.Errorf("a wage type without an amount must set no salary, got %v/%q", unstated.SalaryMin, unstated.SalaryCurrency)
	}
}

// A reshaped answer must fail, never read as a site with no offers.
func TestTalentstormReshapedAnswerFails(t *testing.T) {
	fake := (&routedHTTP{}).route("advades", `{"message":"Authentication Required"}`)
	if _, err := NewTalentstorm(fake).Fetch(context.Background(), CompanyEntry{Provider: "talentstorm", Board: "advades"}); err == nil {
		t.Fatal("an object in place of the offer list must fail")
	}
	null := (&routedHTTP{}).route("advades", `null`)
	if _, err := NewTalentstorm(null).Fetch(context.Background(), CompanyEntry{Provider: "talentstorm", Board: "advades"}); err == nil {
		t.Fatal("a null answer must fail")
	}
	empty := (&routedHTTP{}).route("advades", `[]`)
	if jobs, err := NewTalentstorm(empty).Fetch(context.Background(), CompanyEntry{Provider: "talentstorm", Board: "advades"}); err != nil || len(jobs) != 0 {
		t.Fatalf("an empty list is a site with no offers, got %d/%v", len(jobs), err)
	}
}

// 401 is what both a missing Origin and an unknown label answer, so it is a failure, not gone.
func TestTalentstormUnauthorizedIsNotGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("advades", &StatusError{Code: 401})
	_, err := NewTalentstorm(fake).Fetch(context.Background(), CompanyEntry{Provider: "talentstorm", Board: "advades"})
	if err == nil || errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want a failure that is not ErrBoardGone", err)
	}
}
