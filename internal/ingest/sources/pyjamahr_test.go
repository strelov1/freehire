package sources

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func pyjamahrListingURL(board string, page int) string {
	if page <= 1 {
		return "https://api.pyjamahr.com/api/career/jobs/?company_slug=" + board + "&page=1"
	}
	return "https://api.pyjamahr.com/api/career/jobs/?company_slug=" + board + "&page=2"
}

func pyjamahrDetailURL(board string, id int) string {
	return "https://api.pyjamahr.com/api/career/jobs/" + strconv.Itoa(id) + "/?company_slug=" + board
}

const pyjamahrPage1 = `{"count":2,"next":"https://api.pyjamahr.com/api/career/jobs/?company_slug=dodo-payments&page=2","previous":null,"results":[
{"id":404497,"slug":"talent-acquisition-intern-2","title":"Talent Acquisition Intern","min_experience":0.0,"max_experience":1.0,"country":"India","location":"Bengaluru, Karnataka, India","other_locations":[],"department_name":null,"workplace_type":"ON_SITE","product":null}
]}`

const pyjamahrPage2 = `{"count":2,"next":null,"previous":"x","results":[
{"id":403456,"slug":"growth-intern-sales","title":"Growth Intern","min_experience":0.0,"max_experience":1.0,"country":"India","location":"Bengaluru, Karnataka, India","other_locations":[],"department_name":null,"workplace_type":"REMOTE","product":null}
]}`

const pyjamahrEmptyPage = `{"count":0,"next":null,"previous":null,"results":[]}`

// pyjamahrSinglePage is a one-item, single-page (next:null) listing, for tests that only
// care about one posting and don't want to also route a second page.
const pyjamahrSinglePage = `{"count":1,"next":null,"previous":null,"results":[
{"id":404497,"slug":"talent-acquisition-intern-2","title":"Talent Acquisition Intern","min_experience":0.0,"max_experience":1.0,"country":"India","location":"Bengaluru, Karnataka, India","other_locations":[],"department_name":null,"workplace_type":"ON_SITE","product":null}
]}`

const pyjamahrDetail404497 = `{"id":404497,"uuid":"EB9609F7F3","title":"Talent Acquisition Intern",
"job_type":"INTERN","description":"<p>Great internship.</p>","min_salary":15000.0,"max_salary":20000.0,
"currency":"INR","salary_type":"MONTHLY","is_salary_visible":true,"skill":["recruitment","onboarding"],
"min_experience":0.0,"workplace_type":"ON_SITE","remote":false,"created_at":"2026-09-10T14:40:13+05:30"}`

const pyjamahrDetail403456 = `{"id":403456,"uuid":"AA1122","title":"Growth Intern",
"job_type":"INTERN","description":"<p>Growth role.</p>","min_salary":null,"max_salary":null,
"currency":"INR","salary_type":"MONTHLY","is_salary_visible":false,"skill":["sales"],
"min_experience":0.0,"workplace_type":"REMOTE","remote":true,"created_at":"2026-09-08T08:59:05+05:30"}`

func TestPyjamahrProvider(t *testing.T) {
	if got := NewPyjamahr(nil).Provider(); got != "pyjamahr" {
		t.Errorf("Provider() = %q, want %q", got, "pyjamahr")
	}
}

func TestPyjamahrMarkers(t *testing.T) {
	s := NewPyjamahr(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("pyjamahr should implement the fullBoardListing marker")
	}
}

func TestPyjamahrRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["pyjamahr"] {
		t.Error("FullBoardListingProviders(All(nil)) should include pyjamahr")
	}
}

func TestPyjamahrRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["pyjamahr"]
	if !ok {
		t.Fatal("All() missing provider pyjamahr")
	}
	if s.Provider() != "pyjamahr" {
		t.Errorf("All()[pyjamahr].Provider() = %q", s.Provider())
	}
}

func TestPyjamahrFetchPaginatesListsAndHydrates(t *testing.T) {
	board := "dodo-payments"
	fake := (&routedHTTP{}).
		route(pyjamahrListingURL(board, 1), pyjamahrPage1).
		route(pyjamahrListingURL(board, 2), pyjamahrPage2).
		route(pyjamahrDetailURL(board, 404497), pyjamahrDetail404497).
		route(pyjamahrDetailURL(board, 403456), pyjamahrDetail403456)

	jobs, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Dodo Payments"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (union of both pages)", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "404497" {
		t.Errorf("ExternalID = %q, want 404497", j1.ExternalID)
	}
	if j1.Company != "Dodo Payments" {
		t.Errorf("Company = %q, want the configured CompanyEntry.Company", j1.Company)
	}
	if j1.Location != "Bengaluru, Karnataka, India" {
		t.Errorf("Location = %q", j1.Location)
	}
	if j1.EmploymentType != "internship" {
		t.Errorf("EmploymentType = %q, want internship (INTERN)", j1.EmploymentType)
	}
	if j1.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (ON_SITE)", j1.WorkMode)
	}
	if j1.Description != "<p>Great internship.</p>" {
		t.Errorf("Description = %q", j1.Description)
	}
	if j1.SalaryMin == nil || *j1.SalaryMin != 15000 || j1.SalaryMax == nil || *j1.SalaryMax != 20000 {
		t.Errorf("SalaryMin/Max = %v/%v, want 15000/20000 (is_salary_visible=true)", j1.SalaryMin, j1.SalaryMax)
	}
	if j1.SalaryPeriod != "month" {
		t.Errorf("SalaryPeriod = %q, want month (MONTHLY)", j1.SalaryPeriod)
	}
	if j1.URL != "https://jobs.pyjamahr.com/dodo-payments/talent-acquisition-intern-2" {
		t.Errorf("URL = %q", j1.URL)
	}
	if j1.ExperienceYearsMin == nil || *j1.ExperienceYearsMin != 0 {
		t.Errorf("ExperienceYearsMin = %v, want a real 0 (\"no experience required\" is a stated fact, not absent data)", j1.ExperienceYearsMin)
	}

	j2 := jobs[1]
	if j2.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (REMOTE)", j2.WorkMode)
	}
	if !j2.Remote {
		t.Errorf("Remote = false, want true")
	}
	if j2.SalaryMin != nil || j2.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil (is_salary_visible=false)", j2.SalaryMin, j2.SalaryMax)
	}
}

