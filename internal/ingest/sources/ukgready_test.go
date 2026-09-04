package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseUKGReadyBoard(t *testing.T) {
	cases := []struct {
		board        string
		wantOK       bool
		host, tenant string
	}{
		{"secure4.saashr.com/6162397", true, "secure4.saashr.com", "6162397"},
		{"secure.workforceready.com.au/6179255", true, "secure.workforceready.com.au", "6179255"},
		{"secure5.entertimeonline.com/cbiz767aa", true, "secure5.entertimeonline.com", "cbiz767aa"},
		// The mykronos hosts are the same platform under UKG's own brand, and their labels run
		// deeper than the other families' — the host is taken whole, never sliced.
		{"aus-secure.prd.mykronos.com/6183838", true, "aus-secure.prd.mykronos.com", "6183838"},
		{"prd01-hcm01.npr.mykronos.com/6042637", true, "prd01-hcm01.npr.mykronos.com", "6042637"},
		{"6162397", false, "", ""},                          // no host
		{"secure4.saashr.com/", false, "", ""},              // no tenant
		{"/6162397", false, "", ""},                         // no host
		{"secure4.saashr.com/6162397/extra", false, "", ""}, // a path this adapter never builds
		{"", false, "", ""},
	}
	for _, tc := range cases {
		b, err := parseUKGReadyBoard(tc.board)
		if (err == nil) != tc.wantOK || b.host != tc.host || b.tenant != tc.tenant {
			t.Errorf("parseUKGReadyBoard(%q) = (%q,%q,err=%v), want (%q,%q,ok=%v)",
				tc.board, b.host, b.tenant, err, tc.host, tc.tenant, tc.wantOK)
		}
	}
}

// Two pod hosts of one residency region serve the same tenant, so two board ids that differ
// only in the host are one crawl target — ukgreadyTenant is what boardIdentity folds them with,
// and without it the tenant is crawled twice under two external_id namespaces.
func TestUKGReadyTenantFoldsThePodHost(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Albanese Confectionery", Provider: "ukgready", Board: "secure4.saashr.com/6162397"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Albanese (harvested again)", Provider: "ukgready", Board: "secure2.entertimeonline.com/6162397"})
	if a != b {
		t.Errorf("two pod hosts of one tenant should share a key, got %q and %q", a, b)
	}
	other, _ := BoardDedupeKey(CompanyEntry{Company: "Rapid Robert's", Provider: "ukgready", Board: "secure.entertimeonline.com/10284"})
	if other == a {
		t.Error("a different tenant must not fold onto this one")
	}
	// A board with no host part folds to itself rather than to nothing, so a malformed entry
	// cannot collapse every other malformed entry into one.
	if got := ukgreadyTenant("6162397"); got != "6162397" {
		t.Errorf("ukgreadyTenant(%q) = %q", "6162397", got)
	}
}

