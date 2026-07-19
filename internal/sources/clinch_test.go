package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// routedDetail is a fake HTMLGetter for clinch's detail path: it matches a URL by substring
// and returns either a parsed detail page or a WAF ChallengeError (body "CHALLENGE"). It
// records every URL it was asked to fetch so a test can assert the latch stopped further
// fetches.
type routedDetail struct {
	routes map[string]string
	calls  []string
}

func (r *routedDetail) GetHTML(_ context.Context, url string) (*html.Node, error) {
	r.calls = append(r.calls, url)
	for sub, body := range r.routes {
		if strings.Contains(url, sub) {
			if body == "CHALLENGE" {
				return nil, &ChallengeError{URL: url}
			}
			return html.Parse(strings.NewReader(body))
		}
	}
	return nil, fmt.Errorf("routedDetail: no route for %s", url)
}

func clinchDetailHTML(descInnerHTML string) string {
	return `<html><body><h1>irrelevant</h1>` +
		`<div class="job-description">` + descInnerHTML + `</div>` +
		`</body></html>`
}

func TestClinchDescriptionPreservesBlockStructureAsSanitizedHTML(t *testing.T) {
	// A real posting is multi-block; the description must keep paragraph/list boundaries
	// (stored as sanitized HTML like every other adapter) rather than flatten to run-together
	// text.
	node, err := html.Parse(strings.NewReader(clinchDetailHTML(`<p>First sentence.</p><ul><li>Alpha</li><li>Beta</li></ul><p>Last.</p>`)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := clinchDescription(node)
	want := `<p>First sentence.</p><ul><li>Alpha</li><li>Beta</li></ul><p>Last.</p>`
	if got != want {
		t.Errorf("clinchDescription = %q, want the block structure preserved as sanitized HTML %q", got, want)
	}
}

func TestClinchDescriptionEmptyWhenNoBlock(t *testing.T) {
	node, err := html.Parse(strings.NewReader(`<html><body><p>no description block here</p></body></html>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := clinchDescription(node); got != "" {
		t.Errorf("clinchDescription = %q, want empty", got)
	}
}

func TestClinchFetchHydratesDescriptionFromDetail(t *testing.T) {
	job := "https://careers.withwaymo.com/jobs/software-engineer-mountain-view-california-united-states"
	sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(job))
	detail := &routedDetail{routes: map[string]string{
		"/jobs/software-engineer": clinchDetailHTML("Own the simulator."),
	}}

	jobs, err := NewClinch(sitemap, detail).Fetch(context.Background(), CompanyEntry{
		Company: "Waymo", Board: "careers.withwaymo.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Description != "Own the simulator." {
		t.Errorf("Description = %q, want the detail-page text", jobs[0].Description)
	}
}

func TestClinchFetchChallengeLatchesRemainingToSitemapOnly(t *testing.T) {
	j1 := "https://careers.withwaymo.com/jobs/one-mountain-view-california-united-states"
	j2 := "https://careers.withwaymo.com/jobs/two-london-england-united-kingdom"
	j3 := "https://careers.withwaymo.com/jobs/three-warsaw-masovian-voivodeship-poland"
	sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(j1, j2, j3))
	detail := &routedDetail{routes: map[string]string{
		"/jobs/one-":   clinchDetailHTML("First hydrated."),
		"/jobs/two-":   "CHALLENGE", // WAF trips here
		"/jobs/three-": clinchDetailHTML("Should never be fetched."),
	}}

	jobs, err := NewClinch(sitemap, detail).Fetch(context.Background(), CompanyEntry{Board: "careers.withwaymo.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (all sitemap postings still emitted)", len(jobs))
	}
	if jobs[0].Description != "First hydrated." {
		t.Errorf("job[0].Description = %q, want the hydrated text", jobs[0].Description)
	}
	if jobs[1].Description != "" || jobs[2].Description != "" {
		t.Errorf("after a challenge, remaining descriptions must be empty; got %q, %q", jobs[1].Description, jobs[2].Description)
	}
	// The latch must stop detail fetches: only j1 (ok) and j2 (challenge) are fetched, never j3.
	if len(detail.calls) != 2 {
		t.Errorf("detail fetches = %d (%v), want 2 — the latch must skip j3", len(detail.calls), detail.calls)
	}
}

func TestClinchFetchDetailErrorKeepsPostingWithEmptyDescription(t *testing.T) {
	job := "https://careers.withwaymo.com/jobs/backend-engineer-warsaw-masovian-voivodeship-poland"
	sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(job))
	detail := &routedDetail{routes: map[string]string{}} // no route → per-posting error (not a challenge)

	jobs, err := NewClinch(sitemap, detail).Fetch(context.Background(), CompanyEntry{Board: "careers.withwaymo.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (a detail error must not drop the posting)", len(jobs))
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty on a detail error", jobs[0].Description)
	}
	if jobs[0].Title != "Backend Engineer" {
		t.Errorf("Title = %q, want the slug-derived title", jobs[0].Title)
	}
}

func clinchSitemapXML(locs ...string) string {
	xml := `<?xml version="1.0"?><urlset>`
	for _, l := range locs {
		xml += `<url><loc>` + l + `</loc><lastmod>2026-05-04</lastmod></url>`
	}
	return xml + `</urlset>`
}

func TestClinchProvider(t *testing.T) {
	if got := NewClinch(nil, nil).Provider(); got != "clinch" {
		t.Errorf("Provider() = %q, want %q", got, "clinch")
	}
}

func TestClinchExternalID(t *testing.T) {
	cases := map[string]string{
		// No UUID suffix: the whole slug is the stable id.
		"backend-technical-lead-manager-taas-warsaw-masovian-voivodeship-poland": "backend-technical-lead-manager-taas-warsaw-masovian-voivodeship-poland",
		// Trailing UUID: prefer it — it survives a title edit that reshuffles the slug.
		"machine-learning-engineer-runtime-optimization-mountain-view-california-united-states-846a7827-ae6e-4a38-a368-2606aa465931": "846a7827-ae6e-4a38-a368-2606aa465931",
	}
	for slug, want := range cases {
		if got := clinchExternalID(slug); got != want {
			t.Errorf("clinchExternalID(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestClinchSplitSlug(t *testing.T) {
	cases := []struct {
		name      string
		slug      string
		wantTitle string
		wantLoc   string
	}{
		{
			name:      "single-word city (Warsaw) splits cleanly, BE stays in title",
			slug:      "software-engineer-payment-be-warsaw-masovian-voivodeship-poland",
			wantTitle: "Software Engineer Payment Be",
			wantLoc:   "Warsaw, Masovian, Voivodeship, Poland",
		},
		{
			name:      "multiword city (Mountain View) is the cut point",
			slug:      "machine-learning-engineer-runtime-optimization-mountain-view-california-united-states",
			wantTitle: "Machine Learning Engineer Runtime Optimization",
			wantLoc:   "Mountain View, California, United States",
		},
		{
			name:      "London / England / United Kingdom",
			slug:      "staff-machine-learning-engineer-simulation-london-england-united-kingdom",
			wantTitle: "Staff Machine Learning Engineer Simulation",
			wantLoc:   "London, England, United Kingdom",
		},
		{
			name:      "UUID stripped before splitting",
			slug:      "senior-machine-learning-engineer-simulation-london-england-united-kingdom-6da190c4-27e6-467a-85f8-a242d5a5c206",
			wantTitle: "Senior Machine Learning Engineer Simulation",
			wantLoc:   "London, England, United Kingdom",
		},
		{
			name:      "multi-location tail all goes to location",
			slug:      "senior-android-engineer-mountain-view-california-united-states-san-francisco",
			wantTitle: "Senior Android Engineer",
			wantLoc:   "Mountain View, California, United States, San Francisco",
		},
		{
			name:      "no resolvable location: whole slug is the title",
			slug:      "special-projects-lead-classified-division",
			wantTitle: "Special Projects Lead Classified Division",
			wantLoc:   "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, loc := clinchSplitSlug(c.slug)
			if title != c.wantTitle {
				t.Errorf("title = %q, want %q", title, c.wantTitle)
			}
			if loc != c.wantLoc {
				t.Errorf("location = %q, want %q", loc, c.wantLoc)
			}
		})
	}
}

func TestClinchFetchParsesSitemapAndFiltersNonJobURLs(t *testing.T) {
	job := "https://careers.withwaymo.com/jobs/backend-technical-lead-manager-taas-warsaw-masovian-voivodeship-poland"
	fake := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(
		"https://careers.withwaymo.com/",          // marketing page — filtered out
		"https://careers.withwaymo.com/why-waymo", // marketing page — filtered out
		job,
	))

	// No detail route: hydration fails per-posting (a non-challenge error), so the description
	// stays empty and the sitemap-only fields are what this test asserts.
	jobs, err := NewClinch(fake, &routedDetail{routes: map[string]string{}}).Fetch(context.Background(), CompanyEntry{
		Company: "Waymo", Provider: "clinch", Board: "careers.withwaymo.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (non-job URLs filtered)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "backend-technical-lead-manager-taas-warsaw-masovian-voivodeship-poland" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != job {
		t.Errorf("URL = %q, want %q", j.URL, job)
	}
	if j.Title != "Backend Technical Lead Manager Taas" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Waymo" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Warsaw, Masovian, Voivodeship, Poland" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "" {
		t.Errorf("Description = %q, want empty (detail page is WAF-blocked)", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-04", j.PostedAt)
	}
}

func TestClinchFetchEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML())
	jobs, err := NewClinch(fake, &routedDetail{routes: map[string]string{}}).Fetch(context.Background(), CompanyEntry{Board: "careers.withwaymo.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
