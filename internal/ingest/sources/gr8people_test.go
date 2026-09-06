package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// gr8peopleJobsHTML mirrors the tenant's /jobs page: a Next.js __NEXT_DATA__ blob embeds the
// anonymous session token the GraphQL API requires as a Bearer credential.
const gr8peopleJobsHTML = `<html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"identity":{"token":"eyJhbGciTESTtoken123"}}}}</script></body></html>`

// gr8peopleFake is a test transport: GetText always answers the /jobs shell above; the GraphQL
// POST is routed by the "after" cursor in the request's variables, so a page transition can be
// exercised the same way dayforce/jobappnetwork's fakes do.
type gr8peopleFake struct {
	pages    map[string]string // "after" cursor ("" for the first page) -> graphql response JSON
	postFail bool
	headers  []map[string]string
	bodies   []map[string]any
}

func (f *gr8peopleFake) GetText(context.Context, string) (string, error) {
	return gr8peopleJobsHTML, nil
}

func (f *gr8peopleFake) PostJSONWithHeaders(_ context.Context, _ string, headers map[string]string, body, v any) error {
	if f.postFail {
		return errors.New("gr8peopleFake: boom")
	}
	f.headers = append(f.headers, headers)
	b, _ := body.(map[string]any)
	f.bodies = append(f.bodies, b)
	variables, _ := b["variables"].(map[string]any)
	after, _ := variables["after"].(string)
	raw, ok := f.pages[after]
	if !ok {
		raw = `{"data":{"searchJobs":{"results":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""},"totalCount":0}}}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestGr8PeopleProvider(t *testing.T) {
	if got := NewGr8People(nil).Provider(); got != "gr8people" {
		t.Errorf("Provider() = %q, want %q", got, "gr8people")
	}
}

func TestGr8PeopleFetchMintsTokenAndMapsPosting(t *testing.T) {
	fake := &gr8peopleFake{pages: map[string]string{
		"": `{"data":{"searchJobs":{"results":{"nodes":[
			{"key":"4709","title":"FSR - Chicago, IL","descriptionHTML":"<p>Serve customers.</p><script>x()<\/script>","workplaceType":"ON_SITE","postedOn":"2026-03-18T19:59:49.013Z","positionType":{"name":"Full Time"},"primaryPlace":{"name":"Chicago, IL, USA"},"places":{"nodes":[{"name":"Chicago, IL, USA"}]}}
		],"pageInfo":{"hasNextPage":false,"endCursor":""},"totalCount":1}}}}`,
	}}

	jobs, err := NewGr8People(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Morgan Stanley E-TRADE", Board: "etrade.gr8people.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "4709" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "FSR - Chicago, IL" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Morgan Stanley E-TRADE" {
		t.Errorf("Company = %q, want the board's configured company", j.Company)
	}
	if j.Location != "Chicago, IL, USA" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want onsite/false", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt not parsed")
	}
	if !strings.Contains(j.Description, "Serve customers.") {
		t.Errorf("Description = %q, want it to contain the body", j.Description)
	}
	if strings.Contains(j.Description, "x()") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}

	// The token scraped from the /jobs shell must ride every search request as a Bearer header.
	if len(fake.headers) != 1 || fake.headers[0]["Authorization"] != "Bearer eyJhbGciTESTtoken123" {
		t.Errorf("Authorization header = %v, want Bearer eyJhbGciTESTtoken123", fake.headers)
	}
}

func TestGr8PeopleWorkModeMapping(t *testing.T) {
	cases := []struct {
		workplaceType string
		wantMode      string
		wantRemote    bool
	}{
		{"REMOTE", "remote", true},
		{"HYBRID", "hybrid", false},
		{"ON_SITE", "onsite", false},
		{"", "", false},
	}
	for _, c := range cases {
		fake := &gr8peopleFake{pages: map[string]string{
			"": `{"data":{"searchJobs":{"results":{"nodes":[
				{"key":"1","title":"x","descriptionHTML":"d","workplaceType":"` + c.workplaceType + `"}
			],"pageInfo":{"hasNextPage":false,"endCursor":""},"totalCount":1}}}}`,
		}}
		jobs, err := NewGr8People(fake).Fetch(context.Background(), CompanyEntry{Board: "t.gr8people.com"})
		if err != nil {
			t.Fatalf("Fetch(%q): %v", c.workplaceType, err)
		}
		if jobs[0].WorkMode != c.wantMode || jobs[0].Remote != c.wantRemote {
			t.Errorf("workplaceType=%q: WorkMode/Remote = %q/%v, want %q/%v",
				c.workplaceType, jobs[0].WorkMode, jobs[0].Remote, c.wantMode, c.wantRemote)
		}
	}
}

func TestGr8PeopleFetchPaginatesViaCursor(t *testing.T) {
	fake := &gr8peopleFake{pages: map[string]string{
		"": `{"data":{"searchJobs":{"results":{"nodes":[
			{"key":"1","title":"One","descriptionHTML":"d"}
		],"pageInfo":{"hasNextPage":true,"endCursor":"CURSOR2"},"totalCount":2}}}}`,
		"CURSOR2": `{"data":{"searchJobs":{"results":{"nodes":[
			{"key":"2","title":"Two","descriptionHTML":"d"}
		],"pageInfo":{"hasNextPage":false,"endCursor":""},"totalCount":2}}}}`,
	}}
	jobs, err := NewGr8People(fake).Fetch(context.Background(), CompanyEntry{Board: "t.gr8people.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 across two cursor pages", len(jobs))
	}
	if len(fake.bodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(fake.bodies))
	}
	firstVars, _ := fake.bodies[0]["variables"].(map[string]any)
	if _, hasAfter := firstVars["after"]; hasAfter {
		t.Errorf("first page must not send an after cursor, got %v", firstVars["after"])
	}
	secondVars, _ := fake.bodies[1]["variables"].(map[string]any)
	if secondVars["after"] != "CURSOR2" {
		t.Errorf("second page after = %v, want CURSOR2", secondVars["after"])
	}
}

func TestGr8PeopleFetchEmptyBoard(t *testing.T) {
	fake := &gr8peopleFake{pages: map[string]string{
		"": `{"data":{"searchJobs":{"results":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""},"totalCount":0}}}}`,
	}}
	jobs, err := NewGr8People(fake).Fetch(context.Background(), CompanyEntry{Board: "t.gr8people.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestGr8PeopleFetchSearchTransportErrorFailsBoard(t *testing.T) {
	fake := &gr8peopleFake{postFail: true}
	if _, err := NewGr8People(fake).Fetch(context.Background(), CompanyEntry{Board: "t.gr8people.com"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestGr8PeopleMissingTokenErrors(t *testing.T) {
	fake := &gr8peopleNoTokenFake{}
	if _, err := NewGr8People(fake).Fetch(context.Background(), CompanyEntry{Board: "t.gr8people.com"}); err == nil {
		t.Fatal("Fetch: want an error when the /jobs shell carries no token")
	}
}

// gr8peopleNoTokenFake serves a /jobs page with no embedded token, simulating a platform
// markup change or an unreachable/misconfigured tenant.
type gr8peopleNoTokenFake struct{}

func (gr8peopleNoTokenFake) GetText(context.Context, string) (string, error) {
	return `<html><body>no config blob</body></html>`, nil
}

func (gr8peopleNoTokenFake) PostJSONWithHeaders(context.Context, string, map[string]string, any, any) error {
	return errors.New("gr8peopleNoTokenFake: search must not be called without a token")
}

func TestGr8PeopleRegisteredInAll(t *testing.T) {
	if _, ok := All(nil)["gr8people"]; !ok {
		t.Fatal(`All(nil)["gr8people"] missing`)
	}
}
