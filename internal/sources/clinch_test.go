package sources

import (
	"context"
	"testing"
	"time"
)

func clinchSitemapXML(locs ...string) string {
	xml := `<?xml version="1.0"?><urlset>`
	for _, l := range locs {
		xml += `<url><loc>` + l + `</loc><lastmod>2026-05-04</lastmod></url>`
	}
	return xml + `</urlset>`
}

func TestClinchProvider(t *testing.T) {
	if got := NewClinch(nil).Provider(); got != "clinch" {
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

	jobs, err := NewClinch(fake).Fetch(context.Background(), CompanyEntry{
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
	jobs, err := NewClinch(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.withwaymo.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
