package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// careerplugDetailA/B are CareerPlug job pages: server-rendered HTML whose payload is the
// schema.org JobPosting ld+json. jobLocation is a single Place; hiringOrganization names the
// (per-posting, franchise-level) employer.
const careerplugDetailA = `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",
"title":"Restaurant Team Member","description":"<div>Serve guests.</div><script>x()<\/script>",
"datePosted":"2026-06-20T12:00:00+00:00","employmentType":"FULL_TIME",
"hiringOrganization":{"@type":"Organization","name":"Golden Corral - Concord"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Concord","addressRegion":"CA","addressCountry":"US"}}}</script>
</head><body>Apply</body></html>`

const careerplugDetailB = `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",
"title":"Shift Lead","description":"<p>Lead a shift.</p>","datePosted":"2026-06-19T09:00:00+00:00",
"hiringOrganization":{"@type":"Organization","name":"Golden Corral - Reno"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Reno","addressRegion":"NV","addressCountry":"US"}}}</script>
</head><body>Apply</body></html>`

// careerplugList1/2 are the paginated /jobs listing pages: each links its postings as
// /jobs/<id>, and a job is linked twice (title + row) so the adapter must de-dup.
const careerplugList1 = `<html><body><div id="job_table">
<a href="/jobs/100"><span class="name">Restaurant Team Member</span></a>
<a href="/jobs/100">Apply</a>
<a href="/about">About</a>
<a href="/jobs?page=2">Next</a>
</div></body></html>`

const careerplugList2 = `<html><body><div id="job_table">
<a href="/jobs/200"><span class="name">Shift Lead</span></a>
</div></body></html>`

func TestCareerPlugProvider(t *testing.T) {
	if got := NewCareerPlug(nil).Provider(); got != "careerplug" {
		t.Errorf("Provider() = %q, want %q", got, "careerplug")
	}
}

func TestCareerPlugJobID(t *testing.T) {
	cases := map[string]string{
		"https://acme.careerplug.com/jobs/932346": "932346",
		"/jobs/100":    "100",
		"/jobs?page=2": "",
		"/jobs":        "",
		"/about":       "",
	}
	for u, want := range cases {
		if got := careerplugJobID(u); got != want {
			t.Errorf("careerplugJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestCareerPlugFetchPaginatesThenMaps(t *testing.T) {
	// Contains-matching, so the most specific routes (the detail ids, then page=2) come
	// before the bare /jobs page-1 listing.
	fake := (&routedHTTP{}).
		route("/jobs/100", careerplugDetailA).
		route("/jobs/200", careerplugDetailB).
		route("/jobs?page=2", careerplugList2).
		route("/jobs", careerplugList1)

	jobs, err := NewCareerPlug(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Golden Corral", Provider: "careerplug", Board: "golden-corral-careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (paginated across two listing pages, links de-duped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	a, ok := byID["100"]
	if !ok {
		t.Fatalf("missing job 100; got %v", byID)
	}
	if a.Title != "Restaurant Team Member" || a.Company != "Golden Corral - Concord" {
		t.Errorf("job 100: got title=%q company=%q", a.Title, a.Company)
	}
	if a.Location != "Concord, CA, US" {
		t.Errorf("job 100 location = %q, want %q", a.Location, "Concord, CA, US")
	}
	if a.URL != "https://golden-corral-careers.careerplug.com/jobs/100" {
		t.Errorf("job 100 URL = %q (relative href must resolve against the board host)", a.URL)
	}
	if strings.Contains(a.Description, "<script>") {
		t.Errorf("job 100 description not sanitized: %q", a.Description)
	}
	if a.PostedAt == nil || !a.PostedAt.Equal(time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("job 100 PostedAt = %v", a.PostedAt)
	}
	if _, ok := byID["200"]; !ok {
		t.Errorf("missing job 200 (second listing page not followed)")
	}
}
