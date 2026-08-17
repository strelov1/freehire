package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// uberDetailHTML renders a job page the way jobs.uber.com does: a single
// application/ld+json JobPosting whose identifier is a PropertyValue{value: "<id>"}.
func uberDetailHTML(title, datePosted, employmentType string, locations ...[3]string) string {
	var locs strings.Builder
	for _, l := range locations {
		locs.WriteString(`{"@type":"Place","name":"` + l[0] + `","address":{"@type":"PostalAddress","addressLocality":"` +
			l[0] + `","addressRegion":"` + l[1] + `","addressCountry":"` + l[2] + `"}},`)
	}
	loc := strings.TrimSuffix(locs.String(), ",")
	return `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"description":"<p>Build the future of movement.<\/p><script>alert(1)<\/script>",
"identifier":{"@type":"PropertyValue","name":"Uber","value":"149574"},
"datePosted":"` + datePosted + `",
"employmentType":"` + employmentType + `",
"jobLocation":[` + loc + `]}
</script></head><body>page</body></html>`
}

func uberSitemapXML(entries ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, e := range entries {
		b.WriteString(`<url><loc>` + e[0] + `</loc><lastmod>` + e[1] + `</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestUberProvider(t *testing.T) {
	if got := NewUber(nil).Provider(); got != "uber" {
		t.Errorf("Provider() = %q, want %q", got, "uber")
	}
}

func TestUberRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["uber"]
	if !ok {
		t.Fatal(`All(nil)["uber"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("uber should be boardless (single company, no board id)")
	}
}

func TestUberFetchSitemapThenDetailAndMaps(t *testing.T) {
	loc := "https://jobs.uber.com/en/jobs/149574/"
	fake := (&routedHTTP{}).
		route("/en/jobs/sitemap.xml", uberSitemapXML([2]string{loc, "2026-06-06T19:38:59Z"})).
		route("/en/jobs/149574", uberDetailHTML("Sr Staff Engineer", "2026-06-19", "FULL_TIME",
			[3]string{"Sao Paulo", "SP", "Brazil"}))

	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber", Provider: "uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "149574" {
		t.Errorf("ExternalID = %q, want 149574", j.ExternalID)
	}
	if j.URL != loc {
		t.Errorf("URL = %q, want %q", j.URL, loc)
	}
	if j.Title != "Sr Staff Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Uber" {
		t.Errorf("Company = %q, want Uber", j.Company)
	}
	if want := "Sao Paulo, SP, Brazil"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "Build the future") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	// datePosted ("2026-06-19", date-only) wins over the sitemap <lastmod>.
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-19", j.PostedAt)
	}
	if j.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (no structured signal, only the Remote heuristic)", j.WorkMode)
	}
}

func TestUberPostedAtFallsBackToLastmod(t *testing.T) {
	loc := "https://jobs.uber.com/en/jobs/999/"
	fake := (&routedHTTP{}).
		route("/en/jobs/sitemap.xml", uberSitemapXML([2]string{loc, "2026-05-05T12:00:00Z"})).
		route("/en/jobs/999", uberDetailHTML("Eng", "", "FULL_TIME", [3]string{"Remote", "", "US"}))
	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].PostedAt == nil || !jobs[0].PostedAt.Equal(time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want lastmod fallback 2026-05-05T12:00:00Z", jobs[0].PostedAt)
	}
}

func TestUberEmptyLocationWhenNoJobLocation(t *testing.T) {
	loc := "https://jobs.uber.com/en/jobs/888/"
	fake := (&routedHTTP{}).
		route("/en/jobs/sitemap.xml", uberSitemapXML([2]string{loc, "2026-06-06T00:00:00Z"})).
		route("/en/jobs/888", uberDetailHTML("Eng", "2026-04-14", "FULL_TIME"))
	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "" {
		t.Fatalf("want empty location, got %v", jobs)
	}
}

func TestUberFailedDetailDropsOnlyThatPosting(t *testing.T) {
	ok := "https://jobs.uber.com/en/jobs/111/"
	bad := "https://jobs.uber.com/en/jobs/222/"
	// No route for /en/jobs/222 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("/en/jobs/sitemap.xml", uberSitemapXML(
			[2]string{ok, "2026-06-06T00:00:00Z"}, [2]string{bad, "2026-06-06T00:00:00Z"})).
		route("/en/jobs/111", uberDetailHTML("Kept", "2026-04-14", "FULL_TIME", [3]string{"Remote", "", "US"}))

	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Kept" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestUberEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/en/jobs/sitemap.xml", uberSitemapXML())
	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestUberSitemapFetchErrorFailsBoard(t *testing.T) {
	// No route for the sitemap URL at all: the sitemap fetch itself fails, and Fetch must
	// return that error rather than reading it as an empty catalogue.
	fake := &routedHTTP{}
	if _, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"}); err == nil {
		t.Fatal("Fetch: want error on sitemap fetch failure, got nil")
	}
}
