package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// postStatusGetter serves canned bodies like fakeGetter and answers the listed status for a POST.
type postStatusGetter struct {
	fakeGetter
	status map[string]int
}

func (g postStatusGetter) PostJSON(ctx context.Context, url string, body, v any) error {
	if code, ok := g.status[url]; ok {
		return &sources.StatusError{Method: http.MethodPost, Code: code, URL: url}
	}
	return g.fakeGetter.PostJSON(ctx, url, body, v)
}

func TestGreytHRProbe(t *testing.T) {
	const base = "https://aaryak.greythr.com/hire/api/career/"
	g := postStatusGetter{
		fakeGetter: fakeGetter{
			// aaryak's listing and company details as captured 2026-10-25, trimmed.
			base + "published_jobs/":                                       `{"data":[{"id":"295402a8-ae03-4a5c-b598-f2d7169b3bf3","title":"Jewellery Consultant"}]}`,
			base + "get_company_details/":                                  `{"company_name":"AARYAK JEWELLERY PRIVATE LIMITED","company_logo":"x"}`,
			"https://emptyco.greythr.com/hire/api/career/published_jobs/":  `{"data":[]}`,
			"https://reshaped.greythr.com/hire/api/career/published_jobs/": `{"detail":"x"}`,
		},
		status: map[string]int{"https://nobody.greythr.com/hire/api/career/published_jobs/": http.StatusNotFound},
	}
	p := greythrProber{}
	if name, n, err := p.probe(context.Background(), g, "aaryak.greythr.com"); err != nil || name != "AARYAK JEWELLERY PRIVATE LIMITED" || n != 1 {
		t.Errorf("live: got (%q,%d,%v)", name, n, err)
	}
	for _, board := range []string{"nobody.greythr.com", "emptyco.greythr.com", "evil.example.com"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"reshaped.greythr.com", "refused.greythr.com"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced, not read as an absent board", board)
		}
	}
}
