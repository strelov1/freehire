package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// talentsoftFixture reads one of the adapter's captured pages (adsn-career.talent-soft.com, and
// vinci-groupe.profils.org's empty result, 2026-10-28).
func talentsoftFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "internal", "ingest", "sources", "testdata", "talentsoft", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTalentsoftProbe(t *testing.T) {
	const listing = "/offre-de-emploi/liste-toutes-offres.aspx?all=1&mode=list"
	g := fakeGetter{
		"https://adsn-career.talent-soft.com" + listing: talentsoftFixture(t, "listing_p1.html"),
		"https://vinci-groupe.profils.org" + listing:    talentsoftFixture(t, "listing_empty.html"),
		"https://example.com" + listing:                 "<html><title>Hello - world</title><body>hi</body></html>",
	}
	p := talentsoftProber{}
	if name, n, err := p.probe(context.Background(), g, "ADSN-career.talent-soft.com/"); err != nil || name != "ADSN" || n != 11 {
		t.Errorf("live site: got (%q,%d,%v), want (ADSN,11,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "vinci-groupe.profils.org"); err != nil || n != 0 || name != "Groupe VINCI" {
		t.Errorf("empty site: got (%q,%d,%v), want (Groupe VINCI,0,nil)", name, n, err)
	}
	for _, board := range []string{"example.com", "https://x.talent-soft.com", "nodot"} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 || name != "" {
			t.Errorf("%q: got (%q,%d,%v), want nothing", board, name, n, err)
		}
	}
}
