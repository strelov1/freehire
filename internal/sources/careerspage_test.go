package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// careerspageDetailHTML is a careers-page.com job detail page: a server-rendered page whose
// payload we read is the schema.org JobPosting ld+json. addressCountry is the full country
// name ("United States"), which we keep as free-text location (the location dictionary resolves
// it). The description embeds a <script> (escaped as <\/script>) that sanitizeHTML must strip.
func careerspageDetailHTML(title string) string {
	return `<html><head></head><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"description":"<h2>About</h2><p>Build things.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-29T19:22:10.912789+00:00",
"validThrough":"2026-08-01T21:12:47.666565+00:00",
"employmentType":"FULL_TIME",
"hiringOrganization":{"@type":"Organization","name":"David Joseph & Company"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"New York","addressCountry":"United States"}}}
</script>
</body></html>`
}

// careerspageListingHTML is a careers-page.com listing page carrying the given canonical job
// UUIDs. Each card also renders /refer and /apply sub-action anchors (no leading slash) that
// must NOT be treated as postings, plus a pagination anchor — exercising the job-link predicate.
func careerspageListingHTML(uuids ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs-col">`)
	for _, u := range uuids {
		b.WriteString(`<div class="job-card" data-job-id="` + u + `">`)
		b.WriteString(`<a class="job-title-link" href="/jobs/` + u + `">A Role</a>`)
		b.WriteString(`<a href="jobs/` + u + `/refer">Refer</a>`)
		b.WriteString(`<a href="jobs/` + u + `/apply">Apply</a>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div class="pagination-wrapper"><a href="?page=2">Next</a></div>`)
	b.WriteString(`</body></html>`)
	return b.String()
}

// emptyListingHTML is a listing page past the last real page: it carries no job cards, so the
// pagination loop must stop when it yields no new job links.
const emptyListingHTML = `<html><body><div class="jobs-col"></div></body></html>`

func TestCareerPageProvider(t *testing.T) {
	if got := NewCareerPage(nil).Provider(); got != "careerspage" {
		t.Errorf("Provider() = %q, want %q", got, "careerspage")
	}
}

func TestCareerPageJobID(t *testing.T) {
	cases := map[string]string{
		"https://acme.careers-page.com/jobs/f2222f51-835a-4d0a-9435-291cd2bdaa06":       "f2222f51-835a-4d0a-9435-291cd2bdaa06",
		"/jobs/a593c5a5-855d-4660-ad73-f68ababc77fd":                                    "a593c5a5-855d-4660-ad73-f68ababc77fd",
		"https://acme.careers-page.com/jobs/f2222f51-835a-4d0a-9435-291cd2bdaa06?x=1":   "f2222f51-835a-4d0a-9435-291cd2bdaa06",
		"jobs/f2222f51-835a-4d0a-9435-291cd2bdaa06/refer":                               "", // sub-action, not a posting
		"https://acme.careers-page.com/jobs/f2222f51-835a-4d0a-9435-291cd2bdaa06/apply": "", // sub-action, not a posting
		"https://acme.careers-page.com/":                                                "",
	}
	for loc, want := range cases {
		if got := careerspageJobID(loc); got != want {
			t.Errorf("careerspageJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestCareerPageFetchListingThenDetailAndMaps(t *testing.T) {
	u := "f2222f51-835a-4d0a-9435-291cd2bdaa06"
	fake := (&routedHTTP{}).
		route("?page=1", careerspageListingHTML(u)).
		route("?page=2", emptyListingHTML).
		route("/jobs/"+u, careerspageDetailHTML("Founding Software Engineer"))

	jobs, err := NewCareerPage(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Provider: "careerspage", Board: "davidjoseph-co",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != u {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, u)
	}
	if j.URL != "https://davidjoseph-co.careers-page.com/jobs/"+u {
		t.Errorf("URL = %q, want canonical detail URL", j.URL)
	}
	if j.Title != "Founding Software Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "David Joseph & Company" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "New York, United States" {
		t.Errorf("Location = %q, want %q", j.Location, "New York, United States")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "<h2>About</h2>") || !strings.Contains(j.Description, "Build things") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 29, 19, 22, 10, 912789000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-29T19:22:10Z", j.PostedAt)
	}
}

func TestCareerPagePaginatesAcrossPages(t *testing.T) {
	u1 := "f2222f51-835a-4d0a-9435-291cd2bdaa06"
	u2 := "a593c5a5-855d-4660-ad73-f68ababc77fd"
	u3 := "b1763eee-15ef-4dc5-963d-d783ff466124"
	fake := (&routedHTTP{}).
		route("?page=1", careerspageListingHTML(u1, u2)).
		route("?page=2", careerspageListingHTML(u3)).
		route("?page=3", emptyListingHTML).
		route("/jobs/"+u1, careerspageDetailHTML("Role 1")).
		route("/jobs/"+u2, careerspageDetailHTML("Role 2")).
		route("/jobs/"+u3, careerspageDetailHTML("Role 3"))

	jobs, err := NewCareerPage(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (across two listing pages)", len(jobs))
	}
	ids := []string{jobs[0].ExternalID, jobs[1].ExternalID, jobs[2].ExternalID}
	for _, want := range []string{u1, u2, u3} {
		if !slices.Contains(ids, want) {
			t.Errorf("missing job %q in %v", want, ids)
		}
	}
}

func TestCareerPageDropsDetailWithoutJobPosting(t *testing.T) {
	u1 := "f2222f51-835a-4d0a-9435-291cd2bdaa06"
	u2 := "a593c5a5-855d-4660-ad73-f68ababc77fd"
	fake := (&routedHTTP{}).
		route("?page=1", careerspageListingHTML(u1, u2)).
		route("?page=2", emptyListingHTML).
		route("/jobs/"+u1, careerspageDetailHTML("Good Role")).
		route("/jobs/"+u2, `<html><body>no ld+json here</body></html>`)

	jobs, err := NewCareerPage(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != u1 {
		t.Fatalf("got %v, want only the posting with a JobPosting block", jobs)
	}
}

func TestCareerPageRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["careerspage"]
	if !ok {
		t.Fatal("All() missing provider careerspage")
	}
	if s.Provider() != "careerspage" {
		t.Errorf("All()[careerspage].Provider() = %q", s.Provider())
	}
	// Board-based (not boardless): it appears in the source facet.
	if !slices.Contains(FilterableProviders(), "careerspage") {
		t.Error("FilterableProviders() should include careerspage")
	}
}
