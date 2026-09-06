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
	got, err := professionProber{index: &professionCategoryIndex{}, client: professionProberFixture()}.discover(context.Background(), professionProberFixture())
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
	company, open, err := professionProber{index: &professionCategoryIndex{}, client: professionProberFixture()}.probe(context.Background(), professionProberFixture(), "education")
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
	p := professionProber{index: &professionCategoryIndex{}, client: fixture}
	if _, _, err := p.probe(context.Background(), nil, "education"); err == nil {
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
	p := professionProber{index: &professionCategoryIndex{}, client: c}
	boards, err := p.discover(context.Background(), nil)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, b := range boards {
		// itdev's sitemap is not served by the fixture; the error is not what this test
		// is about, only the requests made getting there.
		_, _, _ = p.probe(context.Background(), nil, b)
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

// TestProfessionProberIgnoresTheSharedClient pins that the prober reads the platform over
// its own single-use-connection client rather than the harvest's shared one, which pools.
// The prober reads exactly the sitemaps the crawl reads, so it meets exactly the wall the
// crawl met — see sources.NewSingleUseConnClient.
func TestProfessionProberIgnoresTheSharedClient(t *testing.T) {
	p := professionProber{index: &professionCategoryIndex{}, client: professionProberFixture()}
	// The shared client passed in would fail every route; the prober must not reach for it.
	got, err := p.discover(context.Background(), fakeGetter{})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !slices.Equal(got, []string{"education", "itdev"}) {
		t.Errorf("discover() = %v", got)
	}
}

// TestProberForProfessionIsUsable exercises the prober the REGISTRY hands out, not one the
// test built for itself.
//
// It exists because every other test here constructs its own professionProber, and that is
// exactly the shape of hole that let a zero-value entry ship: `professionProber{}` carries
// a nil index and a nil client, so the first harvest panicked on a nil mutex before making
// a single request. Tests that build their own subject prove the type works and say nothing
// about the wiring.
func TestProberForProfessionIsUsable(t *testing.T) {
	p, ok := proberFor("profession")
	if !ok {
		t.Fatal("no prober registered for profession")
	}
	d, ok := p.(discoverer)
	if !ok {
		t.Fatal("the registered prober does not discover; harvest-boards would demand a seed file")
	}
	pp, ok := p.(professionProber)
	if !ok {
		t.Fatalf("registry holds %T, not professionProber", p)
	}
	if pp.index == nil {
		t.Error("the registered prober has no index; discover panics on a nil mutex")
	}
	if pp.client == nil {
		t.Error("the registered prober has no client of its own; it would read over the pooled one")
	}
	// A context already cancelled proves the call REACHES the transport rather than
	// panicking on the way — no request leaves the machine.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.discover(ctx, nil); err == nil {
		t.Error("discover on a cancelled context returned no error")
	}
}
