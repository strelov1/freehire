package main

import (
	"context"
	"testing"
)

func TestTrinetHireProbe(t *testing.T) {
	getter := fakeGetter{
		"https://app.trinethire.com/companies/16150-automox-inc/jobs": `<html><head><title>Careers at Automox Inc.</title></head><body>
<table><tr class="job"><td><a href="/companies/16150-automox-inc/jobs/26497-senior-software-engineer-backend">Senior Software Engineer</a></td></tr>
<tr class="job"><td><a href="/companies/16150-automox-inc/jobs/13870-senior-product-manager">Senior Product Manager</a></td></tr></table>
<a href="/companies/16150-automox-inc/jobs">Jobs at Automox</a><a href="/privacy">Privacy</a></body></html>`,
		"https://app.trinethire.com/companies/1-empty/jobs": `<html><head><title>Careers at Empty</title></head><body><a href="/privacy">Privacy</a></body></html>`,
	}
	p := trinethireProber{}
	if name, n, err := p.probe(context.Background(), getter, "16150-automox-inc"); err != nil || name != "Automox Inc." || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"Automox Inc.\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "1-empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unreachable company is not live, and never a fatal error.
	if _, n, err := p.probe(context.Background(), getter, "99999999-gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestGoHireProbe(t *testing.T) {
	getter := fakeGetter{
		"https://jobs.gohire.io/dexerto-de5jlhjo/?page=1": `<html><head><title>Jobs at Dexerto</title></head><body>
<a class="gohire-job" href="https://jobs.gohire.io/dexerto-de5jlhjo/marketing-manager-304991/">Marketing Manager</a>
<a class="gohire-job" href="https://jobs.gohire.io/dexerto-de5jlhjo/video-editor-303923/">Video Editor</a>
<a href="https://jobs.gohire.io/dexerto-de5jlhjo/">Jobs</a><a href="https://gohire.io">GoHire</a></body></html>`,
		"https://jobs.gohire.io/empty-aaaaaaaa/?page=1": `<html><head><title>Jobs at Empty</title></head><body><a href="https://gohire.io">GoHire</a></body></html>`,
	}
	p := gohireProber{}
	if name, n, err := p.probe(context.Background(), getter, "dexerto-de5jlhjo"); err != nil || name != "Dexerto" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"Dexerto\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty-aaaaaaaa"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "gone-bbbbbbbb"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}
