package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// cornerstoneHomeHTML mirrors a CSOD careersite home shell: the JSON config blob embeds the
// regional API base ("cloud") and a JWT read token used as the Bearer credential.
const cornerstoneHomeHTML = `<html><head></head><body>
<script>window.__cfg = {"applicationBase":"/","endpoints":{"cloud":"https://eu-fra.api.csod.com/","api":"/"},"token":"eyJhbGciTESTtoken123","cultureID":1,"cultureName":"en-US"};</script>
</body></html>`

// cornerstoneJobsJSON mirrors the rec-job-search/external/jobs response: data.requisitions
// carries the postings with the description inline (no per-job detail fetch needed).
const cornerstoneJobsJSON = `{
  "status": "200",
  "data": {
    "totalCount": 1,
    "requisitions": [
      {
        "requisitionId": 494,
        "postingEffectiveDate": "6/15/2026",
        "displayJobTitle": "European Operations Administrator",
        "locations": [{"city": "Frankfurt am Main", "country": "DE"}],
        "externalDescription": "<p>Coordinate shipments.</p><script>alert(1)</script>"
      }
    ]
  }
}`

func newCornerstoneFake() *routedHTTP {
	return (&routedHTTP{}).
		route("/ux/ats/careersite/1/home", cornerstoneHomeHTML).
		route("rec-job-search/external/jobs", cornerstoneJobsJSON)
}

func TestCornerstoneProvider(t *testing.T) {
	if got := NewCornerstone(nil).Provider(); got != "cornerstone" {
		t.Errorf("Provider() = %q, want %q", got, "cornerstone")
	}
}

func TestCornerstoneFetchScrapesTokenThenMapsRequisitions(t *testing.T) {
	jobs, err := NewCornerstone(newCornerstoneFake()).Fetch(context.Background(), CompanyEntry{
		Company: "Nintendo of Europe", Provider: "cornerstone", Board: "nintendoeurope",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "494" {
		t.Errorf("ExternalID = %q, want 494", j.ExternalID)
	}
	if j.Title != "European Operations Administrator" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Nintendo of Europe" {
		t.Errorf("Company = %q, want config company", j.Company)
	}
	if j.Location != "Frankfurt am Main, DE" {
		t.Errorf("Location = %q", j.Location)
	}
	want := "https://nintendoeurope.csod.com/ux/ats/careersite/1/home/requisition/494?c=nintendoeurope"
	if j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "Coordinate shipments.") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-15", j.PostedAt)
	}
}

func TestCornerstoneLocationString(t *testing.T) {
	cases := []struct {
		locs []cornerstoneLocation
		want string
	}{
		{[]cornerstoneLocation{{City: "Frankfurt am Main", Country: "DE"}}, "Frankfurt am Main, DE"},
		{[]cornerstoneLocation{{City: "Berlin"}}, "Berlin"},
		{[]cornerstoneLocation{{Country: "US"}}, "US"},
		{[]cornerstoneLocation{{City: "Tokyo"}, {City: "Osaka"}}, "Tokyo"}, // first location only
		{nil, ""},
	}
	for _, c := range cases {
		if got := cornerstonePrimaryLocation(c.locs); got != c.want {
			t.Errorf("cornerstonePrimaryLocation(%v) = %q, want %q", c.locs, got, c.want)
		}
	}
}

// cornerstonePagedHTTP serves the home page, then one requisition per search page up to
// total — exercising the pagination-continues branch and totalCount termination.
type cornerstonePagedHTTP struct {
	total int
	calls int
}

func (h *cornerstonePagedHTTP) GetText(context.Context, string) (string, error) {
	return cornerstoneHomeHTML, nil
}

func (h *cornerstonePagedHTTP) PostJSONWithHeaders(_ context.Context, _ string, _ map[string]string, _, v any) error {
	h.calls++
	reqs := ""
	if h.calls <= h.total {
		reqs = fmt.Sprintf(`{"requisitionId":%d,"displayJobTitle":"Job %d","externalDescription":"<p>x</p>"}`, h.calls, h.calls)
	}
	body := fmt.Sprintf(`{"data":{"totalCount":%d,"requisitions":[%s]}}`, h.total, reqs)
	return json.Unmarshal([]byte(body), v)
}

