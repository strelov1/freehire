package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// textStatusGetter serves canned bodies like fakeGetter and answers the listed status for a
// URL read as text.
type textStatusGetter struct {
	fakeGetter
	status map[string]int
}

func (g textStatusGetter) GetText(ctx context.Context, url string) (string, error) {
	if code, ok := g.status[url]; ok {
		return "", &sources.StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	return g.fakeGetter.GetText(ctx, url)
}

func TestCovetoProbe(t *testing.T) {
	g := fakeGetter{
		// k60360's feed as captured 2026-10-25, items trimmed to two.
		"https://k60360.coveto.de/public/rss/": `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Aktuelle Jobs bei Stadtmission Nürnberg e.V.</title>
<item><title><![CDATA[Koch*Köchin (m/w/d)]]></title><guid><![CDATA[https://k60360.coveto.de/job-id-1937.html]]></guid></item>
<item><title><![CDATA[Gebietsmanager (m/w/d)]]></title><guid><![CDATA[https://k60360.coveto.de/job-id-1934.html]]></guid></item>
</channel></rss>`,
		"https://k10533.coveto.de/public/rss/": `<?xml version="1.0"?><rss version="2.0"><channel><title>Aktuelle Jobs bei Muster GmbH</title></channel></rss>`,
		// A switched-off tenant's notice, served under HTTP 200 in place of the feed.
		"https://k29972.coveto.de/public/rss/": `<!DOCTYPE html><html><head><title>Diese Seite ist nicht mehr aktiv.</title></head><body></body></html>`,
	}
	p := covetoProber{}
	if name, n, err := p.probe(context.Background(), g, "k60360"); err != nil || name != "Stadtmission Nürnberg e.V." || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (Stadtmission Nürnberg e.V.,2,nil)", name, n, err)
	}
	// k99999 is unrouted, the shape of a customer number that does not resolve.
	for _, board := range []string{"k10533", "k29972", "k99999", "www", "stadtmission"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}

func TestConcludisProbe(t *testing.T) {
	const list = "/prj/lst/a181a603769c1f98ad927e7367c7aa51/GesamtlisteOffenePositionen.htm"
	g := textStatusGetter{
		fakeGetter: fakeGetter{
			"https://hhla.concludis.de" + list: `<meta name="keywords" content="Hamburger Hafen und Logistik AG, Stellenbörse, Jobbörse"/>
<div class="stellensum">46 Stellen gefunden</div>`,
			// The keywords as buenting spells them, padded with byte-order marks.
			"https://buenting.concludis.de" + list: "<meta name=\"keywords\" content=\"J. Bünting \ufeff\ufeffBeteiligungs\ufeff\ufeff AG, Stellenbörse\"/>\n" +
				`<div class="stellensum">1 Stelle gefunden</div>`,
			"https://edeka-rr.concludis.de" + list: `<meta name="keywords" content="edeka-rr, Stellenbörse"/>
<div class="stellensum">668 Stellen gefunden</div>`,
			"https://dlr.concludis.de" + list: `<div class="stellensum">Derzeit haben wir keine offenen Stellen.</div>`,
			// A tenant publishing on its own site: the redirect lands on a page with no count.
			"https://edeka.concludis.de" + list: `<html><title>Stellenbörse | EDEKA Verbund</title></html>`,
		},
		status: map[string]int{
			"https://nonexist.concludis.de" + list: http.StatusNotFound,
			"https://refused.concludis.de" + list:  http.StatusForbidden,
		},
	}
	p := concludisProber{}
	for board, want := range map[string]struct {
		name string
		n    int
	}{
		"hhla":     {"Hamburger Hafen und Logistik AG", 46},
		"buenting": {"J. Bünting Beteiligungs AG", 1},
		"edeka-rr": {"", 668}, // the keywords name only the label, which is no company
	} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || name != want.name || n != want.n {
			t.Errorf("%s: got (%q,%d,%v), want (%q,%d,nil)", board, name, n, err, want.name, want.n)
		}
	}
	for _, board := range []string{"dlr", "edeka", "nonexist", "Bad/Label"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "refused"); err == nil {
		t.Error("a refusal must be surfaced, not read as an absent board")
	}
}

func TestKenjoProbe(t *testing.T) {
	url := func(b string) string {
		return "https://" + b + ".kenjo.io/api/controller/career-site/public/" + b + "/positions"
	}
	g := statusGetter{
		fakeGetter: fakeGetter{
			url("ict-ds"):   `{"active":true,"companyName":"ICT Digital Solutions GmbH ","activePositions":[{"_id":"a"},{"_id":"b"},{"_id":"c"},{"_id":"d"}]}`,
			url("kintyre"):  `{"active":true,"companyName":"Kintyre","activePositions":[]}`,
			url("reshaped"): `{"active":true,"companyName":"X"}`,
		},
		status: map[string]int{
			url("ampolon"): http.StatusNotFound,
			url("refused"): http.StatusForbidden,
		},
	}
	p := kenjoProber{}
	if name, n, err := p.probe(context.Background(), g, "ict-ds"); err != nil || name != "ICT Digital Solutions GmbH" || n != 4 {
		t.Errorf("live: got (%q,%d,%v), want (ICT Digital Solutions GmbH,4,nil)", name, n, err)
	}
	for _, board := range []string{"kintyre", "ampolon", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"refused", "reshaped"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced, not read as an absent board", board)
		}
	}
}

func TestMeinCheckInProbe(t *testing.T) {
	g := textStatusGetter{
		fakeGetter: fakeGetter{
			// ksk-ratzeburg's start page as captured 2026-10-25, its menu trimmed to three entries.
			"https://www.mein-check-in.de/ksk-ratzeburg": `<title>Kreissparkasse Herzogtum Lauenburg Onlinebewerbung</title>
<a class="mainmenu_position_link" href="/ksk-ratzeburg/position-159">
	Initiativbewerbung					         </a>
<a class="mainmenu_position_link" href="/ksk-ratzeburg/position-60759">
	(Junior-) Kreditanalyst (*gn) Groß- und Spezialgeschäft					         </a>
<a class="mainmenu_position_link" href="/ksk-ratzeburg/position-156">
	Betriebspraktikum					         </a>`,
		},
		status: map[string]int{"https://www.mein-check-in.de/no-such-tenant": http.StatusNotFound},
	}
	p := meincheckinProber{}
	if name, n, err := p.probe(context.Background(), g, "ksk-ratzeburg"); err != nil || name != "Kreissparkasse Herzogtum Lauenburg" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (Kreissparkasse Herzogtum Lauenburg,2,nil)", name, n, err)
	}
	for _, board := range []string{"no-such-tenant", "Bad Board"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}