func TestUKGReadySalaryPeriod(t *testing.T) {
	cases := map[string]string{
		"YEAR": "year", "MONTH": "month", "HOUR": "hour",
		// WEEK is a real UKG frequency with no value in our vocabulary, so the amount is
		// dropped with the period rather than published as something it is not.
		"WEEK": "", "": "", "FORTNIGHT": "",
	}
	for in, want := range cases {
		if got := ukgreadySalaryPeriod(in); got != want {
			t.Errorf("ukgreadySalaryPeriod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUKGReadyEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full Time": "full_time", "Full-Time": "full_time", "FT Non-Exempt": "full_time",
		"Regular (Full Time)": "full_time", "Salaried, Full-time": "full_time",
		"Reg FT - Non-Exempt": "full_time",
		"Part Time":           "part_time", "PT Non-Exempt": "part_time", "Part-Time": "part_time",
		// Pay classes and tenant-invented labels state no schedule at all.
		"Exempt": "", "Non-Exempt": "", "Student Assistant": "", "SC2 - Supplemental": "", "": "",
		// A label claiming both is not a schedule statement either.
		"FT/PT": "",
	}
	for in, want := range cases {
		if got := ukgreadyEmploymentType(in); got != want {
			t.Errorf("ukgreadyEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

// ukgreadyListing is a one-page listing whose single posting states an hourly range.
const ukgreadyListing = `{"_paging":{"offset":0,"size":200,"total":2},"job_requisitions":[
	{"id":1040452356,"job_title":"Backend Engineer","job_description":"a truncated preview...",
	 "location":{"address_line_1":"5441 E Lincoln Hwy","city":"Merrillville","state":"IN","country":"USA","zip":"46410"},
	 "base_pay_from":32.5,"base_pay_to":41,"base_pay_frequency":"HOUR",
	 "employee_type":{"name":"FT Non-Exempt"},"is_remote_job":false},
	{"id":1040451353,"job_title":"Data Analyst","job_description":"another preview...",
	 "location":{"city":"Sydney","country":"AUS"},
	 "base_pay_frequency":"YEAR","employee_type":{"name":"Exempt"},"is_remote_job":true}
]}`

const ukgreadySettings = `{"locale":{"currency_code":"USD","currency_symbol":"$"}}`

func TestUKGReadyFetch(t *testing.T) {
	// The collection route carries the query marker: routedHTTP matches by substring, and the
	// collection URL is a prefix of every detail URL.
	http := (&routedHTTP{}).
		route("/job-requisitions/", `{"id":1,"job_description":"<p>Full body</p>","job_requirement":"<p>Five years</p>"}`).
		route("/format-settings", ukgreadySettings).
		route("/job-requisitions?offset", ukgreadyListing)

	jobs, err := ukgready{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "secure4.saashr.com/6162397"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "1040452356" || j.Title != "Backend Engineer" || j.Company != "Acme" {
		t.Errorf("job0 id/title/company = %q/%q/%q", j.ExternalID, j.Title, j.Company)
	}
	if want := "https://secure4.saashr.com/ta/6162397.careers?ShowJob=1040452356"; j.URL != want {
		t.Errorf("job0 URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Merrillville, IN, USA" {
		t.Errorf("job0 location = %q", j.Location)
	}
	// The street line and zip stay out of the location, and the alpha-3 country resolves.
	if len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("job0 Countries = %v, want [us]", j.Countries)
	}
	// The detail resource supplies the body; the requirements block is appended to it and the
	// listing's truncated preview never reaches the catalogue.
	if !strings.Contains(j.Description, "Full body") || !strings.Contains(j.Description, "Five years") {
		t.Errorf("job0 description = %q", j.Description)
	}
	if strings.Contains(j.Description, "truncated preview") {
		t.Errorf("job0 description kept the listing preview: %q", j.Description)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("job0 EmploymentType = %q, want full_time", j.EmploymentType)
	}
	// The bare pay numbers are published only once the tenant's format settings name a currency.
	if j.SalaryMin == nil || *j.SalaryMin != 33 || j.SalaryMax == nil || *j.SalaryMax != 41 {
		t.Errorf("job0 salary = %v..%v, want 33..41 (32.5 rounds up)", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "USD" || j.SalaryPeriod != "hour" {
		t.Errorf("job0 salary currency/period = %q/%q", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt != nil {
		t.Errorf("job0 PostedAt = %v, want nil: the platform publishes no posting date", j.PostedAt)
	}

	// The second posting states the remote flag and no pay bounds.
	if !jobs[1].Remote || jobs[1].WorkMode != "remote" {
		t.Errorf("job1 remote/workmode = %v/%q", jobs[1].Remote, jobs[1].WorkMode)
	}
	if jobs[1].SalaryMin != nil || jobs[1].SalaryPeriod != "" {
		t.Errorf("job1 salary = %v/%q, want none", jobs[1].SalaryMin, jobs[1].SalaryPeriod)
	}
	if jobs[1].EmploymentType != "" {
		t.Errorf("job1 EmploymentType = %q, want empty (a pay class states no schedule)", jobs[1].EmploymentType)
	}
}

// "offset" is a 1-based PAGE NUMBER, so the walk asks for page 2, not for row 200. Advancing it
// by the row count instead reads page 1 and then page 200 — which the API answers empty, with no
// error — so every board past one page would silently truncate.
func TestUKGReadyFetchWalksPagesNotRowOffsets(t *testing.T) {
	page := func(total int, ids ...int) string {
		rows := make([]string, len(ids))
		for i, id := range ids {
			rows[i] = fmt.Sprintf(`{"id":%d,"job_title":"Engineer %d"}`, id, id)
		}
		return fmt.Sprintf(`{"_paging":{"total":%d},"job_requisitions":[%s]}`, total, strings.Join(rows, ","))
	}
	http := (&routedHTTP{}).
		route("/job-requisitions/", `{"id":1,"job_description":"<p>Body</p>"}`).
		route("offset=1&size=200", page(3, 1, 2)).
		route("offset=2&size=200", page(3, 3))
		// No offset=3 route: the total is what ends the walk, so a third page is never asked for.

	jobs, err := ukgready{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "secure4.saashr.com/6162397"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 across two pages: %+v", len(jobs), jobs)
	}
	for i, want := range []string{"1", "2", "3"} {
		if jobs[i].ExternalID != want {
			t.Errorf("jobs[%d].ExternalID = %q, want %q", i, jobs[i].ExternalID, want)
		}
	}
}

// A board whose postings state no publishable pay must not pay for the tenant's format
// settings — the request exists only to qualify an amount.
func TestUKGReadyFetchSkipsSettingsWithoutPay(t *testing.T) {
	listing := `{"_paging":{"total":1},"job_requisitions":[
		{"id":7,"job_title":"Backend Engineer","base_pay_frequency":"WEEK","base_pay_from":1200,
		 "location":{"city":"Merrillville","country":"USA"}}
	]}`
	http := (&routedHTTP{}).
		route("/job-requisitions/", `{"id":7,"job_description":"<p>Body</p>"}`).
		route("/job-requisitions?offset", listing)
		// No /format-settings route: reaching for it would error the test's fake client.

	jobs, err := ukgready{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "secure4.saashr.com/6162397"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].SalaryMin != nil {
		t.Fatalf("want 1 job with no salary, got %+v", jobs)
	}
}

// A posting the catalogue already holds is refreshed by identity: no detail request, and no
// content that would overwrite the body hydrated when it was new.
func TestUKGReadyFetchNewSkipsSeenDetail(t *testing.T) {
	http := (&routedHTTP{}).
		route("/job-requisitions/", `{"id":1,"job_description":"<p>Full body</p>"}`).
		route("/format-settings", ukgreadySettings).
		route("/job-requisitions?offset", ukgreadyListing)

	seen := func(id string) bool { return id == "1040452356" }
	jobs, err := ukgready{http: http}.FetchNew(context.Background(),
		CompanyEntry{Company: "Acme", Board: "secure4.saashr.com/6162397"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	refresh := jobs[0]
	if !refresh.SeenRefresh || refresh.ExternalID != "1040452356" || refresh.Title != "Backend Engineer" {
		t.Errorf("seen posting = %+v, want an identity-only refresh", refresh)
	}
	if refresh.Description != "" || refresh.SalaryMin != nil {
		t.Errorf("seen posting carries content: %+v", refresh)
	}
	if jobs[1].SeenRefresh || !strings.Contains(jobs[1].Description, "Full body") {
		t.Errorf("unseen posting = %+v, want a hydrated job", jobs[1])
	}
}

// A posting whose detail request fails is skipped rather than stored with the listing's
// truncated preview: it stays unseen, so the next crawl retries it.
func TestUKGReadyFetchDropsPostingWithoutDetail(t *testing.T) {
	http := (&routedHTTP{}).
		route("/format-settings", ukgreadySettings).
		route("/job-requisitions?offset", ukgreadyListing) // no detail route → GetJSON errors

	jobs, err := ukgready{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "secure4.saashr.com/6162397"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// The first listing page failing is a board-level error; the pipeline counts the board failed
// rather than sweeping its still-live postings as unseen.
func TestUKGReadyFetchFailsOnFirstListingPage(t *testing.T) {
	if _, err := (ukgready{http: &routedHTTP{}}).Fetch(context.Background(),
		CompanyEntry{Board: "secure4.saashr.com/6162397"}); err == nil {
		t.Error("want error when the listing is unreachable")
	}
}

func TestUKGReadyFetchBadBoard(t *testing.T) {
	if _, err := (ukgready{http: &routedHTTP{}}).Fetch(context.Background(),
		CompanyEntry{Board: "6162397"}); err == nil {
		t.Error("want error for a board with no host")
	}
}
