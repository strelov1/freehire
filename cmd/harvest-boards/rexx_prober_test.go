package main

import (
	"context"
	"testing"
)

func TestRexxProbe(t *testing.T) {
	const base = "https://acme-portal.rexx-systems.com/stellenangebote.html?start="
	// Two pages (each linking the posting set under a fresh session id), then a page with
	// nothing new; the postings are linked on the employer's own domain.
	page := func(ids ...string) string {
		s := "<html><body>"
		for _, id := range ids {
			s += `<a href="https://jobs.acme.de/Dev-de-j` + id + `.html?sid=x` + id + `">x</a>`
		}
		return s + `<a href="/jobalert.html">alert</a></body></html>`
	}
	g := fakeGetter{
		base + "0":                            page("1", "2", "2"),
		base + "100":                          page("3"),
		base + "200":                          page("3"),
		"https://jobs.acme.de/Dev-de-j1.html": `<html><head><script type="application/ld+json">{"@type":"JobPosting","title":"Dev","hiringOrganization":{"name":"Acme GmbH Jobportal"}}</script></head></html>`,
		// A portal that renders no JobPosting.
		"https://bare-portal.rexx-systems.com/stellenangebote.html?start=0":   page("7"),
		"https://bare-portal.rexx-systems.com/stellenangebote.html?start=100": page("7"),
		"https://jobs.acme.de/Dev-de-j7.html":                                 `<html><body><h1>Dev</h1></body></html>`,
		// A portal with no postings.
		"https://empty-portal.rexx-systems.com/stellenangebote.html?start=0": page(),
	}
	p := rexxProber{}
	if name, n, err := p.probe(context.Background(), g, "acme-portal.rexx-systems.com"); err != nil || name != "Acme GmbH" || n != 3 {
		t.Errorf("live: got (%q,%d,%v), want (Acme GmbH,3,nil)", name, n, err)
	}
	for _, board := range []string{"bare-portal.rexx-systems.com", "empty-portal.rexx-systems.com", "acme", "https://x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	// An unreachable portal is surfaced, not read as an absent one.
	if _, _, err := p.probe(context.Background(), g, "down-portal.rexx-systems.com"); err == nil {
		t.Error("an unreachable portal must be surfaced")
	}
}
