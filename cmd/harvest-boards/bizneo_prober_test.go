package main

import (
	"context"
	"testing"
)

func TestBizneoProbe(t *testing.T) {
	g := fakeGetter{
		// grupo-euroformac's first listing page as captured 2026-10-27, trimmed to the heading and
		// two of its ten cards.
		"https://grupo-euroformac.careers.ats.bizneo.cloud/jobs?page=1": `<html><head><meta property="og:title" content="Grupo Euroformac"></head><body>
<p class="subtitle">Tenemos <span>19 ofertas</span> y <span>21 vacantes</span> para ti</p>
<a class="job-card card-grid-image" href="https://grupo-euroformac.careers.ats.bizneo.cloud/jobs/docente-online-ssce0110-badajoz"></a>
<a class="job-card card-grid-image" href="https://grupo-euroformac.careers.ats.bizneo.cloud/jobs/responsable-innovacion-educativa"></a>
</body></html>`,
		// A customer's own domain: counted only with Bizneo's career-site bundle on the page.
		"https://emprego.imancorp.pt/jobs?page=1": `<html><head><meta property="og:title" content="Iman Corp"><script src="https://assets.bizneo.com/vite/assets/custom-companies-DEK45LIi.js"></script></head><body>
<a class="job-card" href="https://emprego.imancorp.pt/jobs/auxiliar-de-limpeza-bombarral"></a></body></html>`,
		"https://careers.example.com/jobs?page=1":              `<html><body><a class="job-card" href="/jobs/x"></a></body></html>`,
		"https://solturo.careers.ats.bizneo.cloud/jobs?page=1": `<html><head><meta property="og:title" content="Solturo"></head><body><p class="title">Ofertas de empleo</p></body></html>`,
	}
	p := bizneoProber{}
	if name, n, err := p.probe(context.Background(), g, "grupo-euroformac.careers.ats.bizneo.cloud"); err != nil || name != "Grupo Euroformac" || n != 19 {
		t.Errorf("live: got (%q,%d,%v), want (Grupo Euroformac,19,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "emprego.imancorp.pt"); err != nil || name != "Iman Corp" || n != 1 {
		t.Errorf("custom domain: got (%q,%d,%v), want (Iman Corp,1,nil)", name, n, err)
	}
	for _, board := range []string{"solturo.careers.ats.bizneo.cloud", "groundforce.bizneo.com", "acme", "careers.example.com"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}
