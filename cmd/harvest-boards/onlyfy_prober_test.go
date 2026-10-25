package main

import (
	"context"
	"testing"
)

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
