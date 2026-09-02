package sources

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// schoolspringSearchBody is a one-page keyword slice: two postings, the second carrying the
// HTML-entity encoding the listing (unlike the detail resource) leaves in its titles.
const schoolspringSearchBody = `{
  "success": true, "message": "", "validationErrors": [],
  "value": {"page": 1, "size": 100, "jobsList": [
    {"jobId": 5911831, "employer": "Columbia River High School", "title": "Network Administrator",
     "location": "Vancouver, Washington", "displayDate": "2026-09-02T07:00:00"},
    {"jobId": 4823888, "employer": "Central Office", "title": "Senior Network &amp; Systems Engineer",
     "location": "Uvalde, Texas", "displayDate": "2024-10-04T00:00:00"}
  ]}
}`

// schoolspringDetailBody is the posting resource for 5911831: an hourly wage the employer chose
// to publish, a US contact country, two named places, and the district as the employer.
const schoolspringDetailBody = `{
  "success": true, "message": "", "validationErrors": [],
  "value": {
    "jobInfo": {
      "jobId": 5911831,
      "jobTitle": "Network Administrator",
      "jobDescription": "<p>Run the district network.</p>",
      "employerName": "Vancouver Public Schools",
      "displayEmployer": "Columbia River High School",
      "displayDate": "2026-09-02T07:00:00",
      "postDate": "2026-09-01T22:34:38",
      "jobTypeName": "Full-time",
      "payDisplay": 1, "payMin": 27.02, "payMax": 32.84,
      "salaryCode": "Per Hour", "payTypeName": "",
      "countryName": "United States"
    },
    "jobLocations": [
      {"displayLocation": "Vancouver, Washington"},
      {"displayLocation": "Vancouver, Washington"},
      {"displayLocation": "Camas, Washington"}
    ]
  }
}`

func schoolspringFake() *routedHTTP {
	return (&routedHTTP{}).
		route("GetPagedJobsWithSearch", schoolspringSearchBody).
		route("/api/Jobs/5911831", schoolspringDetailBody)
}

