package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"slices"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func gz(t *testing.T, s string) string {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestPandapeRegisteredAsDiscoverer(t *testing.T) {
	for _, p := range []string{"pandape", "pandapelatam"} {
		if _, ok := probers[p].(discoverer); !ok {
			t.Fatalf("probers[%q] should be a discoverer", p)
		}
	}
}

// Shapes trimmed from live responses (2026-10-23).
func TestPandapeDiscoverReadsEveryShardAndSkipsPlatformHosts(t *testing.T) {
	shard1 := "https://pandape.infojobs.com.br/pandapeofertas1.xml.gz"
	shard2 := "https://pandape.infojobs.com.br/pandapeofertas2.xml.gz"
	getter := fakeGetter{
		pandapeSitemapIndex: gz(t, `<?xml version="1.0" encoding="utf-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>`+shard1+`</loc><lastmod>2026-10-23</lastmod></sitemap>
<sitemap><loc>`+shard2+`</loc><lastmod>2026-10-23</lastmod></sitemap>
<sitemap><loc>https://pandape.infojobs.com.br/pandapeofertas3.xml.gz</loc></sitemap></sitemapindex>`),
		shard1: gz(t, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://basefacilities.pandape.infojobs.com.br/Detail/3861562</loc></url>
<url><loc>https://braspress.pandape.infojobs.com.br/Detail/3861560</loc></url>
<url><loc>https://basefacilities.pandape.infojobs.com.br/Detail/3861554</loc></url>
<url><loc>https://empregos.pandape.infojobs.com.br/Detail/3561691</loc></url>
<url><loc>/Detail/3552612</loc></url></urlset>`),
		// shard 3 is absent: a failed shard is skipped, never fatal.
		shard2: gz(t, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://loungerie.pandape.catho.com.br/Detail/3856149</loc></url>
<url><loc>https://braspress.pandape.infojobs.com.br/Detail/3838881</loc></url></urlset>`),
	}
	got, err := pandapeBrazilProber.discover(context.Background(), getter)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"basefacilities", "braspress", "loungerie"}
	if !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}

func TestPandapeDiscoverFailsWithoutAnIndex(t *testing.T) {
	if _, err := pandapeBrazilProber.discover(context.Background(), fakeGetter{}); err == nil {
		t.Fatal("an unreadable sitemap index must fail discovery")
	}
}

// pdStatusGetter answers listed URLs with a status error, the rest from the canned bodies.
type pdStatusGetter struct {
	fakeGetter
	codes map[string]int
}

func (g pdStatusGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if code, ok := g.codes[url]; ok {
		return nil, &sources.StatusError{Method: "GET", Code: code, URL: url}
	}
	return g.fakeGetter.GetHTML(ctx, url)
}

func TestPandapeProbe(t *testing.T) {
	getter := pdStatusGetter{fakeGetter: fakeGetter{
		"https://braspress.pandape.infojobs.com.br/Vacancies": `<html><head><title>Braspress - Trabalhe Conosco | Pandapé</title></head><body>
<div id="textFiltered"><span id="subTotal">54</span> Vagas encontradas com o filtro aplicado</div></body></html>`,
		"https://huge.pandape.infojobs.com.br/Vacancies":  `<html><head><title>Huge - Trabalhe Conosco | Pandapé</title></head><body><span id="subTotal">55483</span></body></html>`,
		"https://quiet.pandape.infojobs.com.br/Vacancies": `<html><head><title>Quiet - Trabalhe Conosco | Pandapé</title></head><body><span id="subTotal">0</span></body></html>`,
	}, codes: map[string]int{
		"https://missing.pandape.infojobs.com.br/Vacancies": 404,
		"https://refused.pandape.infojobs.com.br/Vacancies": 403,
	}}
	cases := []struct {
		tenant, wantName string
		wantN            int
		wantErr          bool
	}{
		{"braspress", "Braspress", 54, false},
		{"quiet", "", 0, false},
		{"huge", "", 0, false},    // past the pager's window: the crawl could never list it whole
		{"missing", "", 0, false}, // the platform's 404 "não está mais disponível" page -> skip
		{"refused", "", 0, true},  // the burst 403 is a probe that did not happen, not an absent board
	}
	for _, c := range cases {
		name, n, err := pandapeBrazilProber.probe(context.Background(), getter, c.tenant)
		if (err != nil) != c.wantErr || name != c.wantName || n != c.wantN {
			t.Errorf("probe(%s) = (%q, %d, %v), want (%q, %d, err=%v)", c.tenant, name, n, err, c.wantName, c.wantN, c.wantErr)
		}
	}
}

func TestPandapeHarvestHoldsTheCrawlPace(t *testing.T) {
	want := float64(time.Second) / float64(sources.PandapeRequestInterval)
	for _, flagPace := range []float64{0, 35, want} {
		if got := effectivePace(pandapeBrazilProber, flagPace); got != want {
			t.Errorf("effectivePace(pandape, %v) = %v, want %v", flagPace, got, want)
		}
	}
	if got := effectivePace(pandapeBrazilProber, want/2); got != want/2 {
		t.Errorf("a slower -pace must be kept, got %v", got)
	}
	if got := effectivePace(greenhouseProber{}, 0); got != 0 {
		t.Errorf("a prober with no ceiling stays unpaced, got %v", got)
	}
}

// The Latin-American list page, trimmed from avos-tech's /Vacancies (2026-10-24): a tenant on
// the richer home template, whose home page carries no counter at all.
func TestPandapeLatamProbeReadsTheListPage(t *testing.T) {
	getter := fakeGetter{
		"https://avos-tech.pandape.computrabajo.com/Vacancies": `<html><head><title>Avos Tech - Trabaja con nosotros | Pandapé</title></head><body>
<div id="textFiltered"><span id="subTotal">1241</span> Ofertas encontradas</div></body></html>`,
	}
	name, n, err := pandapeLatamProber.probe(context.Background(), getter, "avos-tech")
	if err != nil || name != "Avos Tech" || n != 1241 {
		t.Errorf("probe = (%q, %d, %v), want (Avos Tech, 1241, nil)", name, n, err)
	}
}

// Common Crawl names the Latin-American tenants: the platform's own hosts and the other
// deployment's URLs are not candidates.
func TestPandapeLatamDiscoverFromCommonCrawl(t *testing.T) {
	api := "https://index.commoncrawl.org/CC-MAIN-2026-43-index"
	q := api + "?url=" + pandapeLatamCrawlPattern + "&output=json"
	getter := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": `[{"id":"CC-MAIN-2026-43","cdx-api":"` + api + `"}]`,
		q + "&showNumPages=true&pageSize=1":           `{"pages":1}`,
		q + "&page=0&pageSize=1": `{"url": "https://netcall.pandape.computrabajo.com/Detail/13958406"}
{"url": "https://avos-tech.pandape.computrabajo.com/robots.txt"}
{"url": "https://empleos.pandape.computrabajo.com/Detail/9842801"}
{"url": "https://www.pandape.computrabajo.com/"}
{"url": "https://netcall.pandape.computrabajo.com/"}`,
	}
	got, err := pandapeLatamProber.discover(context.Background(), getter)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"avos-tech", "netcall"}; !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}

func TestPandapeLatamProbesWithTheCompatibleClient(t *testing.T) {
	if pandapeLatamProber.probeClient() == nil {
		t.Error("the Latin-American edge refuses the default client; the probe must replace it")
	}
	if pandapeBrazilProber.probeClient() != nil {
		t.Error("the Brazilian probe keeps the default egress")
	}
}
