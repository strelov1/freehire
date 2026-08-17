package main

import (
	"context"
	"testing"
)

func TestOracleProbe(t *testing.T) {
	p := oracleProber{}
	const live = "https://acme.fa.us2.oraclecloud.com/hcmRestApi/resources/latest/recruitingCEJobRequisitions?onlyData=true&finder=findReqs;siteNumber=CX,limit=1"
	const empty = "https://acme.fa.us2.oraclecloud.com/hcmRestApi/resources/latest/recruitingCEJobRequisitions?onlyData=true&finder=findReqs;siteNumber=empty,limit=1"
	getter := fakeGetter{
		live:  `{"items":[{"TotalJobsCount":42,"requisitionList":[{"Id":"1"}]}]}`,
		empty: `{"items":[{"TotalJobsCount":0,"requisitionList":[]}]}`,
	}
	// live board: Oracle exposes no employer name, so the prober returns count + empty name
	// (a seed-supplied company fills the label downstream).
	if name, n, err := p.probe(context.Background(), getter, "acme.fa.us2.oraclecloud.com/CX"); err != nil || name != "" || n != 42 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",42,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "acme.fa.us2.oraclecloud.com/empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// malformed board id (no "/site") => skip
	if _, n, err := p.probe(context.Background(), getter, "no-slash"); err != nil || n != 0 {
		t.Errorf("malformed: got n=%d err=%v, want 0,nil", n, err)
	}
	// absent board (getter error) => skip
	if _, n, err := p.probe(context.Background(), getter, "gone.fa.us2.oraclecloud.com/CX"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestJazzHRProbe(t *testing.T) {
	p := jazzhrProber{}
	listing := `<html><body>
<a href="/apply/abc123def/senior-engineer">Senior Engineer</a>
<a href="/apply/xyz789ghi/product-manager">Product Manager</a>
<a href="/apply/abc123def/senior-engineer">Senior Engineer (dup card link)</a>
<a href="/login">Login</a>
</body></html>`
	getter := fakeGetter{
		"https://acme.applytojob.com/apply":  listing,
		"https://empty.applytojob.com/apply": `<html><body><a href="/login">Login</a></body></html>`,
	}
	// live: two DISTINCT postings (the duplicate link counts once), no API name.
	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

// TestOracleDedupKey: Oracle serves the same board under any spelling of its site segment, so
// the dedup key must fold case — otherwise a candidate taken from a job URL lands beside the
// board we already crawl. Different site NAMES on one tenant stay different boards.
func TestOracleDedupKey(t *testing.T) {
	p := oracleProber{}
	same := []string{
		"eetz.fa.us6.oraclecloud.com/CX_2001",
		"eetz.fa.us6.oraclecloud.com/cx_2001",
		"EETZ.fa.us6.oraclecloud.com/Cx_2001",
	}
	want := p.dedupKey(same[0])
	for _, id := range same[1:] {
		if got := p.dedupKey(id); got != want {
			t.Errorf("dedupKey(%q) = %q, want %q", id, got, want)
		}
	}
	if p.dedupKey("eetz.fa.us6.oraclecloud.com/CX_1") == want {
		t.Error("CX_1 and CX_2001 are different sites, not different spellings of one")
	}
}
