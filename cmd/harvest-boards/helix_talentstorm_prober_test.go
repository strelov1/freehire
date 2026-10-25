package main

import (
	"context"
	"net/http"
	"slices"
	"testing"
)

// headerStatusGetter is statusGetter for a prober that reads JSON with request headers.
type headerStatusGetter struct{ statusGetter }

func (g headerStatusGetter) GetJSONWithHeaders(ctx context.Context, url string, _ map[string]string, v any) error {
	return g.GetJSON(ctx, url, v)
}

func TestHelixjobsProbe(t *testing.T) {
	list := func(iid string) string { return "https://helixjobs.com/_/joblist?iid=" + iid + "&page=1" }
	g := textStatusGetter{
		fakeGetter: fakeGetter{
			// iid 146's first page as captured 2026-10-25, trimmed to its result line and one line.
			list("146"): `<div class="result-text" role="status">Suchergebnis: 			<strong>62</strong> Treffer		</div>
<li class="joblist-item"><a href="/_/jobad?prj=146P13181"><h3>Fahrer</h3>
<ul><li class="company-name"><div class="svg-icon"></div>
<span>apetito AG</span></li></ul></a></li>`,
			list("95"): `<div class="result-text" role="status">Suchergebnis: <strong>2.899</strong> Treffer</div>`,
			// An iid with no posting, and one that does not exist, answer the same.
			list("1"): `<h4>Keine Ergebnisse gefunden. Bitte ändern Sie die Suchkriterien.</h4>`,
		},
		status: map[string]int{list("7"): http.StatusForbidden},
	}
	p := helixjobsProber{}
	if name, n, err := p.probe(context.Background(), g, "146"); err != nil || name != "apetito AG" || n != 62 {
		t.Errorf("live: got (%q,%d,%v), want (apetito AG,62,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), g, "95"); err != nil || n != 2899 {
		t.Errorf("thousands separator: got n=%d err=%v, want 2899", n, err)
	}
	for _, board := range []string{"1", "apetito-ag", "0"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "7"); err == nil {
		t.Error("a refusal must be surfaced, not read as an absent board")
	}
}

func TestHelixjobsDiscoverWalksThePortal(t *testing.T) {
	page := func(n string) string { return "https://helixjobs.com/_/joblist?page=" + n }
	g := fakeGetter{
		page("1"): `<a href="/_/jobad?prj=146P1"></a><a href="/_/jobad?prj=146P2"></a><a href="/_/jobad?prj=100007p36"></a>`,
		page("2"): `<a href="/_/jobad?prj=2728P663"></a>`,
		page("3"): `<h4>Keine Ergebnisse gefunden.</h4>`,
	}
	got, err := helixjobsProber{}.discover(context.Background(), g)
	if err != nil || !slices.Equal(got, []string{"146", "100007", "2728"}) {
		t.Fatalf("discover = %v, %v; want the iids in portal order", got, err)
	}
}

func TestTalentstormProbe(t *testing.T) {
	api := func(b string) string {
		return "https://api.talentstorm.de/api/v1/joboffers/basic/" + b + "?pagination=false"
	}
	g := headerStatusGetter{statusGetter{
		fakeGetter: fakeGetter{
			api("advades"): `[{"isPublished":true,"idJobtype":1,"jobofferLocations":[{"location":{"isHeadquarter":true,"label":"advades  GmbH"}}]},
{"isPublished":true,"idJobtype":4,"jobofferLocations":[]},
{"isPublished":true,"idJobtype":6,"jobofferLocations":[]}]`,
			api("empty"): `[]`,
			api("null"):  `null`,
		},
		status: map[string]int{
			api("nosuchsite"): http.StatusUnauthorized,
			api("refused"):    http.StatusForbidden,
		},
	}}
	p := talentstormProber{}
	if name, n, err := p.probe(context.Background(), g, "advades"); err != nil || name != "advades GmbH" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (advades GmbH,2,nil) — the open application not counted", name, n, err)
	}
	for _, board := range []string{"empty", "Bad/Label"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	// 401 answers an unknown label and a refused Origin alike, so it is surfaced, as is null.
	for _, board := range []string{"refused", "nosuchsite", "null"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced, not read as an absent board", board)
		}
	}
}
