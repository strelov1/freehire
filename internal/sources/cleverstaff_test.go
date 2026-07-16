package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// cleverstaffHTTP is a route-aware test JSONGetter. CleverStaff has one public endpoint,
// getAllOpenVacancy, scoped to a tenant by the ?alias= query. This fake routes by that alias,
// recording each requested alias so a test can assert the adapter fetched the configured board.
type cleverstaffHTTP struct {
	bodies   map[string]string // response body keyed by requested alias
	fail     bool              // every request fails
	gotAlias []string
}

func (f *cleverstaffHTTP) GetJSON(_ context.Context, raw string, v any) error {
	u, _ := url.Parse(raw)
	alias := u.Query().Get("alias")
	f.gotAlias = append(f.gotAlias, alias)
	if f.fail {
		return errors.New("cleverstaffHTTP: boom")
	}
	body, ok := f.bodies[alias]
	if !ok {
		return errors.New("cleverstaffHTTP: no body for alias")
	}
	return json.Unmarshal([]byte(body), v)
}

// oneVacancy is a getAllOpenVacancy payload with a single open vacancy carrying the fields the
// adapter maps.
const oneVacancy = `{
  "status": "ok",
  "orgId": "38ebf20274234344a0972a8f8e5677ca",
  "objects": [
    {
      "vacancyId": "c3fdb13932974023abdf822201f48752",
      "localId": "KIFCf6",
      "position": "Founding Engineer",
      "descr": "<p>Build <b>things</b>.</p>",
      "employmentType": "fullEmployment",
      "workCondition": "remote",
      "status": "inwork",
      "dc": 1774428209001,
      "dm": 1784214264996,
      "clientName": "DOIT Software",
      "industry": "IT"
    }
  ]
}`

func TestCleverstaffProvider(t *testing.T) {
	if got := NewCleverstaff(nil).Provider(); got != "cleverstaff" {
		t.Errorf("Provider() = %q, want %q", got, "cleverstaff")
	}
}

func TestCleverstaffRegisteredAsBoardProvider(t *testing.T) {
	reg := All(nil)
	if _, ok := reg["cleverstaff"]; !ok {
		t.Fatal("cleverstaff not registered in sources.All")
	}
	// A first-party ATS must NOT be an aggregator (else reindex would suppress its postings
	// against ATS twins — backwards, since CleverStaff is the ATS).
	if slices.Contains(AggregatorProviders(reg), "cleverstaff") {
		t.Error("cleverstaff is in AggregatorProviders; a first-party ATS must not be reindex-suppressed")
	}
	// Per-tenant: config validation requires a board (it is not boardless).
	cfg := Config{Provider: "cleverstaff", Sources: []CompanyEntry{{Company: "DOIT Software", Board: ""}}}
	if err := cfg.Validate(reg); err == nil {
		t.Error("Validate accepted a cleverstaff entry with empty board; a per-tenant ATS requires a board")
	}
}

func TestCleverstaffFetchMapsVacancy(t *testing.T) {
	c := &cleverstaffHTTP{bodies: map[string]string{"doit-software1": oneVacancy}}
	src := NewCleverstaff(c)

	jobs, err := src.Fetch(context.Background(), CompanyEntry{Company: "DOIT Software", Board: "doit-software1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "c3fdb13932974023abdf822201f48752" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://cleverstaff.net/i/vacancy-KIFCf6" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Founding Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "DOIT Software" {
		t.Errorf("Company = %q, want configured company", j.Company)
	}
	if !strings.Contains(j.Description, "Build") {
		t.Errorf("Description = %q, want sanitized descr", j.Description)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil {
		t.Errorf("PostedAt = nil, want mapped from dc/dm")
	}
	if want := "doit-software1"; len(c.gotAlias) == 0 || c.gotAlias[0] != want {
		t.Errorf("requested alias %v, want first %q", c.gotAlias, want)
	}
}

func TestCleverstaffFetchNonOKStatusErrors(t *testing.T) {
	c := &cleverstaffHTTP{bodies: map[string]string{"x": `{"status":"error","message":"Service is temporarily unavailable"}`}}
	if _, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Board: "x"}); err == nil {
		t.Fatal("Fetch on non-ok status = nil error, want error so board_health cools the board")
	}
}

func TestCleverstaffFetchTransportErrorErrors(t *testing.T) {
	c := &cleverstaffHTTP{fail: true}
	if _, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Board: "x"}); err == nil {
		t.Fatal("Fetch on transport error = nil error, want error")
	}
}

