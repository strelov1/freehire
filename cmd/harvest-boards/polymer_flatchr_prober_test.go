package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// statusGetter serves canned bodies like fakeGetter and answers the listed status for a URL.
type statusGetter struct {
	fakeGetter
	status map[string]int
}

func (g statusGetter) GetJSON(ctx context.Context, url string, v any) error {
	if code, ok := g.status[url]; ok {
		return &sources.StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	return g.fakeGetter.GetJSON(ctx, url, v)
}

func TestPolymerProbe(t *testing.T) {
	const api = "https://api.polymer.co/v1/hire/organizations/"
	g := statusGetter{
		fakeGetter: fakeGetter{
			// motive's first page as captured 2026-10-24, items trimmed to one.
			api + "motive/jobs?page=1":   `{"items":[{"id":41656,"title":"Performance Manager"}],"meta":{"total":1,"is_last":true,"is_first":true,"page":1,"next_page":null,"count":4,"organization_name":"Motive"}}`,
			api + "inaza/jobs?page=1":    `{"items":[],"meta":{"total":0,"is_last":true,"count":0,"organization_name":"Inaza"}}`,
			api + "reshaped/jobs?page=1": `{"items":[]}`,
		},
		status: map[string]int{
			api + "alphafund/jobs?page=1": http.StatusUnprocessableEntity,
			api + "refused/jobs?page=1":   http.StatusForbidden,
		},
	}
	p := polymerProber{}
	if name, n, err := p.probe(context.Background(), g, "motive"); err != nil || name != "Motive" || n != 4 {
		t.Errorf("live: got (%q,%d,%v), want (Motive,4,nil) — the count is the board's, not the page's", name, n, err)
	}
	for _, board := range []string{"inaza", "alphafund", "../x"} {
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

func TestFlatchrProbe(t *testing.T) {
	const site = "https://careers.flatchr.io/company/"
	g := statusGetter{
		fakeGetter: fakeGetter{
			site + "additi.json": `{"items":[
				{"status":"published","published":true,"vacancy":{"company":{"name":"Medialex ","slug":"medialex"}}},
				{"status":"published","published":true,"vacancy":{"company":{"name":"Additi ","slug":"additi"}}},
				{"status":"draft","published":false,"vacancy":{"company":{"name":"Additi","slug":"additi"}}}]}`,
			site + "sub.json":       `{"items":[{"status":"published","published":true,"vacancy":{"company":{"name":"Other","slug":"other"}}}]}`,
			site + "accessite.json": `<section class="notFound"><div>Votre recherche est tombée à l’eau.</div></section>`,
			site + "broken.json":    `<html>maintenance</html>`,
			site + "reshaped.json":  `{"statusCode":200}`,
		},
		status: map[string]int{site + "nosuch.json": http.StatusNotFound},
	}
	p := flatchrProber{}
	if name, n, err := p.probe(context.Background(), g, "additi"); err != nil || name != "Additi" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (Additi,2,nil)", name, n, err)
	}
	// Only a subsidiary named: the employer is left to the seed rather than misnamed.
	if name, n, err := p.probe(context.Background(), g, "sub"); err != nil || name != "" || n != 1 {
		t.Errorf("group: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
	for _, board := range []string{"accessite", "nosuch", "Bad/x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"broken", "reshaped"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: a document the board cannot be read from must be surfaced", board)
		}
	}
}

func TestPolymerFlatchrDiscoveryRecognizesBoards(t *testing.T) {
	for _, c := range []struct{ provider, url, want string }{
		{"polymer", "https://jobs.polymer.co/anautics/21077", "anautics"},
		{"polymer", "https://jobs.polymer.co/alphafund/35653?source=LinkedIn", "alphafund"},
		{"flatchr", "https://careers.flatchr.io/fr/company/cder/vacancy/abc-x/", "cder"},
		{"flatchr", "https://careers.flatchr.io/fr/404-not-found", ""},
		{"flatchr", "https://jobs.polymer.co/anautics/21077", ""},
	} {
		got, ok := recognizedBoard(c.provider)(c.url)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("recognizedBoard(%s)(%s) = %q,%v, want %q", c.provider, c.url, got, ok, c.want)
		}
	}
}

func TestPolymerFlatchrProbersRegistered(t *testing.T) {
	for _, provider := range []string{"polymer", "flatchr"} {
		p, ok := proberFor(provider)
		if !ok {
			t.Fatalf("proberFor(%q) not found", provider)
		}
		if _, fellBack := p.(adapterProber); fellBack {
			t.Errorf("%s resolved to the adapter fallback, got %T", provider, p)
		}
		if _, ok := p.(discoverer); !ok {
			t.Errorf("%s prober cannot discover", provider)
		}
	}
}
