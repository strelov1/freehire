package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
)

// professionIndexXML is the platform's sitemap index, carrying the two IT categories, one
// general category, and two sitemaps that are not categories at all.
const professionIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://www.profession.hu/sitemap-listings-education-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-listings-itdev-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-companies-hu.xml</loc></sitemap>
</sitemapindex>`

const professionEducationSitemapXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://www.profession.hu/allas/tanar-acme-kft-1001</loc></url>
<url><loc>https://www.profession.hu/allas/rendszergazda-acme-kft-1002</loc></url>
<url><loc>https://www.profession.hu/kategoria/oktatas</loc></url>
</urlset>`

func newProfessionProber() professionProber {
	return professionProber{index: &professionCategoryIndex{}}
}

func professionProberFixture() fakeGetter {
	return fakeGetter{
		"https://www.profession.hu/sitemap-listings-index-hu.xml":     professionIndexXML,
		"https://www.profession.hu/sitemap-listings-education-hu.xml": professionEducationSitemapXML,
	}
}

// TestProfessionProberDiscover pins the reason this provider has a prober of its own: its
// boards are the platform's own categories, published in one authoritative index, so they
// are enumerated rather than seeded. The non-category sitemaps in the index must not
// become boards.
func TestProfessionProberDiscover(t *testing.T) {
	got, err := newProfessionProber().discover(context.Background(), professionProberFixture())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"education", "itdev"}
	if !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v", got, want)
	}
}

// TestProfessionProberProbe pins the cheap probe. Counting a category's sitemap costs one
// request; probing it through the adapter — which is what proberFor falls back to without
// this type — would crawl every posting in all 23 categories.
func TestProfessionProberProbe(t *testing.T) {
	company, open, err := newProfessionProber().probe(context.Background(), professionProberFixture(), "education")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	// The board is a category of one central catalogue, so there is no employer to
	// report; the label names the platform and the category, matching the two rows the
	// catalogue already holds.
	if company != "Profession.hu — education" {
		t.Errorf("company = %q", company)
	}
	// Only the two posting URLs count. The category page listed beside them is not a
	// posting.
	if open != 2 {
		t.Errorf("openJobs = %d, want 2", open)
	}
}

// TestProfessionProberProbeUnreachableCategoryErrors pins that a category whose sitemap
// cannot be read is surfaced rather than silently counted as empty. probeAll logs and
// skips it either way, but it fails the whole run when EVERY probe errors — which is how
// an outage is told apart from a platform that has genuinely retired its categories.
func TestProfessionProberProbeUnreachableCategoryErrors(t *testing.T) {
	// The index names the category; its sitemap is not served.
	fixture := fakeGetter{
		"https://www.profession.hu/sitemap-listings-index-hu.xml": professionIndexXML,
	}
	if _, _, err := (newProfessionProber()).probe(context.Background(), fixture, "education"); err == nil {
		t.Error("probe on an unreadable category sitemap returned no error")
	}
}

// countingGetter counts every XML request, so the prober can be held to reading the
// platform's sitemap index once per run rather than once per candidate.
type countingGetter struct {
	fakeGetter
	mu  sync.Mutex
	xml []string
}

func (c *countingGetter) GetXML(ctx context.Context, url string, v any) error {
	c.mu.Lock()
	c.xml = append(c.xml, url)
	c.mu.Unlock()
	return c.fakeGetter.GetXML(ctx, url, v)
}

// TestProfessionProberReadsTheIndexOnce pins the load fix. probeAll runs candidates
// concurrently, so resolving the index per candidate would ask for one document 23 times
// in parallel — which is what the platform closed the connection on when it was measured
// live on 2026-09-06, arriving as "every category is unreachable".
func TestProfessionProberReadsTheIndexOnce(t *testing.T) {
	c := &countingGetter{fakeGetter: professionProberFixture()}
	p := newProfessionProber()
	boards, err := p.discover(context.Background(), c)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, b := range boards {
		// itdev's sitemap is not served by the fixture; the error is not what this test
		// is about, only the requests made getting there.
		_, _, _ = p.probe(context.Background(), c, b)
	}
	indexReads := 0
	for _, u := range c.xml {
		if strings.Contains(u, "sitemap-listings-index-hu.xml") {
			indexReads++
		}
	}
	if indexReads != 1 {
		t.Errorf("read the sitemap index %d times over discover + %d probes, want 1 (requests: %v)",
			indexReads, len(boards), c.xml)
	}
}