func TestSchoolSpringFetchNewMapsAPosting(t *testing.T) {
	// 4823888 is already ingested, so only 5911831 is hydrated — and the fake has no detail
	// route for 4823888, so a stray detail request would fail the test rather than pass quietly.
	src := NewSchoolSpring(schoolspringFake()).(HydratingSource)
	seen := func(id string) bool { return id == "4823888" }
	jobs, err := src.FetchNew(context.Background(),
		CompanyEntry{Company: "SchoolSpring — Network administrators", Board: "network administrator"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2: %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	got := byID["5911831"]
	if got.Title != "Network Administrator" {
		t.Errorf("Title = %q", got.Title)
	}
	// The DISTRICT employs, not the individual school the listing files the posting under.
	if got.Company != "Vancouver Public Schools" {
		t.Errorf("Company = %q, want the district", got.Company)
	}
	if got.URL != "https://www.schoolspring.com/jobdetail?jobId=5911831" {
		t.Errorf("URL = %q", got.URL)
	}
	// Repeated places collapse; distinct ones are kept, since a posting filed in two towns is
	// hidden from a filter on the second if only the first survives.
	if got.Location != "Vancouver, Washington; Camas, Washington" {
		t.Errorf("Location = %q", got.Location)
	}
	if !strings.Contains(got.Description, "Run the district network.") {
		t.Errorf("Description = %q", got.Description)
	}
	if got.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", got.EmploymentType)
	}
	if !reflect.DeepEqual(got.Countries, []string{"us"}) {
		t.Errorf("Countries = %v", got.Countries)
	}
	if got.PostedAt == nil || got.PostedAt.Format("2006-01-02") != "2026-09-02" {
		t.Errorf("PostedAt = %v, want the display date", got.PostedAt)
	}
	if got.SalaryCurrency != "USD" || got.SalaryPeriod != "hour" ||
		got.SalaryMin == nil || *got.SalaryMin != 27 || got.SalaryMax == nil || *got.SalaryMax != 33 {
		t.Errorf("salary = %v..%v %s/%s", got.SalaryMin, got.SalaryMax, got.SalaryCurrency, got.SalaryPeriod)
	}

	// A seen posting is a liveness refresh: identity only, no detail request, and no content that
	// would overwrite the body hydrated when it was new.
	refresh := byID["4823888"]
	if !refresh.SeenRefresh || refresh.Description != "" {
		t.Errorf("seen posting = %+v, want a content-less refresh", refresh)
	}
	// The listing serves entity-encoded titles; the refresh path has no detail resource to take a
	// decoded one from, so it must decode them itself or the catalogue filter judges "&amp;".
	if refresh.Title != "Senior Network & Systems Engineer" {
		t.Errorf("refresh Title = %q, want the entities decoded", refresh.Title)
	}
	// Company stays empty: the pipeline matches the stored row by identity, and the only employer
	// the listing names is the individual school, not the district the row is filed under. Filling
	// it with the board's own label would put a made-up employer on the wire for no reader.
	if refresh.Company != "" {
		t.Errorf("refresh Company = %q, want empty", refresh.Company)
	}
}

// Fetch is the list-only fallback: with no seen set every posting is hydrated, so the posting the
// fake has no detail route for is dropped rather than stored body-less.
func TestSchoolSpringFetchHydratesEverything(t *testing.T) {
	jobs, err := NewSchoolSpring(schoolspringFake()).Fetch(context.Background(),
		CompanyEntry{Company: "SchoolSpring", Board: "network administrator"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "5911831" {
		t.Fatalf("jobs = %+v, want only the posting whose detail resolved", jobs)
	}
}

// The search URL states the national job board and percent-escapes the multi-word keyword; a
// keyword pasted in raw would be a malformed URL rather than a narrower search.
func TestSchoolSpringSearchURL(t *testing.T) {
	got := schoolspringSearchURL("help desk", 2)
	for _, want := range []string{
		"https://api.schoolspring.com/api/Jobs/GetPagedJobsWithSearch",
		"domainName=www.schoolspring.com",
		"keyword=help+desk",
		"page=2",
		"size=100",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("schoolspringSearchURL() = %q, missing %q", got, want)
		}
	}
}

// An entry with no keyword addresses the whole 80k-posting index — a school board, not an IT one —
// so it is refused rather than crawled.
func TestSchoolSpringRejectsAnEmptyBoard(t *testing.T) {
	_, err := NewSchoolSpring(schoolspringFake()).Fetch(context.Background(),
		CompanyEntry{Company: "SchoolSpring", Board: "  "})
	if err == nil {
		t.Fatal("Fetch with no keyword returned no error")
	}
}

// The API answers a request it refuses with HTTP 200 and success=false, so an adapter reading only
// the payload would mistake a failed first page for an empty slice — which is a board-level error,
// not a board with no postings.
func TestSchoolSpringFirstPageFailureIsABoardError(t *testing.T) {
	cases := map[string]*routedHTTP{
		"transport": (&routedHTTP{}).route("GetPagedJobsWithSearch", `{`),
		"envelope": (&routedHTTP{}).route("GetPagedJobsWithSearch",
			`{"success": false, "message": "Search failed", "value": null}`),
	}
	for name, http := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewSchoolSpring(http).Fetch(context.Background(),
				CompanyEntry{Company: "SchoolSpring", Board: "webmaster"})
			if err == nil {
				t.Fatal("Fetch returned no error")
			}
		})
	}
}

// A posting whose detail request fails, or whose detail carries no body or no employer, is skipped
// rather than stored: a stored row is `seen`, so it would keep its empty description past the
// hydration-retry window and never reach the search index, whereas a skipped one is retried.
func TestSchoolSpringSkipsAnUnusablePosting(t *testing.T) {
	cases := map[string]string{
		"detail refused": `{"success": false, "message": "JobDetail not found : 5911831", "value": {"jobInfo": null}}`,
		"no body": `{"success": true, "value": {"jobInfo": {"jobTitle": "Network Administrator",
			"employerName": "Vancouver Public Schools", "jobDescription": "   "}, "jobLocations": []}}`,
		"no employer": `{"success": true, "value": {"jobInfo": {"jobTitle": "Network Administrator",
			"employerName": "", "jobDescription": "<p>Run the district network.</p>"}, "jobLocations": []}}`,
	}
	for name, detail := range cases {
		t.Run(name, func(t *testing.T) {
			http := (&routedHTTP{}).
				route("GetPagedJobsWithSearch", schoolspringSearchBody).
				route("/api/Jobs/5911831", detail)
			jobs, err := NewSchoolSpring(http).Fetch(context.Background(),
				CompanyEntry{Company: "SchoolSpring", Board: "network administrator"})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(jobs) != 0 {
				t.Fatalf("jobs = %+v, want none", jobs)
			}
		})
	}
}

// A later page failing ends the walk with what has been gathered, so a mid-listing hiccup costs a
// page rather than the whole board.
func TestSchoolSpringLaterPageFailureKeepsTheEarlierPage(t *testing.T) {
	http := &pagedSchoolSpringHTTP{pages: []string{schoolspringSearchBody}, detail: schoolspringDetailBody}
	jobs, err := NewSchoolSpring(http).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "SchoolSpring", Board: "network administrator"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want the first page's 2", len(jobs))
	}
}

