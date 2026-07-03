package sources

import (
	"context"
	"strings"
	"testing"
)

// hireologyAPIJSON is an api.hireology.com/v1/careers/<slug> response (JSON:API): one Open
// posting with the description inline, and one non-Open posting the adapter must drop.
const hireologyAPIJSON = `{"data":[
{"type":"careers","id":"25816","attributes":{"id":25816,"name":"Pet Sitter","remote":false,
 "job-description":"<p>Sit pets.</p><script>x()<\/script>","locations":["Highlands Ranch, CO"],
 "status":"Open","career-site-url":"https://careers.hireology.com/acme/25816/description"}},
{"type":"careers","id":"999","attributes":{"id":999,"name":"Closed Role","status":"Filled","locations":[]}}
]}`

func TestHireologyProvider(t *testing.T) {
	if got := NewHireology(nil).Provider(); got != "hireology" {
		t.Errorf("Provider() = %q, want %q", got, "hireology")
	}
}

func TestHireologyFetch(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/careers/", hireologyAPIJSON)

	jobs, err := NewHireology(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (non-Open posting must be dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "25816" {
		t.Errorf("external_id = %q", j.ExternalID)
	}
	if j.Title != "Pet Sitter" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Location != "Highlands Ranch, CO" {
		t.Errorf("location = %q", j.Location)
	}
	if j.URL != "https://careers.hireology.com/acme/25816/description" {
		t.Errorf("url = %q", j.URL)
	}
	if j.Company != "Acme" {
		t.Errorf("company = %q", j.Company)
	}
	if !strings.Contains(j.Description, "Sit pets") || strings.Contains(j.Description, "x()") {
		t.Errorf("description not sanitized: %q", j.Description)
	}
}
