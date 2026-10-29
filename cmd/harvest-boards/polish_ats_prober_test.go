package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// originGetter is fakeGetter for an API that answers by the request's Origin: bodies are keyed
// by origin + " " + url, and an unknown pair is the API's 404.
type originGetter struct{ fakeGetter }

func (g originGetter) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	body, ok := g.fakeGetter[headers["Origin"]+" "+url]
	if !ok {
		return &sources.StatusError{Method: http.MethodGet, Code: http.StatusNotFound, URL: url}
	}
	return json.Unmarshal([]byte(body), v)
}

func TestRecruitifyProbe(t *testing.T) {
	const (
		cfg  = "https://api.recruitify.ai/apply/config"
		list = "https://api.recruitify.ai/apply/project?page=1"
	)
	g := originGetter{fakeGetter{
		// As captured 2026-10-28, trimmed to the fields the probe reads.
		"https://linkgroup.recruitify.ai " + cfg:  `{"companyName":"Link Group","website":"www.linkgroup.pl"}`,
		"https://linkgroup.recruitify.ai " + list: `[{"guid":"f5fd636f-9efb-4c23-bd96-9943ed166cf0"},{"guid":"f4c758a7-8ea7-4919-a679-b6eecf39aa4c"}]`,
		"https://decerto.recruitify.ai " + cfg:    `{"companyName":"Decerto"}`,
		"https://decerto.recruitify.ai " + list:   `[]`,
	}}
	p := recruitifyProber{}
	for board, want := range map[string]struct {
		name string
		n    int
	}{
		"linkgroup": {"Link Group", 2},
		"decerto":   {"Decerto", 0},
	} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || name != want.name || n != want.n {
			t.Errorf("%s: got (%q,%d,%v), want (%q,%d,nil)", board, name, n, err, want.name, want.n)
		}
	}
	for _, board := range []string{"infogain", "Not A Label"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}

func TestErecruiterProbeSkipsWhatIsNotACfg(t *testing.T) {
	p := erecruiterProber{}
	for _, board := range []string{"inpost", "aaa769425b3f4db0aba90feb1a03c2d", "zzz769425b3f4db0aba90feb1a03c2d0"} {
		if _, n, err := p.probe(context.Background(), fakeGetter{}, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip before any request", board, n, err)
		}
	}
}
