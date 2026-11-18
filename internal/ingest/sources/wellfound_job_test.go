package sources

import (
	"context"
	"strings"
	"testing"
)

func TestWellfound_Fetch_MapsAWellFormedEntryToAJob(t *testing.T) {
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, pages: map[string]string{url1: wellfoundOnePageSinglePage()}}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var backend *Job
	for i := range jobs {
		if jobs[i].ExternalID == "100" {
			backend = &jobs[i]
		}
	}
	if backend == nil {
		t.Fatalf("job 100 not found among %d mapped jobs", len(jobs))
	}
	if backend.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", backend.Title)
	}
	if backend.Company != "Acme Inc" {
		t.Errorf("Company = %q, want Acme Inc", backend.Company)
	}
	if backend.URL != "https://wellfound.com/jobs/100-senior-backend-engineer" {
		t.Errorf("URL = %q", backend.URL)
	}
	if !backend.Remote {
		t.Error("Remote = false, want true")
	}
	if backend.Location != "United States" {
		t.Errorf("Location = %q, want United States", backend.Location)
	}
	if !strings.Contains(backend.Description, "Build things.") {
		t.Errorf("Description = %q, want it to contain the body text", backend.Description)
	}
	if !strings.Contains(backend.Description, "150k") {
		t.Errorf("Description = %q, want the free-text compensation folded in (SEEK/Workstream precedent)", backend.Description)
	}
}

// TestWellfound_Fetch_ConvertsMarkdownDescriptionToHTML guards against a real bug found live
// (freehire.me/jobs/ai-engineer-co-founder-role-brandbrahma-pkuqoprb): Wellfound's own
// description field is Markdown, not HTML — the earlier test fixtures happened to use
// already-HTML bodies ("<p>Build things.</p>"), which never exercised this. A raw
// "**bold**"/"* list item" was rendering literally on the site instead of as real markup.
// join.go's markdownToHTML (goldmark) is the established fix for exactly this shape of
// source in this package.
func TestWellfound_Fetch_ConvertsMarkdownDescriptionToHTML(t *testing.T) {
	data := `{
  "ROOT_QUERY": {
    "__typename": "Query",
    "talent": {
      "__typename": "Talent",
      "seoLandingPageJobSearchResults({\"page\":1,\"remote\":true,\"role\":\"software-engineer\"})": {
        "__typename": "Results",
        "pageCount": 1,
        "totalJobCount": 1,
        "totalStartupCount": 1,
        "perPage": 20,
        "startups": [{"__ref": "StartupResult:1"}]
      }
    }
  },
  "StartupResult:1": {
    "__typename": "StartupResult",
    "id": "1",
    "name": "Acme Inc",
    "slug": "acme-inc",
    "highlightedJobListings": [{"__ref": "JobListingSearchResult:100"}]
  },
  "JobListingSearchResult:100": {
    "__typename": "JobListingSearchResult",
    "id": "100",
    "slug": "founding-engineer",
    "title": "Founding Engineer",
    "description": "**Equity-Only Position**: no cash salary until funding.\n\n* Own the roadmap\n* Ship fast",
    "compensation": "",
    "remote": true,
    "locationNames": []
  }
}`
	url1 := wellfoundPageURL("software-engineer", 1)
	fake := &wellfoundFakeHTTP{t: t, pages: map[string]string{url1: data}}
	jobs, err := NewWellfound(fake).Fetch(context.Background(), CompanyEntry{Provider: "wellfound", Board: "software-engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	desc := jobs[0].Description
	if strings.Contains(desc, "**Equity-Only Position**") {
		t.Errorf("Description = %q, raw Markdown was not rendered to HTML", desc)
	}
	if !strings.Contains(desc, "<strong>Equity-Only Position</strong>") {
		t.Errorf("Description = %q, want the Markdown bold rendered as <strong>", desc)
	}
	if !strings.Contains(desc, "<li>") {
		t.Errorf("Description = %q, want the Markdown bullet list rendered as <li>", desc)
	}
}

func TestWellfoundToJob_ReadsARupeeRange(t *testing.T) {
	tests := []struct {
		comp     string
		lo, hi   int
		currency string
	}{
		{"₹25L – ₹30L • No equity", 2500000, 3000000, "INR"},
		{"₹40L – ₹1 cr • 0.1% – 0.5%", 4000000, 10000000, "INR"},
		// Dollars, plain rupees with no period, and prose stay display text.
		{"$150k – $180k", 0, 0, ""},
		{"₹10,000 – ₹15,000 • No equity", 0, 0, ""},
		{"Competitive salary + ESOPs", 0, 0, ""},
	}
	for _, tc := range tests {
		rj := wellfoundResolvedJob{Entry: wellfoundJobEntry{ID: "1", Slug: "x", Title: "Engineer", Compensation: tc.comp}, Company: "Acme"}
		j, ok := rj.toJob()
		if !ok {
			t.Fatalf("toJob(%q) not ok", tc.comp)
		}
		if intOrZero(j.SalaryMin) != tc.lo || intOrZero(j.SalaryMax) != tc.hi || j.SalaryCurrency != tc.currency {
			t.Errorf("toJob(%q) salary = %d-%d %q, want %d-%d %q", tc.comp, intOrZero(j.SalaryMin), intOrZero(j.SalaryMax), j.SalaryCurrency, tc.lo, tc.hi, tc.currency)
		}
		if tc.currency != "" && j.SalaryPeriod != "year" {
			t.Errorf("toJob(%q) period = %q, want year", tc.comp, j.SalaryPeriod)
		}
	}
}
