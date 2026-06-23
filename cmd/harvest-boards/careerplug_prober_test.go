package main

import (
	"context"
	"testing"
)

func TestCareerPlugProbe(t *testing.T) {
	p := careerplugProber{}
	listing := `<html><body><div id="job_table">
<a href="/jobs/100"><span class="name">Server</span></a>
<a href="/jobs/100">Apply</a>
<a href="/jobs/200">Cook</a>
<a href="/about">About</a>
<a href="/jobs?page=2">Next</a>
</div></body></html>`
	getter := fakeGetter{
		"https://acme.careerplug.com/jobs":  listing,
		"https://empty.careerplug.com/jobs": `<html><body><a href="/about">About</a></body></html>`,
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
