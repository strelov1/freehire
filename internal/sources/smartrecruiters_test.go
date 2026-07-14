package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// routedHTTP is a test HTTPClient that returns a different canned body per URL,
// matching the first route whose substring is contained in the requested URL. It is
// concurrency-safe so the SmartRecruiters adapter can fan out detail fetches.
type routedHTTP struct {
	routes []struct{ match, body string }
	mu     sync.Mutex
	calls  int
}

func (r *routedHTTP) route(match, body string) *routedHTTP {
	r.routes = append(r.routes, struct{ match, body string }{match, body})
	return r
}

func (r *routedHTTP) GetJSON(_ context.Context, url string, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetXML(_ context.Context, url string, v any) error {
	return r.decode(url, xml.Unmarshal, v)
}

func (r *routedHTTP) PostJSON(_ context.Context, url string, _, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetJSONWithHeaders(_ context.Context, url string, _ map[string]string, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) PostJSONWithHeaders(_ context.Context, url string, _ map[string]string, _, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return html.Parse(strings.NewReader(rt.body))
		}
	}
	return nil, fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) GetText(_ context.Context, url string) (string, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return rt.body, nil
		}
	}
	return "", fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) decode(url string, unmarshal func([]byte, any) error, v any) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return unmarshal([]byte(rt.body), v)
		}
	}
	return fmt.Errorf("routedHTTP: no route for %s", url)
}

func detailBody(id, title string) string {
	return fmt.Sprintf(`{
		"id": %q,
		"postingUrl": "https://jobs.smartrecruiters.com/Acme/%s",
		"experienceLevel": {"id": "mid_senior_level", "label": "Mid-Senior Level"},
		"typeOfEmployment": {"id": "permanent", "label": "Full-time"},
		"jobAd": {"sections": {
			"companyDescription": {"title": "Company", "text": "<p>boilerplate</p>"},
			"jobDescription": {"title": "Job", "text": "<p>%s do the job.</p>"},
			"qualifications": {"title": "Qualifications", "text": "<ul><li>Go</li></ul>"},
			"additionalInformation": {"title": "More", "text": "<p>EEO notice.</p>"}
		}}
	}`, id, id, title)
}

func TestSmartRecruitersProvider(t *testing.T) {
	if got := NewSmartRecruiters(nil).Provider(); got != "smartrecruiters" {
		t.Errorf("Provider() = %q, want %q", got, "smartrecruiters")
	}
}

// The SmartRecruiters experienceLevel enum maps onto freehire's seniority vocabulary.
// Ambiguous/unset values (not_applicable, unknown) map to "" so the title dictionary
// decides instead — structured signal only, never a guess.
func TestSmartRecruitersSeniority(t *testing.T) {
	cases := map[string]string{
		"internship":       "intern",
		"entry_level":      "junior",
		"associate":        "middle",
		"mid_senior_level": "senior",
		"director":         "lead",
		"executive":        "c_level",
		"not_applicable":   "",
		"":                 "",
		"something_new":    "",
	}
	for in, want := range cases {
		if got := smartRecruitersSeniority(in); got != want {
			t.Errorf("smartRecruitersSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

// The SmartRecruiters typeOfEmployment enum maps onto freehire's employment-type
// vocabulary; unknown/absent ids map to "" so the description parser decides.
func TestSmartRecruitersEmploymentType(t *testing.T) {
	cases := map[string]string{
		"permanent":   "full_time",
		"full-time":   "full_time",
		"part-time":   "part_time",
		"contract":    "contract",
		"temporary":   "contract",
		"internship":  "internship",
		"traineeship": "internship",
		"":            "",
		"whatever":    "",
	}
	for in, want := range cases {
		if got := smartRecruitersEmploymentType(in); got != want {
			t.Errorf("smartRecruitersEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSmartRecruitersFetchPaginatesAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 3, "content": [
			{"id": "P1", "name": "Backend Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Berlin", "region": "", "country": "de", "remote": true}},
			{"id": "P2", "name": "Frontend Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Remote", "country": "us", "remote": true}}
		]}`).
		route("offset=2", `{"totalFound": 3, "content": [
			{"id": "P3", "name": "Data Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "NYC", "country": "us", "remote": false}}
		]}`).
		route("/postings/P1", detailBody("P1", "P1")).
		route("/postings/P2", detailBody("P2", "P2")).
		route("/postings/P3", detailBody("P3", "P3"))

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["P1"]
	if !ok {
		t.Fatal("posting P1 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.smartrecruiters.com/Acme/P1" {
		t.Errorf("URL = %q, want the postingUrl from detail", j.URL)
	}
	if j.Location != "Berlin, de" {
		t.Errorf("Location = %q, want city/country joined", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from location.remote")
	}
	for _, want := range []string{"do the job.", "Go", "EEO notice."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "boilerplate") {
		t.Errorf("Description should exclude companyDescription, got %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed releasedDate (2024)", j.PostedAt)
	}
	if j.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior (mapped from experienceLevel mid_senior_level)", j.Seniority)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (mapped from typeOfEmployment permanent)", j.EmploymentType)
	}
}

func TestSmartRecruitersFetchSkipsFailedDetail(t *testing.T) {
	// P2 has no detail route -> its detail fetch errors and the posting is skipped,
	// but P1 still comes through.
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 2, "content": [
			{"id": "P1", "name": "Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Berlin", "country": "de", "remote": false}},
			{"id": "P2", "name": "Broken", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "NYC", "country": "us", "remote": false}}
		]}`).
		route("/postings/P1", detailBody("P1", "P1"))

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "P1" {
		t.Fatalf("want only P1 to survive, got %d jobs", len(jobs))
	}
}
