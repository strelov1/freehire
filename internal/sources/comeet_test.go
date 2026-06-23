package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// comeetPageHTML mirrors a Comeet careers page: the public read token is embedded in an
// inline script's JSON state.
const comeetPageHTML = `<html><head><title>Overwolf Careers</title></head><body>
<div id="comeet"></div>
<script>window.__data = {"company":{"uid":"B1.001"},"token":"1B16C4BD7005131B1A26F391B1"};</script>
</body></html>`

// comeetPositionsJSON mirrors the careers-api positions payload: a list of positions, each
// with structured location, a workplace_type enum, and details split into named HTML sections.
const comeetPositionsJSON = `[
  {
    "uid": "BC.26F",
    "name": "Data Analyst Lead",
    "company_name": "Overwolf",
    "url_active_page": "https://careers.overwolf.com/career/BC.26F",
    "workplace_type": "Hybrid",
    "time_updated": "2026-06-22T18:23:19Z",
    "location": {"name": "HQ", "country": "IL", "city": "Ramat Gan", "state": "Israel"},
    "details": [
      {"name": "Description", "value": "<p>Lead our data team.</p>", "order": 1},
      {"name": "Requirements", "value": "<ul><li>SQL</li></ul><script>alert(1)</script>", "order": 2}
    ]
  }
]`

func newComeetFake() *routedHTTP {
	return (&routedHTTP{}).
		route("/jobs/overwolf/B1.001", comeetPageHTML).
		route("/careers-api/2.0/company/B1.001/positions", comeetPositionsJSON)
}

func TestComeetProvider(t *testing.T) {
	if got := NewComeet(nil).Provider(); got != "comeet" {
		t.Errorf("Provider() = %q, want %q", got, "comeet")
	}
}

func TestComeetFetchScrapesTokenThenMapsPositions(t *testing.T) {
	jobs, err := NewComeet(newComeetFake()).Fetch(context.Background(), CompanyEntry{
		Company: "Overwolf", Provider: "comeet", Board: "overwolf/B1.001",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "BC.26F" {
		t.Errorf("ExternalID = %q, want BC.26F", j.ExternalID)
	}
	if j.Title != "Data Analyst Lead" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Overwolf" {
		t.Errorf("Company = %q, want config company", j.Company)
	}
	if j.URL != "https://careers.overwolf.com/career/BC.26F" {
		t.Errorf("URL = %q, want the active careers page", j.URL)
	}
	if j.Location != "Ramat Gan, Israel" {
		t.Errorf("Location = %q, want \"Ramat Gan, Israel\"", j.Location)
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true, want false for a hybrid posting")
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Lead our data team.") || !strings.Contains(j.Description, "<li>SQL</li>") {
		t.Errorf("Description missing section content: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Requirements") {
		t.Errorf("Description should label non-intro sections: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 22, 18, 23, 19, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-22T18:23:19Z", j.PostedAt)
	}
}

func TestComeetLocation(t *testing.T) {
	cases := []struct {
		loc  comeetLocation
		want string
	}{
		{comeetLocation{City: "Ramat Gan", State: "Israel"}, "Ramat Gan, Israel"},
		{comeetLocation{City: "Berlin"}, "Berlin"},
		{comeetLocation{Country: "DE"}, "DE"},
		{comeetLocation{Name: "Remote"}, "Remote"},
		{comeetLocation{}, ""},
	}
	for _, c := range cases {
		if got := c.loc.String(); got != c.want {
			t.Errorf("comeetLocation%+v = %q, want %q", c.loc, got, c.want)
		}
	}
}

func TestComeetMissingTokenErrors(t *testing.T) {
	fake := (&routedHTTP{}).route("/jobs/overwolf/B1.001", `<html><body>no token here</body></html>`)
	_, err := NewComeet(fake).Fetch(context.Background(), CompanyEntry{Board: "overwolf/B1.001"})
	if err == nil {
		t.Fatal("expected error when the page exposes no token")
	}
}

func TestComeetDropsPositionWithNoUID(t *testing.T) {
	page := `<html><body><script>{"token":"ABCDEF1234567890ABCDEF12"}</script></body></html>`
	positions := `[{"name":"No UID","workplace_type":"On-site"}]`
	fake := (&routedHTTP{}).
		route("/jobs/x/UID.1", page).
		route("/careers-api/2.0/company/UID.1/positions", positions)
	jobs, err := NewComeet(fake).Fetch(context.Background(), CompanyEntry{Board: "x/UID.1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (no-uid dropped)", len(jobs))
	}
}

func TestComeetRemotePositionSetsRemote(t *testing.T) {
	page := `<html><body><script>{"token":"ABCDEF1234567890ABCDEF12"}</script></body></html>`
	positions := `[{"uid":"R1.A0","name":"SRE","workplace_type":"Remote","details":[{"name":"Description","value":"<p>x</p>"}]}]`
	fake := (&routedHTTP{}).
		route("/jobs/x/UID.1", page).
		route("/careers-api/2.0/company/UID.1/positions", positions)
	jobs, err := NewComeet(fake).Fetch(context.Background(), CompanyEntry{Board: "x/UID.1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].WorkMode != "remote" || !jobs[0].Remote {
		t.Fatalf("remote mapping failed: %+v", jobs)
	}
}

func TestComeetBoardWithoutUIDErrors(t *testing.T) {
	_, err := NewComeet(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "noslug"})
	if err == nil {
		t.Fatal("expected error for a board without a /companyUID segment")
	}
}
