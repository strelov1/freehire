package main

import (
	"context"
	"testing"
)

func TestHRMDirectProbe(t *testing.T) {
	p := hrmdirectProber{}
	page := func(siteID string) string {
		return `<html><body><div id="cc-widget">` +
			`<script src="https://careers-content.clearcompany.com/js/v1/career-site.js?hosted=true" data-site-id="` +
			siteID + `" data-default-source="CJB-0"></script></div></body></html>`
	}
	getter := fakeGetter{
		"https://apply.clearcompany.com/acme":                                       page("site-acme"),
		"https://careers-api.clearcompany.com/v1/site-acme?pageIndex=0&pageSize=1":  `{"results":[{"id":"a"}],"totalCount":64}`,
		"https://apply.clearcompany.com/empty":                                      page("site-empty"),
		"https://careers-api.clearcompany.com/v1/site-empty?pageIndex=0&pageSize=1": `{"results":[],"totalCount":0}`,
		// A career page that names no site cannot be listed, so it counts nothing.
		"https://apply.clearcompany.com/bare": `<html><body><main></main></body></html>`,
	}
	// Live: the count is the total the posting API states, not the one posting it returned.
	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || name != "" || n != 64 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",64,nil)", name, n, err)
	}
	for _, slug := range []string{"empty", "bare", "gone"} {
		// An empty, unreadable or nonexistent tenant is a skip, never a fatal error.
		if _, n, err := p.probe(context.Background(), getter, slug); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want 0,nil", slug, n, err)
		}
	}
}
