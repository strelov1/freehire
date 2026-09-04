package sources

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// hibobFake answers the job-ad call, recording the headers it was called with — the Referer is
// load-bearing, not cosmetic (see hibob.go), so a test has to be able to see it.
type hibobFake struct {
	body    string
	url     string
	headers map[string]string
	err     error
}

func (f *hibobFake) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.url, f.headers = url, headers
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

const hibobJobAdBody = `{"filterGroups":{"departments":[]},"jobAdDetails":[
{"id":"04e764ae-fb6c-4303-ba2f-509b917804c2",
 "title":"Data Scientist, Client Facing",
 "department":"Customer",
 "employmentType":"Employee",
 "site":"New York",
 "country":"United States",
 "language":"en",
 "description":"<p>We build AI &amp; finance tooling.</p><script>alert(1)<\/script>",
 "requirements":"<ul><li>4+ years as a data scientist</li></ul>",
 "responsibilities":"<ul><li>Work on-site with clients</li></ul>",
 "benefits":"<ul><li>$140-170k DOE</li></ul>",
 "sectionLabels":{},
 "publishedAt":"2026-04-22T07:20:46.518438210Z",
 "workspaceTypeId":"hybrid",
 "workspaceType":"Hybrid"}]}`

func TestHiBobProvider(t *testing.T) {
	if got := NewHiBob(nil).Provider(); got != "hibob" {
		t.Errorf("Provider() = %q, want hibob", got)
	}
}

func TestHiBobRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["hibob"]; !ok {
		t.Error("All() should register provider hibob")
	}
	if !slices.Contains(FilterableProviders(), "hibob") {
		t.Error("FilterableProviders() should include hibob")
	}
}

// The API answers 401 without a Referer naming the tenant's own careers page — no cookie, no
// token, just that header. Losing it silently empties every board, so it is asserted.
func TestHiBobSendsTenantRefererElseTheAPIRefuses(t *testing.T) {
	fake := &hibobFake{body: hibobJobAdBody}
	if _, err := NewHiBob(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Qogita", Provider: "hibob", Board: "qogita",
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "https://qogita.careers.hibob.com/api/job-ad"; fake.url != want {
		t.Errorf("called %q, want %q", fake.url, want)
	}
	if want := "https://qogita.careers.hibob.com/jobs"; fake.headers["Referer"] != want {
		t.Errorf("Referer = %q, want %q", fake.headers["Referer"], want)
	}
}

func TestHiBobMapsThePosting(t *testing.T) {
	fake := &hibobFake{body: hibobJobAdBody}
	jobs, err := NewHiBob(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Qogita", Provider: "hibob", Board: "qogita",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "04e764ae-fb6c-4303-ba2f-509b917804c2" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if want := "https://qogita.careers.hibob.com/jobs/04e764ae-fb6c-4303-ba2f-509b917804c2"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Title != "Data Scientist, Client Facing" || j.Company != "Qogita" {
		t.Errorf("title/company: %q %q", j.Title, j.Company)
	}
	if j.Location != "New York, United States" {
		t.Errorf("Location = %q, want site and country joined", j.Location)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("WorkMode = %q, Remote = %v", j.WorkMode, j.Remote)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 4, 22, 7, 20, 46, 518438210, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
}

// HiBob splits a posting's body across four fields. All of them are the job ad, so all of them
// have to reach the description — enrichment and skill tagging read that text, and the
// requirements section is where the skills live.
func TestHiBobDescriptionCarriesEverySection(t *testing.T) {
	fake := &hibobFake{body: hibobJobAdBody}
	jobs, err := NewHiBob(fake).Fetch(context.Background(), CompanyEntry{Company: "Qogita", Board: "qogita"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	d := jobs[0].Description
	for _, want := range []string{
		"We build AI", "4+ years as a data scientist", "Work on-site with clients", "$140-170k DOE",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("description is missing %q:\n%s", want, d)
		}
	}
	// Sections must not run together: role_fingerprint hashes the visible text, and glued
	// words ("clients4+ years") would change the hash for no real difference.
	if strings.Contains(d, "clients4+") || strings.Contains(d, "tooling4+") {
		t.Errorf("sections glued together: %q", d)
	}
}

func TestHiBobRemotePostingIsFlagged(t *testing.T) {
	body := strings.ReplaceAll(hibobJobAdBody, `"workspaceType":"Hybrid"`, `"workspaceType":"Remote"`)
	jobs, err := NewHiBob(&hibobFake{body: body}).Fetch(context.Background(), CompanyEntry{Board: "qogita"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !jobs[0].Remote || jobs[0].WorkMode != "remote" {
		t.Errorf("Remote = %v, WorkMode = %q", jobs[0].Remote, jobs[0].WorkMode)
	}
}

func TestHiBobOnSiteMapsToOnsite(t *testing.T) {
	body := strings.ReplaceAll(hibobJobAdBody, `"workspaceType":"Hybrid"`, `"workspaceType":"On site"`)
	jobs, err := NewHiBob(&hibobFake{body: body}).Fetch(context.Background(), CompanyEntry{Board: "qogita"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[0].WorkMode != "onsite" || jobs[0].Remote {
		t.Errorf("WorkMode = %q, Remote = %v", jobs[0].WorkMode, jobs[0].Remote)
	}
}

// A posting with no id has no dedup key, so it would collide with every other id-less posting
// on the board rather than being stored.
func TestHiBobSkipsPostingWithoutID(t *testing.T) {
	body := `{"jobAdDetails":[{"id":"","title":"Ghost","description":"<p>x</p>"}]}`
	jobs, err := NewHiBob(&hibobFake{body: body}).Fetch(context.Background(), CompanyEntry{Board: "qogita"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
