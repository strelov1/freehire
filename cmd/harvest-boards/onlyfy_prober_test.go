package main

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// resolvingGetter is fakeGetter that reports a redirect for the listed URL prefixes.
type resolvingGetter struct {
	fakeGetter
	redirect map[string]string // requested prefix -> landed prefix
}

func (g resolvingGetter) GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error) {
	for from, to := range g.redirect {
		if strings.HasPrefix(url, from) {
			root, err := html.Parse(strings.NewReader(`<script>$('#pageInfoTotalCount').text(3);</script>`))
			return root, to + strings.TrimPrefix(url, from), err
		}
	}
	root, err := g.GetHTML(ctx, url)
	return root, url, err
}

func TestOnlyfyProbe(t *testing.T) {
	const q = "/candidate/job/ajax_list?display_length=1&page=1&sort=date&sort_dir=DESC"
	g := fakeGetter{
		// jungheinrich's first row as captured 2026-10-25, trimmed.
		"https://jungheinrich.onlyfy.jobs" + q:                                 `<div><a href="/job/qqeva17cjz1xgg6jzmc9hw4jnsc68o7">Senior Cybersecurity Engineer</a></div><script>$('#pageInfoTotalCount').text(185);</script>`,
		"https://jungheinrich.onlyfy.jobs/job/qqeva17cjz1xgg6jzmc9hw4jnsc68o7": `<html><head><script type="application/ld+json">{"@type":"JobPosting","title":"x","hiringOrganization":{"name":"Jungheinrich AG"}}</script></head></html>`,
		"https://cms-rrh.onlyfy.jobs" + q:                                      `<div>There are currently no vacant positions.</div><script>$('#pageInfoTotalCount').text(0);</script>`,
		"https://reshaped.onlyfy.jobs" + q:                                     `<div>maintenance</div>`,
	}
	p := onlyfyProber{}
	if name, n, err := p.probe(context.Background(), g, "jungheinrich"); err != nil || name != "Jungheinrich AG" || n != 185 {
		t.Errorf("live: got (%q,%d,%v), want (Jungheinrich AG,185,nil)", name, n, err)
	}
	for _, board := range []string{"cms-rrh", "x.onlyfy.jobs"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"reshaped", "down"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced", board)
		}
	}
}

// A site that redirects to another name is that other site; probing it under the old name would
// store one site twice.
func TestOnlyfyProbeSkipsRenamedSite(t *testing.T) {
	g := resolvingGetter{redirect: map[string]string{"https://lloyd-fonds-ag.onlyfy.jobs/": "https://laiqon-ag.onlyfy.jobs/"}}
	if _, n, err := (onlyfyProber{}).probe(context.Background(), g, "lloyd-fonds-ag"); err != nil || n != 0 {
		t.Errorf("renamed site: got n=%d err=%v, want a skip", n, err)
	}
}
