package main

import (
	"context"
	"slices"
	"testing"
)

func TestPaycorProbe(t *testing.T) {
	p := paycorProber{}
	const live = "8a7883c664f8df9e0165119e33353606"
	listing := `<html><body>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=aa11&amp;source=&amp;lang=en">Line Cook</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=aa11">Apply</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=bb22">Dishwasher</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `">Submit Resume</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b2456f3b601247ccb93d40fa2&amp;id=cc33">Elsewhere</a>
<a href="https://example.com/about">About</a>
</body></html>`
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + live: listing,
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=empty":   `<html><body><a href="/about">About</a></body></html>`,
	}
	// Live: two DISTINCT postings — the repeated link counts once, the id-less "submit your
	// resume" link is not a posting, and neither is one on another board. No name is published.
	if name, n, err := p.probe(context.Background(), getter, live); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unknown client id 404s, which the client surfaces as an error: not a live board.
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}

	ids, err := p.postingIDs(context.Background(), getter, live)
	if err != nil || !slices.Equal(ids, []string{"aa11", "bb22"}) {
		t.Errorf("postingIDs = %v, %v; want [aa11 bb22], nil", ids, err)
	}
}

func TestPaycorProberDedupKeyFoldsCase(t *testing.T) {
	if got := (paycorProber{}).dedupKey("8A7883C664F8DF9E"); got != "8a7883c664f8df9e" {
		t.Errorf("dedupKey = %q, want the lower-case client id", got)
	}
}