// The walk stops on a page that adds no posting it has not already collected — the endpoint
// reports no total, so a repeated page is the only other end-of-slice signal there is.
func TestSchoolSpringStopsOnAPageThatAddsNothing(t *testing.T) {
	http := &pagedSchoolSpringHTTP{
		pages:  []string{schoolspringSearchBody, schoolspringSearchBody, schoolspringSearchBody},
		detail: schoolspringDetailBody,
	}
	jobs, err := NewSchoolSpring(http).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "SchoolSpring", Board: "network administrator"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2 distinct postings", len(jobs))
	}
	if http.listCalls != 2 {
		t.Errorf("listCalls = %d, want the walk to stop on the first repeated page", http.listCalls)
	}
}

func TestSchoolSpringSalaryPeriod(t *testing.T) {
	cases := map[string]string{
		"Per Year": "year", "Per Hour": "hour", "Per Month": "month", "Per Day": "day",
		"yearly": "year", "HOURLY": "hour", " Per Year ": "year",
		// Employer-authored free text that names no period at all: the amount is dropped with it
		// rather than published as a figure whose unit is a guess.
		"BU Salary Schedule": "", "SY": "", "Based on Education and Experience": "",
		"Per Semester": "", "Not Applicable": "", "": "",
	}
	for in, want := range cases {
		if got := schoolspringSalaryPeriod(in); got != want {
			t.Errorf("schoolspringSalaryPeriod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSchoolSpringEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full-time": "full_time", "Part-time": "part_time",
		// The unpaid variants state a schedule too; freehire's employment type is about how much
		// of a week a role takes, not about pay.
		"Full-time Unpaid": "full_time", "Part-time Unpaid": "part_time",
		// The rest of the platform's enum names no schedule freehire has a value for, so the
		// description parser decides instead.
		"Summer": "", "After school/Evening": "", "Not provided": "", "": "",
	}
	for in, want := range cases {
		if got := schoolspringEmploymentType(in); got != want {
			t.Errorf("schoolspringEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

// The pay figures are bare numbers and nothing on either resource names a currency — the site
// simply renders a "$". So a salary is published only for a posting whose own country vouches for
// that currency, and only when the employer switched publishing on and named a period we have.
func TestSchoolSpringApplySalary(t *testing.T) {
	full := schoolspringJobInfo{
		PayDisplay: 1, PayMin: 55000, PayMax: 70000, SalaryCode: "Per Year", CountryName: "United States",
	}
	cases := []struct {
		name string
		info schoolspringJobInfo
		want bool
	}{
		{"complete", full, true},
		{"period from payTypeName when salaryCode is blank", func() schoolspringJobInfo {
			i := full
			i.SalaryCode, i.PayTypeName = "", "Per Year"
			return i
		}(), true},
		{"one-sided range still publishes", func() schoolspringJobInfo {
			i := full
			i.PayMax = 0
			return i
		}(), true},
		{"employer did not publish the figure", func() schoolspringJobInfo {
			i := full
			i.PayDisplay = 0
			return i
		}(), false},
		{"no country: the dollar sign would be an assumption", func() schoolspringJobInfo {
			i := full
			i.CountryName = ""
			return i
		}(), false},
		{"a country whose currency is not the dollar", func() schoolspringJobInfo {
			i := full
			i.CountryName = "United Arab Emirates"
			return i
		}(), false},
		{"period the platform states as free text", func() schoolspringJobInfo {
			i := full
			i.SalaryCode = "BU Salary Schedule"
			return i
		}(), false},
		{"no amount", func() schoolspringJobInfo {
			i := full
			i.PayMin, i.PayMax = 0, 0
			return i
		}(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var job Job
			tc.info.applySalary(&job)
			if got := job.SalaryCurrency != ""; got != tc.want {
				t.Errorf("applySalary published = %v (%v..%v %s/%s), want %v",
					got, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod, tc.want)
			}
			if tc.want && job.SalaryCurrency != "USD" {
				t.Errorf("SalaryCurrency = %q, want USD", job.SalaryCurrency)
			}
		})
	}
}

// pagedSchoolSpringHTTP serves one listing body per call and then fails, so a test can say what
// each page of a walk answers. The detail body is shared by every posting.
type pagedSchoolSpringHTTP struct {
	pages     []string
	detail    string
	listCalls int
}

func (p *pagedSchoolSpringHTTP) GetJSON(_ context.Context, url string, v any) error {
	if !strings.Contains(url, "GetPagedJobsWithSearch") {
		return json.Unmarshal([]byte(p.detail), v)
	}
	p.listCalls++
	if p.listCalls > len(p.pages) {
		return errors.New("schoolspring test: no page left")
	}
	return json.Unmarshal([]byte(p.pages[p.listCalls-1]), v)
}
