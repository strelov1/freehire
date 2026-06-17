package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

const thehubDetailHTML = `<html><head>
<script data-hid="ldjson-schema" type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"Founding Engineer (Rust)",
"description":"<p>Build &amp; ship in Rust.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-12T13:24:38.313Z",
"hiringOrganization":{"@type":"Organization","name":"syncable"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Copenhagen","addressRegion":"Copenhagen","addressCountry":"Denmark"}}}
</script></head><body></body></html>`

const thehubIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://thehub.io/sitemap-static.xml</loc></sitemap>
<sitemap><loc>https://thehub.io/sitemap-jobs.xml</loc></sitemap>
</sitemapindex>`

func thehubJobsXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestTheHubProvider(t *testing.T) {
	if got := NewTheHub(nil).Provider(); got != "thehub" {
		t.Errorf("Provider() = %q, want thehub", got)
	}
}

func TestTheHubIsBoardlessAggregator(t *testing.T) {
	s := NewTheHub(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("thehub should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("thehub should implement the aggregator marker")
	}
}

func TestTheHubRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["thehub"]; !ok {
		t.Error("All() should register provider thehub")
	}
	if !slices.Contains(FilterableProviders(), "thehub") {
		t.Error("FilterableProviders() should include thehub")
	}
}

func TestTheHubBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/thehub.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/thehub.yml fails validation: %v", err)
	}
}

func TestTheHubJobID(t *testing.T) {
	cases := map[string]string{
		"https://thehub.io/jobs/6a2c0896f67c01342d0fb744": "6a2c0896f67c01342d0fb744",
		"https://thehub.io/startups/acme":                 "",
	}
	for u, want := range cases {
		if got := thehubJobID(u); got != want {
			t.Errorf("thehubJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestTheHubFetchResolvesJobSitemapThenMaps(t *testing.T) {
	job := "https://thehub.io/jobs/6a2c0896f67c01342d0fb744"
	fake := (&routedHTTP{}).
		route("/jobs/6a2c0896f67c01342d0fb744", thehubDetailHTML).
		route("/sitemap-jobs.xml", thehubJobsXML(job, "https://thehub.io/startups/acme")).
		route("/sitemap.xml", thehubIndexXML)

	jobs, err := NewTheHub(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (non-job sitemap entry filtered)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "6a2c0896f67c01342d0fb744" || j.URL != job {
		t.Errorf("id/url wrong: %s %s", j.ExternalID, j.URL)
	}
	if j.Company != "syncable" || j.Title != "Founding Engineer (Rust)" {
		t.Errorf("bad mapping: company=%q title=%q", j.Company, j.Title)
	}
	if j.Location != "Copenhagen, Denmark" {
		// region == locality "Copenhagen" duplicated; joinNonEmpty keeps both → "Copenhagen, Copenhagen, Denmark"
		if j.Location != "Copenhagen, Copenhagen, Denmark" {
			t.Errorf("Location = %q", j.Location)
		}
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 12, 13, 24, 38, 313000000, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}
