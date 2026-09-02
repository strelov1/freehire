package main

import (
	"context"
	"testing"
)

func TestHiringThingProbe(t *testing.T) {
	p := hiringthingProber{}
	listing := `<html><body><div class="jobs-list-container">
<div class="job-container"><a href="/job/952170/mobile-shred-operator"><h2>Mobile Shred Operator</h2></a>
<a href="/job/952170/mobile-shred-operator">Learn more</a></div>
<div class="job-container"><a href="/job/1054668/registered-occupational-therapist">Therapist</a></div>
<a href="/privacy">Privacy</a>
<a href="/privacy?next=/job/1052705">Privacy notice</a>
</div></body></html>`
	getter := fakeGetter{
		// The board is the full careers host, and a reseller domain is as much a board as the
		// vendor's own — both are probed by the same one request to the site root.
		"https://crown-shredding-llc.prismhr-hire.com/": listing,
		"https://skijapan.hiringthing.com/":             listing,
		"https://empty.oasisrecruit.com/":               `<html><body><a href="/privacy">Privacy</a></body></html>`,
	}
	// live: the two links to one posting inflate the count, which is fine — the probe judges
	// liveness, not board size. The privacy link carrying an id in its query is NOT counted:
	// counting it would accept a board the ingest adapter cannot enumerate a posting from. The
	// name is empty by design (see hiringthingProber).
	if name, n, err := p.probe(context.Background(), getter, "crown-shredding-llc.prismhr-hire.com"); err != nil || name != "" || n != 3 {
		t.Errorf("live reseller board: got (%q,%d,%v), want (\"\",3,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "skijapan.hiringthing.com"); err != nil || n != 3 {
		t.Errorf("live vendor board: got n=%d err=%v, want 3,nil", n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty.oasisrecruit.com"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unreachable host is not live, and never a fatal error: one dead candidate must not
	// abort the harvest.
	if _, n, err := p.probe(context.Background(), getter, "gone.hiringthing.com"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}
