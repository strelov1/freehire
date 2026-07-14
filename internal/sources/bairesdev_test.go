package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// bairesDevFake routes the calls the adapter makes: GetText → the talent listing HTML, GetJSON →
// the JobPosting endpoint (keyed by JobPostingId, for meta) and the Job endpoint (keyed by
// JobOfferId, for the full description).
type bairesDevFake struct {
	listing string
	jobs    map[string]string // JobPostingId → JSON-LD body
	details map[string]string // JobOfferId → Job body (full description)
}

func (f *bairesDevFake) GetText(context.Context, string) (string, error) {
	return f.listing, nil
}

func (f *bairesDevFake) GetJSON(_ context.Context, url string, v any) error {
	for id, body := range f.jobs {
		if strings.Contains(url, "JobPostingId="+id) {
			return json.Unmarshal([]byte(body), v)
		}
	}
	for id, body := range f.details {
		if strings.Contains(url, "JobOfferId="+id) {
			return json.Unmarshal([]byte(body), v)
		}
	}
	return fmt.Errorf("bairesDevFake: no route for %s", url)
}

// bairesDevListingHTML wraps a widget config (built from the given apply URLs) in the one attribute
// the adapter reads.
func bairesDevListingHTML(applyURLs ...string) string {
	rows := make([]string, len(applyURLs))
	for i, u := range applyURLs {
		rows[i] = fmt.Sprintf(`{"page_item_url":%q}`, u)
	}
	cfg := `{"jobList":[` + strings.Join(rows, ",") + `]}`
	b64 := base64.StdEncoding.EncodeToString([]byte(cfg))
	return `<div data-widget-config="` + b64 + `"></div>`
}

func TestBairesDevCrawlsListingDedupsAndHydrates(t *testing.T) {
	// The same posting 284579 is listed twice (two locations under one id) plus a distinct 176816.
	fake := &bairesDevFake{
		listing: bairesDevListingHTML(
			"https://applicants.bairesdev.com/job/97/284579/apply",
			"https://applicants.bairesdev.com/job/97/284579/apply",
			"https://applicants.bairesdev.com/job/111/176816/apply",
		),
		jobs: map[string]string{
			"284579": `{"@type":"JobPosting","title":"Sales Director - Remote Work",` +
				`"description":"Short teaser.","datePosted":"2025-06-02T10:23:21.503",` +
				`"jobLocationType":"TELECOMMUTE","hiringOrganization":{"name":"BairesDev"}}`,
			"176816": `{"@type":"JobPosting","title":"Node Developer",` +
				`"description":"Build it.","datePosted":"2022-07-28T00:00:00",` +
				`"jobLocationType":"TELECOMMUTE","hiringOrganization":{"name":"BairesDev"}}`,
		},
		// The Job endpoint carries the full HTML description (JobPosting's is a teaser). 284579 has
		// one; 176816 does not (so it falls back to the teaser).
		details: map[string]string{
			"284579": `{"jobResults":[{"description":"<h3>Own it.</h3><p>The full role text.</p>` +
				`<script>evil()</script>"}]}`,
		},
	}

	jobs, err := NewBairesDev(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (284579 deduped, 176816)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	sd, ok := byID["284579"]
	if !ok {
		t.Fatal("missing job 284579")
	}
	if sd.Title != "Sales Director - Remote Work" {
		t.Errorf("Title = %q", sd.Title)
	}
	if sd.Company != "BairesDev" {
		t.Errorf("Company = %q", sd.Company)
	}
	if sd.URL != "https://applicants.bairesdev.com/job/97/284579/apply" {
		t.Errorf("URL = %q, want canonical apply link (matches linksource identity)", sd.URL)
	}
	if !sd.Remote || sd.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote for TELECOMMUTE", sd.Remote, sd.WorkMode)
	}
	// The FULL description (Job endpoint) is used and sanitized, not the JobPosting teaser.
	if strings.Contains(sd.Description, "<script>") || !strings.Contains(sd.Description, "The full role text.") {
		t.Errorf("Description not the sanitized full text: %q", sd.Description)
	}
	if strings.Contains(sd.Description, "Short teaser.") {
		t.Errorf("Description used the JobPosting teaser instead of the full Job text: %q", sd.Description)
	}
	// 176816 has no Job-endpoint detail → falls back to its JobPosting teaser.
	if nd := byID["176816"]; !strings.Contains(nd.Description, "Build it.") {
		t.Errorf("176816 Description = %q, want teaser fallback", nd.Description)
	}
	// datePosted has no timezone → date-only fallback.
	if sd.PostedAt == nil || !sd.PostedAt.Equal(time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-06-02", sd.PostedAt)
	}
}

func TestBairesDevSkipsPostingThatNoLongerResolves(t *testing.T) {
	// The listing references 999999, but the endpoint returns an empty body (id gone) → skipped.
	fake := &bairesDevFake{
		listing: bairesDevListingHTML("https://applicants.bairesdev.com/job/97/999999/apply"),
		jobs:    map[string]string{"999999": `{}`},
	}
	jobs, err := NewBairesDev(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (stale id skipped)", len(jobs))
	}
}

func TestBairesDevErrorsWhenNoWidget(t *testing.T) {
	fake := &bairesDevFake{listing: "<html>no widget here</html>"}
	if _, err := NewBairesDev(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when the talent listing has no job widget")
	}
}