// pyjamahrDetailHiddenSalary carries real, non-null salary bounds but is_salary_visible
// false — isolating the visibility gate from the null-bounds branch (a fixture where BOTH
// conditions would hide the salary can't tell which one the code is actually checking).
const pyjamahrDetailHiddenSalary = `{"id":1,"uuid":"ZZ","title":"A",
"job_type":"FULLTIME","description":"<p>A.</p>","min_salary":50000.0,"max_salary":80000.0,
"currency":"INR","salary_type":"ANNUAL","is_salary_visible":false,"skill":[],
"min_experience":2.0,"workplace_type":"ON_SITE","remote":false,"created_at":"2026-09-08T08:59:05+05:30"}`

func TestPyjamahrSalaryHiddenDespiteRealBoundsPresent(t *testing.T) {
	board := "acme"
	item := `{"id":1,"slug":"a","title":"A","min_experience":2.0,"max_experience":5.0,"country":"India","location":"India","other_locations":[],"department_name":null,"workplace_type":"ON_SITE","product":null}`
	page := `{"count":1,"next":null,"previous":null,"results":[` + item + `]}`
	fake := (&routedHTTP{}).
		route(pyjamahrListingURL(board, 1), page).
		route(pyjamahrDetailURL(board, 1), pyjamahrDetailHiddenSalary)

	jobs, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].SalaryMin != nil || jobs[0].SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil: is_salary_visible=false must hide real bounds, not just null ones",
			jobs[0].SalaryMin, jobs[0].SalaryMax)
	}
}

func TestPyjamahrEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	board := "empty-co"
	fake := (&routedHTTP{}).route(pyjamahrListingURL(board, 1), pyjamahrEmptyPage)
	jobs, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestPyjamahrFetchFailsWholeBoardOnListingError(t *testing.T) {
	board := "boom-co"
	fake := (&routedHTTP{}).routeErr(pyjamahrListingURL(board, 1), errors.New("boom"))
	if _, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

// A later-page failure must abort the whole Fetch, never return a partial result as
// success — the property the fullBoardListing marker rests on.
func TestPyjamahrFetchFailsWholeBoardOnLaterPageError(t *testing.T) {
	board := "dodo-payments"
	fake := (&routedHTTP{}).
		route(pyjamahrListingURL(board, 1), pyjamahrPage1).
		routeErr(pyjamahrListingURL(board, 2), errors.New("boom"))

	if _, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite a later-page listing error")
	}
}

// pyjamahrNeverEndingPageJSON always carries a non-empty "next", the shape a misbehaving
// tenant (or a page-count bug) would produce — reaching the safety ceiling here must fail
// the whole Fetch, never quietly return the partial result gathered so far.
const pyjamahrNeverEndingPageJSON = `{"count":999,"next":"https://api.pyjamahr.com/api/career/jobs/?company_slug=loop-co&page=2","previous":null,"results":[
{"id":1,"slug":"a","title":"A","min_experience":0.0,"max_experience":1.0,"country":"India","location":"India","other_locations":[],"department_name":null,"workplace_type":"ON_SITE","product":null}
]}`

func TestPyjamahrFetchFailsAtSafetyCeiling(t *testing.T) {
	fake := (&routedHTTP{}).route("company_slug=loop-co", pyjamahrNeverEndingPageJSON) // matches every page URL
	_, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: "loop-co"})
	if err == nil {
		t.Fatal("Fetch succeeded despite next never becoming null")
	}
}

func TestPyjamahrUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	board := "dodo-payments"
	fake := (&routedHTTP{}).
		route(pyjamahrListingURL(board, 1), pyjamahrSinglePage).
		routeErr(pyjamahrDetailURL(board, 404497), errors.New("timeout"))

	jobs, err := NewPyjamahr(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Dodo Payments"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (an unreadable marker, not a drop)", len(jobs))
	}
	if jobs[0].ExternalID != "404497" {
		t.Errorf("ExternalID = %q, want 404497", jobs[0].ExternalID)
	}
}
