package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkableMarketplaceProvider(t *testing.T) {
	if got := NewWorkableMarketplace(nil).Provider(); got != "workablemarketplace" {
		t.Errorf("Provider() = %q, want %q", got, "workablemarketplace")
	}
}

func TestWorkableMarketplaceFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"totalSize": 2,
		"jobs": [
			{
				"id": "ee3feec0-74d5-46f1-9996-5ab064f75050",
				"title": "Senior Backend Engineer",
				"state": "published",
				"employmentType": "Contract",
				"workplace": "remote",
				"url": "https://jobs.workable.com/view/vqncebKcuVxZHgabGqKWHW/remote-senior-backend",
				"created": "2026-01-08T15:25:12.375Z",
				"location": {"city": "Buenos Aires", "subregion": "Buenos Aires", "countryName": "Argentina"},
				"description": "<p>Build <strong>things</strong>.</p><script>x()</script>"
			},
			{
				"id": "draft-1",
				"title": "Draft Role",
				"state": "draft",
				"workplace": "onsite",
				"url": "https://jobs.workable.com/view/draft/x",
				"created": "2026-01-01T00:00:00.000Z",
				"location": {"city": "", "countryName": "Brazil"},
				"description": "<p>hidden</p>"
			}
		],
		"nextPageToken": ""
	}`}

	jobs, err := NewWorkableMarketplace(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Intuition Machines", Provider: "workablemarketplace", Board: "ix7jhWd4JnugmXDK5RaQSZ",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "companies/ix7jhWd4JnugmXDK5RaQSZ") {
		t.Errorf("requested URL %q should target the company hashid", fake.gotURL)
	}
	// The draft posting is dropped: only state=="published" jobs are ingested.
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (draft dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "ee3feec0-74d5-46f1-9996-5ab064f75050" {
		t.Errorf("ExternalID = %q, want the uuid id", j.ExternalID)
	}
	if j.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.workable.com/view/vqncebKcuVxZHgabGqKWHW/remote-senior-backend" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Intuition Machines" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Buenos Aires, Argentina" {
		t.Errorf("Location = %q, want city + country joined", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote from workplace", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed 2026 date", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description should be sanitized: %q", j.Description)
	}
}

// pagingHTTP returns a queued body per call and records every requested URL, so the
// pageToken pagination flow can be exercised (fakeHTTP returns one body for any URL).
type pagingHTTP struct {
	bodies []string
	urls   []string
	i      int
}

func (p *pagingHTTP) GetJSON(_ context.Context, url string, v any) error {
	p.urls = append(p.urls, url)
	body := p.bodies[p.i]
	if p.i < len(p.bodies)-1 {
		p.i++
	}
	return json.Unmarshal([]byte(body), v)
}

func TestWorkableMarketplacePagination(t *testing.T) {
	page1 := `{"jobs":[{"id":"a","title":"A","state":"published","workplace":"remote","url":"u/a","location":{"countryName":"US"}}],"nextPageToken":"TOK2"}`
	page2 := `{"jobs":[{"id":"b","title":"B","state":"published","workplace":"remote","url":"u/b","location":{"countryName":"US"}}],"nextPageToken":""}`
	fake := &pagingHTTP{bodies: []string{page1, page2}}

	jobs, err := NewWorkableMarketplace(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workablemarketplace", Board: "HASH",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across two pages", len(jobs))
	}
	if len(fake.urls) < 2 || !strings.Contains(fake.urls[1], "pageToken=TOK2") {
		t.Errorf("second request %v should carry pageToken from page 1", fake.urls)
	}
}
