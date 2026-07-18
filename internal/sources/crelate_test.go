package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// crelateFeedJSON mirrors a candidateportal GetAllJobs response: a Jobs array plus the
// IsError/ErrorMessage envelope. Fields match the live Career Tree Network portal — a per-posting
// CompanyName (aggregator), structured City/State/Country, a relative Url job code, and an
// RFC3339 LastPostedOnDate with fractional seconds.
const crelateFeedJSON = `{
  "Jobs": [
    {
      "Id": "2ab5a6fa-8b64-414a-9cd0-d6de0804d599",
      "Title": "Physical Therapist Assistant",
      "CompanyName": "Career Tree Network (Discovery At Home)",
      "Description": "<p>Home Health role.</p><script>alert(1)</script>",
      "City": "Tampa",
      "State": "FL",
      "Country": "United States",
      "Url": "/mw9k5zg5f4m8hfigm4p8hnzz6w",
      "JobCode": "mw9k5zg5f4m8hfigm4p8hnzz6w",
      "LastPostedOnDate": "2026-07-17T17:17:12.81Z"
    },
    {
      "Id": "no-company-1",
      "Title": "Recruiter",
      "Description": "<p>Join us.</p>",
      "Country": "United States",
      "JobCode": "abc123",
      "LastPostedOnDate": "2026-07-01T00:00:00Z"
    }
  ],
  "IsError": false,
  "ErrorMessage": null
}`

func newCrelateFake() *routedHTTP {
	return (&routedHTTP{}).route("candidateportal/GetAllJobs", crelateFeedJSON)
}

const crelateBoard = "careertree:ec546cba-84d5-4d8a-97e5-52e8ef47db08"

func TestCrelateProvider(t *testing.T) {
	if got := NewCrelate(nil).Provider(); got != "crelate" {
		t.Errorf("Provider() = %q, want %q", got, "crelate")
	}
}

func TestCrelateIsAggregator(t *testing.T) {
	if _, ok := NewCrelate(nil).(aggregator); !ok {
		t.Error("NewCrelate should implement aggregator (a portal fronts many client companies)")
	}
}

func TestCrelateFetchMapsJobs(t *testing.T) {
	jobs, err := NewCrelate(newCrelateFake()).Fetch(context.Background(), CompanyEntry{
		Company: "Career Tree Network", Provider: "crelate", Board: crelateBoard,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	a := jobs[0]
	if a.ExternalID != "2ab5a6fa-8b64-414a-9cd0-d6de0804d599" {
		t.Errorf("ExternalID = %q", a.ExternalID)
	}
	if a.Title != "Physical Therapist Assistant" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Company != "Career Tree Network (Discovery At Home)" {
		t.Errorf("Company = %q, want per-posting CompanyName (aggregator)", a.Company)
	}
	wantURL := "https://jobs.crelate.com/portal/careertree/job/mw9k5zg5f4m8hfigm4p8hnzz6w"
	if a.URL != wantURL {
		t.Errorf("URL = %q, want %q", a.URL, wantURL)
	}
	if a.Location != "Tampa, FL" {
		t.Errorf("Location = %q, want \"Tampa, FL\"", a.Location)
	}
	if strings.Contains(a.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", a.Description)
	}
	if a.PostedAt == nil || !a.PostedAt.Equal(time.Date(2026, 7, 17, 17, 17, 12, 810000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-17T17:17:12.81Z", a.PostedAt)
	}
}

func TestCrelateCompanyFallsBackToConfig(t *testing.T) {
	jobs, err := NewCrelate(newCrelateFake()).Fetch(context.Background(), CompanyEntry{
		Company: "Career Tree Network", Board: crelateBoard,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[1].Company != "Career Tree Network" {
		t.Errorf("Company = %q, want config fallback when posting omits CompanyName", jobs[1].Company)
	}
	// The second posting has no City/State — Country is the location fallback.
	if jobs[1].Location != "United States" {
		t.Errorf("Location = %q, want country fallback", jobs[1].Location)
	}
}

func TestCrelateDropsJobWithNoID(t *testing.T) {
	fake := (&routedHTTP{}).route("GetAllJobs", `{"Jobs":[{"Title":"No ID"}],"IsError":false}`)
	jobs, err := NewCrelate(fake).Fetch(context.Background(), CompanyEntry{Company: "C", Board: crelateBoard})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (no-id dropped)", len(jobs))
	}
}

func TestCrelateIsErrorFails(t *testing.T) {
	fake := (&routedHTTP{}).route("GetAllJobs", `{"Jobs":[],"IsError":true,"ErrorMessage":"bad org"}`)
	if _, err := NewCrelate(fake).Fetch(context.Background(), CompanyEntry{Board: crelateBoard}); err == nil {
		t.Fatal("expected error when IsError is true")
	}
}

func TestCrelateBoardMustBeSlugColonOrg(t *testing.T) {
	for _, board := range []string{"", "nocolon", ":org", "slug:"} {
		if _, err := NewCrelate(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q: expected error, got nil", board)
		}
	}
}
