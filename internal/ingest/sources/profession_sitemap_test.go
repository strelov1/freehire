package sources

import (
	"context"
	"maps"
	"strings"
	"testing"
)

func TestProfessionCategorySitemaps(t *testing.T) {
	// The index carries entries that are not category sitemaps — the platform publishes
	// article and company indexes beside them — and they must not become boards.
	const indexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://www.profession.hu/sitemap-listings-education-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-listings-itdev-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-companies-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-articles-hu.xml</loc></sitemap>
</sitemapindex>`
	http := (&routedHTTP{}).route("sitemap-listings-index-hu.xml", indexXML)
	got, err := ProfessionCategorySitemaps(context.Background(), http)
	if err != nil {
		t.Fatalf("ProfessionCategorySitemaps: %v", err)
	}
	want := map[string]string{
		"education": "https://www.profession.hu/sitemap-listings-education-hu.xml",
		"itdev":     "https://www.profession.hu/sitemap-listings-itdev-hu.xml",
	}
	if !maps.Equal(got, want) {
		t.Errorf("ProfessionCategorySitemaps() = %v, want %v", got, want)
	}
}

// TestProfessionSitemapPostings pins the cheap liveness measure a board harvest needs: how
// many postings a category lists, counted from its sitemap rather than by crawling it. The
// category page listed beside the postings is not one.
func TestProfessionSitemapPostings(t *testing.T) {
	const sitemapXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://www.profession.hu/allas/tanar-acme-kft-1001</loc></url>
<url><loc>https://www.profession.hu/allas/rendszergazda-acme-kft-1002</loc></url>
<url><loc>https://www.profession.hu/kategoria/oktatas</loc></url>
</urlset>`
	http := (&routedHTTP{}).route("sitemap-listings-education-hu.xml", sitemapXML)
	n, err := ProfessionSitemapPostings(context.Background(), http,
		"https://www.profession.hu/sitemap-listings-education-hu.xml")
	if err != nil {
		t.Fatalf("ProfessionSitemapPostings: %v", err)
	}
	if n != 2 {
		t.Errorf("ProfessionSitemapPostings() = %d, want 2", n)
	}
}

// TestProfessionResolvesTheSitemapIndexOnce pins that a crawl reads the platform's sitemap
// index once however many of its categories it visits.
//
// This is load, not tidiness. The index is where a board's own sitemap is resolved, so
// re-reading it per board scaled with the number of boards — invisible while the crawl was
// the two IT categories, and 23 requests for the same document once the general categories
// joined. Measured live on 2026-09-06, the platform answered the first request and then
// closed the connection on the rest: it rate-limits hard enough that this alone took a
// board harvest from working to reporting every category as unreachable.
func TestProfessionResolvesTheSitemapIndexOnce(t *testing.T) {
	fixture := &professionRecorder{routedHTTP: (&routedHTTP{}).
		route("sitemap-listings-index-hu.xml", professionSitemapIndexXML).
		route("sitemap-listings-itdev-hu.xml", professionCategorySitemapXML()).
		route("sitemap-listings-itops-hu.xml", professionCategorySitemapXML())}

	src := NewProfession(fixture)
	for _, board := range []string{"itdev", "itops"} {
		if _, err := src.Fetch(context.Background(), CompanyEntry{Provider: "profession", Board: board}); err != nil {
			t.Fatalf("Fetch(%s): %v", board, err)
		}
	}
	indexReads := 0
	for _, u := range fixture.xml {
		if strings.Contains(u, "sitemap-listings-index-hu.xml") {
			indexReads++
		}
	}
	if indexReads != 1 {
		t.Errorf("read the sitemap index %d times across two boards, want 1 (requests: %v)",
			indexReads, fixture.xml)
	}
}

// TestProfessionGetsASingleUseConnectionClient pins that the crawl hands this adapter a
// client of its own rather than the shared one.
//
// The platform mishandles a connection it has already answered on. Measured from the
// production host on 2026-09-06, the ingest run failed both boards in the same second with
// a bare EOF while curl answered the identical URLs twenty seconds earlier — and the crawl
// had been failing that way since 2026-09-04, with the two boards eventually cooled off
// and 716 postings going stale. The shared client pools connections, so a fix that only
// paced the crawl would have left it.
func TestProfessionGetsASingleUseConnectionClient(t *testing.T) {
	shared := NewClient()
	registry := All(shared)
	src, ok := registry["profession"]
	if !ok {
		t.Fatal("profession missing from the registry")
	}
	p, ok := src.(profession)
	if !ok {
		t.Fatalf("registry holds %T, not the profession adapter", src)
	}
	if p.http == nil {
		t.Fatal("profession got no transport")
	}
	if any(p.http) == any(shared) {
		t.Error("profession shares the pooled client; it needs one that does not reuse connections")
	}
	// The taxonomy path builds the registry with no transport at all, and must not panic
	// or invent one on the way to Provider()/marker assertions.
	if got := All(nil)["profession"].Provider(); got != "profession" {
		t.Errorf("transport-free registry Provider() = %q", got)
	}
}
