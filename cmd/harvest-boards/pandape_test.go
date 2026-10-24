package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"slices"
	"testing"
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
	if _, ok := probers["pandape"].(discoverer); !ok {
		t.Fatal(`probers["pandape"] should be a discoverer`)
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
	got, err := pandapeProber{}.discover(context.Background(), getter)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"basefacilities", "braspress", "loungerie"}
	if !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}

func TestPandapeDiscoverFailsWithoutAnIndex(t *testing.T) {
	if _, err := (pandapeProber{}).discover(context.Background(), fakeGetter{}); err == nil {
		t.Fatal("an unreadable sitemap index must fail discovery")
	}
}

func TestPandapeProbe(t *testing.T) {
	getter := fakeGetter{
		"https://braspress.pandape.infojobs.com.br/": `<html><head><title>Braspress - Trabalhe Conosco | Pandapé</title></head><body>
<div id="textFiltered"><span id="subTotal">54</span> Vagas encontradas com o filtro aplicado</div></body></html>`,
		"https://quiet.pandape.infojobs.com.br/": `<html><head><title>Quiet - Trabalhe Conosco | Pandapé</title></head><body><span id="subTotal">0</span></body></html>`,
	}
	cases := []struct {
		tenant, wantName string
		wantN            int
	}{
		{"braspress", "Braspress", 54},
		{"quiet", "", 0},
		{"missing", "", 0}, // the platform's 404 "não está mais disponível" page -> skip
	}
	for _, c := range cases {
		name, n, err := pandapeProber{}.probe(context.Background(), getter, c.tenant)
		if err != nil || name != c.wantName || n != c.wantN {
			t.Errorf("probe(%s) = (%q, %d, %v), want (%q, %d)", c.tenant, name, n, err, c.wantName, c.wantN)
		}
	}
}