// mixedVacancies has one good open vacancy plus objects that must be dropped: no vacancyId, no
// localId, no position, and a non-open status.
const mixedVacancies = `{
  "status": "ok",
  "objects": [
    {"vacancyId":"good","localId":"L1","position":"Backend Engineer","status":"inwork","descr":"x"},
    {"vacancyId":"","localId":"L2","position":"No ID","status":"inwork"},
    {"vacancyId":"n2","localId":"","position":"No LocalID","status":"inwork"},
    {"vacancyId":"n3","localId":"L4","position":"","status":"inwork"},
    {"vacancyId":"closed","localId":"L5","position":"Closed Role","status":"closed"}
  ]
}`

func TestCleverstaffFetchDropsUnusableAndClosed(t *testing.T) {
	c := &cleverstaffHTTP{bodies: map[string]string{"x": mixedVacancies}}
	jobs, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "x"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (only the good open vacancy survives)", len(jobs))
	}
	if jobs[0].ExternalID != "good" {
		t.Errorf("surviving job = %q, want the good open vacancy", jobs[0].ExternalID)
	}
}

// hubVacancy carries a clientName distinct from any configured company, to prove hub attribution.
const hubVacancy = `{
  "status": "ok",
  "objects": [
    {"vacancyId":"v1","localId":"L1","position":"QA Engineer","status":"inwork","descr":"x","clientName":"Acme Corp"},
    {"vacancyId":"v2","localId":"L2","position":"Ops Engineer","status":"inwork","descr":"x","clientName":""}
  ]
}`

func TestCleverstaffHubUsesClientName(t *testing.T) {
	c := &cleverstaffHTTP{bodies: map[string]string{"agency": hubVacancy}}
	jobs, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Company: "Some Agency", Board: "agency", Hub: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].Company != "Acme Corp" {
		t.Errorf("hub job[0].Company = %q, want clientName %q", jobs[0].Company, "Acme Corp")
	}
	if jobs[1].Company != "Some Agency" {
		t.Errorf("hub job[1].Company = %q, want fallback to configured company on blank clientName", jobs[1].Company)
	}
}

func TestCleverstaffEmploymentTypeMapping(t *testing.T) {
	// Keys are CleverStaff's real employmentType values (observed live), not guessed ones.
	cases := map[string]string{
		"fullEmployment":  "full_time",
		"underemployment": "part_time",
		"projectWork":     "contract",
		"somethingElse":   "",
	}
	for raw, want := range cases {
		body := `{"status":"ok","objects":[{"vacancyId":"v","localId":"L","position":"P","status":"inwork","descr":"x","employmentType":"` + raw + `"}]}`
		c := &cleverstaffHTTP{bodies: map[string]string{"b": body}}
		jobs, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Company: "Co", Board: "b"})
		if err != nil {
			t.Fatalf("employmentType %q: Fetch: %v", raw, err)
		}
		if len(jobs) != 1 || jobs[0].EmploymentType != want {
			t.Errorf("employmentType %q → %q, want %q", raw, jobs[0].EmploymentType, want)
		}
	}
}

func TestCleverstaffNonHubKeepsConfiguredCompany(t *testing.T) {
	c := &cleverstaffHTTP{bodies: map[string]string{"co": hubVacancy}}
	jobs, err := NewCleverstaff(c).Fetch(context.Background(), CompanyEntry{Company: "DOIT Software", Board: "co"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for i, j := range jobs {
		if j.Company != "DOIT Software" {
			t.Errorf("non-hub job[%d].Company = %q, want configured company regardless of clientName", i, j.Company)
		}
	}
}
