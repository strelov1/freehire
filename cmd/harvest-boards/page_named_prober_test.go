package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

func TestPageNamedProbeNamesALiveBoardFromItsPage(t *testing.T) {
	src := &companyGatedSource{live: map[string]int{"walnutfolks.zohorecruit.in": 3}}
	p := pageNamedProber{
		adapter: adapterProber{provider: "gated", newSource: func() sources.Source { return src }},
		nameURL: func(b string) string { return "https://" + b + "/jobs/Careers" },
		name:    func(root *html.Node) string { return metaProperty(root, "og:site_name") },
	}
	g := fakeGetter{
		// walnutfolks' careers page head as captured 2026-10-25.
		"https://walnutfolks.zohorecruit.in/jobs/Careers": `<html><head><title>Careers at Walnut Folks Group - Confluencr &amp; Intent Farm</title>
<meta property="og:site_name" content="Walnut Folks"></head></html>`,
	}
	if name, n, err := p.probe(context.Background(), g, "walnutfolks.zohorecruit.in"); err != nil || name != "Walnut Folks" || n != 3 {
		t.Errorf("live: got (%q,%d,%v), want (Walnut Folks,3,nil)", name, n, err)
	}
	// A live board whose page cannot be read is still live, just unnamed.
	src.live["nameless.zohorecruit.in"] = 1
	if name, n, err := p.probe(context.Background(), g, "nameless.zohorecruit.in"); err != nil || name != "" || n != 1 {
		t.Errorf("nameless: got (%q,%d,%v), want a live unnamed board", name, n, err)
	}
	// An empty board costs no page request and is a skip.
	if _, n, err := p.probe(context.Background(), g, "empty.zohorecruit.in"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want a skip", n, err)
	}
}

func TestPageNamedProbeSurfacesAnAdapterFailure(t *testing.T) {
	p := pageNamedProber{
		adapter: adapterProber{provider: "failing", newSource: func() sources.Source { return failingSource{} }},
		nameURL: func(b string) string { return "https://" + b },
		name:    pageTitle,
	}
	if _, _, err := p.probe(context.Background(), fakeGetter{}, "x"); err == nil {
		t.Error("an adapter failure must be surfaced, not read as an empty board")
	}
}

// The three platforms whose adapters take the employer from the catalogue row must not fall back
// to the unnamed adapter probe, which stores the board id as the company.
func TestNamingProbersRegistered(t *testing.T) {
	for _, provider := range []string{"darwinbox", "zohorecruit", "pyjamahr"} {
		p, ok := proberFor(provider)
		if !ok {
			t.Fatalf("proberFor(%q) not found", provider)
		}
		if _, named := p.(pageNamedProber); !named {
			t.Errorf("proberFor(%q) = %T, want pageNamedProber", provider, p)
		}
	}
	if p, _ := proberFor("keka"); p == nil {
		t.Error("keka must have a prober")
	} else if _, ok := p.(kekaProber); !ok {
		t.Errorf("proberFor(keka) = %T, want kekaProber", p)
	}
}
