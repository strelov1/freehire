package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// jsonStatusGetter serves canned bodies like fakeGetter and answers the listed status for a URL
// read as JSON.
type jsonStatusGetter struct {
	fakeGetter
	status map[string]int
}

func (g jsonStatusGetter) GetJSON(ctx context.Context, url string, v any) error {
	if code, ok := g.status[url]; ok {
		return &sources.StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	return g.fakeGetter.GetJSON(ctx, url, v)
}

func TestHROfficeProbe(t *testing.T) {
	g := jsonStatusGetter{
		fakeGetter: fakeGetter{
			// middendelfland's listing as captured 2026-10-25, trimmed to ids, and its home page's
			// site name.
			"https://middendelfland.inhroffice.com/api/nl/vacancies": `[{"id":391727},{"id":391710},{"id":317503}]`,
			"https://middendelfland.inhroffice.com/":                 `<html><head><meta property="og:site_name" content="Gemeente Midden-Delfland"></head></html>`,
			"https://c-job.inhroffice.com/api/nl/vacancies":          `[]`,
		},
		status: map[string]int{"https://amstelwijs.inhroffice.com/api/nl/vacancies": http.StatusInternalServerError},
	}
	p := hrofficeProber{}
	if name, n, err := p.probe(context.Background(), g, "middendelfland"); err != nil || name != "Gemeente Midden-Delfland" || n != 3 {
		t.Errorf("live: got (%q,%d,%v), want (Gemeente Midden-Delfland,3,nil)", name, n, err)
	}
	for _, board := range []string{"c-job", "amstelwijs", "Not A Label"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}
