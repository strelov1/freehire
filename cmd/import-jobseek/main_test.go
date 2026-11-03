package main

import "testing"

func TestJoinSlug(t *testing.T) {
	tests := []struct {
		url  string
		slug string
		ok   bool
	}{
		{"https://join.com/companies/1komma5grad", "1komma5grad", true},
		{"https://join.com/companies/1komma5grad/", "1komma5grad", true},
		{"https://join.com/companies/ahtiinteriors/jobs", "", false},
		{"https://join.com/companies/", "", false},
		{"https://join.com/jobs", "", false},
		{"https://example.com/companies/acme", "", false},
	}
	for _, tt := range tests {
		got, ok := joinSlug(tt.url)
		if ok != tt.ok || got != tt.slug {
			t.Errorf("joinSlug(%q) = %q, %v; want %q, %v", tt.url, got, ok, tt.slug, tt.ok)
		}
	}
}

func TestEightfoldHost(t *testing.T) {
	tests := []struct {
		url  string
		host string
		ok   bool
	}{
		{"https://aexp.eightfold.ai/careers", "aexp.eightfold.ai", true},
		{"https://jobs.alnylam.com/careers", "", false},
		{"https://www.eightfold.ai/", "", false},
		{"https://eightfold.ai/pricing", "", false},
	}
	for _, tt := range tests {
		got, ok := eightfoldHost(tt.url)
		if ok != tt.ok || got != tt.host {
			t.Errorf("eightfoldHost(%q) = %q, %v; want %q, %v", tt.url, got, ok, tt.host, tt.ok)
		}
	}
}

func TestClassifySkip(t *testing.T) {
	for _, mt := range []string{"dom", "sitemap", "rss", "api_sniffer", "nextdata", "inline", "linkedin", "notion"} {
		if got := classifySkip(boardRow{monitorType: mt}); got != "custom scrape of a corporate career site (no ATS)" &&
			got != "covered by our own aggregator provider" {
			t.Errorf("classifySkip(monitor %q) = %q, want a scrape or aggregator reason", mt, got)
		}
	}
	if got := classifySkip(boardRow{monitorType: "jobylon"}); got != "covered by our own aggregator provider" {
		t.Errorf("classifySkip(jobylon) = %q", got)
	}
	if got := classifySkip(boardRow{monitorType: "mystery"}); got != "unrecognized ATS host" {
		t.Errorf("classifySkip(mystery) = %q", got)
	}
}