func TestCornerstonePaginatesUntilTotalCount(t *testing.T) {
	fake := &cornerstonePagedHTTP{total: 3}
	jobs, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "tenant"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 across pages", len(jobs))
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		seen[j.ExternalID] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 distinct requisitions across pages, got ids %v", seen)
	}
}

func TestCornerstoneMissingTokenErrors(t *testing.T) {
	fake := (&routedHTTP{}).route("/ux/ats/careersite/1/home", `<html><body>no config blob</body></html>`)
	_, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "nintendoeurope"})
	if err == nil {
		t.Fatal("expected error when the home page exposes no token/cloud endpoint")
	}
}

func TestCornerstoneDropsRequisitionWithNoID(t *testing.T) {
	jobs := `{"data":{"totalCount":1,"requisitions":[{"displayJobTitle":"No ID","externalDescription":"<p>x</p>"}]}}`
	fake := (&routedHTTP{}).
		route("/ux/ats/careersite/1/home", cornerstoneHomeHTML).
		route("rec-job-search/external/jobs", jobs)
	got, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "nintendoeurope"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d jobs, want 0 (requisition with no id dropped)", len(got))
	}
}

// cornerstoneSitesHTTP serves the home page and answers each search by the careerSiteId in
// the request body, so a tenant can spread its postings over several career sites.
type cornerstoneSitesHTTP struct {
	sites map[int]string // careerSiteId → requisitions JSON array
	fail  map[int]bool   // careerSiteId → search fails
}

func (h *cornerstoneSitesHTTP) GetText(context.Context, string) (string, error) {
	return cornerstoneHomeHTML, nil
}

func (h *cornerstoneSitesHTTP) PostJSONWithHeaders(_ context.Context, _ string, _ map[string]string, body, v any) error {
	site := body.(map[string]any)["careerSiteId"].(int)
	if h.fail[site] {
		return fmt.Errorf("site %d unavailable", site)
	}
	reqs := h.sites[site]
	n := strings.Count(reqs, "requisitionId")
	return json.Unmarshal([]byte(fmt.Sprintf(`{"data":{"totalCount":%d,"requisitions":[%s]}}`, n, reqs)), v)
}

// Krka keeps nothing on career site 1 and one site per country from 13 on; a requisition
// listed on two sites is one posting, linked from the first site that lists it.
func TestCornerstoneCollectsEveryCareerSite(t *testing.T) {
	fake := &cornerstoneSitesHTTP{sites: map[int]string{
		13: `{"requisitionId":9171,"displayJobTitle":"Slovenia"}`,
		14: `{"requisitionId":9115,"displayJobTitle":"Poland"},{"requisitionId":9171,"displayJobTitle":"Slovenia"}`,
	}}
	jobs, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "krka"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	urls := map[string]string{}
	for _, j := range jobs {
		urls[j.ExternalID] = j.URL
	}
	want := map[string]string{
		"9171": "https://krka.csod.com/ux/ats/careersite/13/home/requisition/9171?c=krka",
		"9115": "https://krka.csod.com/ux/ats/careersite/14/home/requisition/9115?c=krka",
	}
	if len(jobs) != len(want) {
		t.Fatalf("got %d jobs (%v), want %d", len(jobs), urls, len(want))
	}
	for id, u := range want {
		if urls[id] != u {
			t.Errorf("URL for %s = %q, want %q", id, urls[id], u)
		}
	}
}

func TestCornerstoneSkipsALaterCareerSiteThatFails(t *testing.T) {
	fake := &cornerstoneSitesHTTP{
		sites: map[int]string{1: `{"requisitionId":1,"displayJobTitle":"A"}`, 3: `{"requisitionId":3,"displayJobTitle":"C"}`},
		fail:  map[int]bool{2: true},
	}
	jobs, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "tenant"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (site 2 skipped)", len(jobs))
	}
}

func TestCornerstoneFirstSearchFailingFailsTheBoard(t *testing.T) {
	fake := &cornerstoneSitesHTTP{fail: map[int]bool{1: true}}
	if _, err := NewCornerstone(fake).Fetch(context.Background(), CompanyEntry{Board: "tenant"}); err == nil {
		t.Fatal("expected an error when the very first search fails")
	}
}
