package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// bullhornPage1JSON mirrors a search/JobOrder envelope: an HTML publicDescription, a structured
// address, dateAdded as epoch milliseconds, and a per-customer employmentType label. total
// exceeds the page's data length so the adapter pages again.
const bullhornPage1JSON = `{
  "total": 1000,
  "start": 0,
  "count": 500,
  "data": [
    {
      "id": 4231,
      "title": "Senior Go Engineer",
      "publicDescription": "<p>Build backends.</p><script>alert(1)</script>",
      "address": {"city": "Austin", "state": "TX", "countryName": "United States"},
      "dateAdded": 1750617799000,
      "employmentType": "Contract To Hire"
    }
  ]
}`

// bullhornPage2JSON is the next window: empty data ends pagination.
const bullhornPage2JSON = `{"total": 1000, "start": 500, "count": 500, "data": []}`

func TestBullhornProvider(t *testing.T) {
	if got := NewBullhorn(nil).Provider(); got != "bullhorn" {
		t.Errorf("Provider() = %q, want %q", got, "bullhorn")
	}
}

func TestBullhornFetchMapsJobOrders(t *testing.T) {
	fake := (&routedHTTP{}).
		route("start=0", bullhornPage1JSON).
		route("start=500", bullhornPage2JSON)

	jobs, err := NewBullhorn(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme Staffing", Provider: "bullhorn", Board: "91:abc123def",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want 2 (paged until empty window)", fake.calls)
	}
	j := jobs[0]
	if j.ExternalID != "4231" {
		t.Errorf("ExternalID = %q, want 4231", j.ExternalID)
	}
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme Staffing" {
		t.Errorf("Company = %q, want config company", j.Company)
	}
	wantURL := "https://public-rest91.bullhornstaffing.com/rest-services/abc123def/entity/JobOrder/4231"
	if j.URL != wantURL {
		t.Errorf("URL = %q, want %q", j.URL, wantURL)
	}
	if j.Location != "Austin, TX" {
		t.Errorf("Location = %q, want \"Austin, TX\"", j.Location)
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build backends.") {
		t.Errorf("Description missing body: %q", j.Description)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j.EmploymentType)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2025, 6, 22, 18, 43, 19, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-06-22T18:43:19Z", j.PostedAt)
	}
}

func TestBullhornBreaksOnFullResult(t *testing.T) {
	// total equals the window size, so the adapter must stop after one call.
	single := `{"total": 1, "start": 0, "count": 500, "data": [
		{"id": 7, "title": "X", "publicDescription": "<p>y</p>", "address": {"city": "Remote"}, "dateAdded": 0, "employmentType": ""}
	]}`
	fake := (&routedHTTP{}).route("start=0", single)
	jobs, err := NewBullhorn(fake).Fetch(context.Background(), CompanyEntry{Company: "C", Board: "1:tok"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || fake.calls != 1 {
		t.Fatalf("jobs=%d calls=%d, want 1 job in 1 call", len(jobs), fake.calls)
	}
	if jobs[0].PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil for dateAdded 0", jobs[0].PostedAt)
	}
	if jobs[0].Location != "Remote" {
		t.Errorf("Location = %q, want Remote (city fallback)", jobs[0].Location)
	}
}

func TestBullhornDropsJobOrderWithNoID(t *testing.T) {
	body := `{"total": 1, "start": 0, "count": 500, "data": [{"title": "No ID"}]}`
	fake := (&routedHTTP{}).route("start=0", body)
	jobs, err := NewBullhorn(fake).Fetch(context.Background(), CompanyEntry{Company: "C", Board: "1:tok"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (no-id dropped)", len(jobs))
	}
}

func TestBullhornBoardMustBeClsColonToken(t *testing.T) {
	for _, board := range []string{"nocolon", ":tok", "91:"} {
		if _, err := NewBullhorn(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q: expected error, got nil", board)
		}
	}
}

func TestBullhornEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Contract To Hire": "contract",
		"Temporary":        "contract",
		"Permanent":        "full_time",
		"Direct Hire":      "full_time",
		"Full-Time":        "full_time",
		"Part-Time":        "part_time",
		"Internship":       "internship",
		"Variable Hour":    "",
		"":                 "",
	}
	for in, want := range cases {
		if got := bullhornEmploymentType(in); got != want {
			t.Errorf("bullhornEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
