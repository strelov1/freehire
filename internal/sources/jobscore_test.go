package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// jobscoreFeedJSON mirrors a careers.jobscore.com feed.json response: company identity plus
// postings with structured location, a Yes/No remote string, job_type, experience_level, an
// opened_date with millisecond precision, and an HTML description.
const jobscoreFeedJSON = `{
  "company_name": "IERUS Technologies, Inc.",
  "jobs": [
    {
      "id": "dgLTVq3fbbbkS61af48PpK",
      "title": "Systems and Network Administrator",
      "detail_url": "https://careers.jobscore.com/careers/ierustechnologiesinc/jobs/systems-and-network-administrator-dgLTVq3fbbbkS61af48PpK?ref=rss&sid=68",
      "description": "<p>IERUS specializes in RF.</p><script>alert(1)</script>",
      "city": "Huntsville",
      "state": "AL",
      "country": "US",
      "location": "Huntsville, AL, US",
      "remote": "No | Must be able to work onsite all of the time",
      "job_type": "Full Time",
      "experience_level": "Mid-Level",
      "opened_date": "2026-03-31T17:04:28.293Z"
    },
    {
      "id": "remote42",
      "title": "Senior Front-End Engineer",
      "detail_url": "https://careers.jobscore.com/careers/x/jobs/fe-remote42",
      "description": "<p>Build UI.</p>",
      "location": "Remote in Joinville, Santa Catarina, Brazil",
      "remote": "Yes | Can work remotely all of the time",
      "job_type": "Contractor",
      "experience_level": "Experienced (Non-Manager)",
      "opened_date": "2026-05-01"
    }
  ]
}`

func newJobscoreFake() *routedHTTP {
	return (&routedHTTP{}).route("/jobs/ierustechnologiesinc/feed.json", jobscoreFeedJSON)
}

func TestJobscoreProvider(t *testing.T) {
	if got := NewJobscore(nil).Provider(); got != "jobscore" {
		t.Errorf("Provider() = %q, want %q", got, "jobscore")
	}
}

func TestJobscoreFetchMapsFeed(t *testing.T) {
	jobs, err := NewJobscore(newJobscoreFake()).Fetch(context.Background(), CompanyEntry{
		Company: "IERUS Technologies", Provider: "jobscore", Board: "ierustechnologiesinc",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	a := jobs[0]
	if a.ExternalID != "dgLTVq3fbbbkS61af48PpK" {
		t.Errorf("ExternalID = %q", a.ExternalID)
	}
	if a.Company != "IERUS Technologies" {
		t.Errorf("Company = %q, want config company", a.Company)
	}
	if !strings.HasPrefix(a.URL, "https://careers.jobscore.com/careers/ierustechnologiesinc/") {
		t.Errorf("URL = %q, want the detail_url", a.URL)
	}
	if a.Location != "Huntsville, AL" {
		t.Errorf("Location = %q, want \"Huntsville, AL\"", a.Location)
	}
	if a.WorkMode != "onsite" || a.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want onsite/false", a.WorkMode, a.Remote)
	}
	if a.Seniority != "middle" {
		t.Errorf("Seniority = %q, want middle", a.Seniority)
	}
	if a.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", a.EmploymentType)
	}
	if strings.Contains(a.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", a.Description)
	}
	if a.PostedAt == nil || !a.PostedAt.Equal(time.Date(2026, 3, 31, 17, 4, 28, 293000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-03-31T17:04:28.293Z", a.PostedAt)
	}

	b := jobs[1]
	if b.WorkMode != "remote" || !b.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want remote/true", b.WorkMode, b.Remote)
	}
	if b.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", b.EmploymentType)
	}
	if b.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior", b.Seniority)
	}
	if b.Location != "Remote in Joinville, Santa Catarina, Brazil" {
		t.Errorf("Location = %q, want the free-text location fallback", b.Location)
	}
	if b.PostedAt == nil || !b.PostedAt.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-01", b.PostedAt)
	}
}

func TestJobscoreCompanyFallsBackToFeed(t *testing.T) {
	jobs, err := NewJobscore(newJobscoreFake()).Fetch(context.Background(), CompanyEntry{Board: "ierustechnologiesinc"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[0].Company != "IERUS Technologies, Inc." {
		t.Errorf("Company = %q, want feed company_name fallback", jobs[0].Company)
	}
}

func TestJobscoreDropsJobWithNoID(t *testing.T) {
	fake := (&routedHTTP{}).route("/jobs/x/feed.json", `{"company_name":"X","jobs":[{"title":"No ID"}]}`)
	jobs, err := NewJobscore(fake).Fetch(context.Background(), CompanyEntry{Company: "X", Board: "x"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (no-id dropped)", len(jobs))
	}
}

func TestJobscoreEmptyBoardErrors(t *testing.T) {
	if _, err := NewJobscore(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("expected error for empty board")
	}
}

func TestJobscoreWorkMode(t *testing.T) {
	cases := map[string]string{
		"No | Must be able to work onsite all of the time": "onsite",
		"Yes | Can work remotely all of the time":          "remote",
		"Yes | Can work remotely some of the time":         "hybrid",
		"": "",
	}
	for in, want := range cases {
		if got := jobscoreWorkMode(in); got != want {
			t.Errorf("jobscoreWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJobscoreSeniority(t *testing.T) {
	cases := map[string]string{
		"Student (College)":                     "intern",
		"Entry Level":                           "junior",
		"Associate":                             "junior",
		"Mid-Level":                             "middle",
		"Experienced (Non-Manager)":             "senior",
		"Manager (Manager/Supervisor of Staff)": "",
		"Executive (SVP, VP, Department Head)":  "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := jobscoreSeniority(in); got != want {
			t.Errorf("jobscoreSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}
