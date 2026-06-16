package main

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeGetter decodes a canned JSON body per URL into v; an unmapped URL is an error,
// standing in for the real client's response to a missing/moved board.
type fakeGetter map[string]string

func (f fakeGetter) GetJSON(_ context.Context, url string, v any) error {
	body, ok := f[url]
	if !ok {
		return errMissing
	}
	return json.Unmarshal([]byte(body), v)
}

func TestGreenhouseProbe(t *testing.T) {
	g := greenhouseProber{}
	getter := fakeGetter{
		"https://boards-api.greenhouse.io/v1/boards/acme/jobs":  `{"jobs":[{"id":1},{"id":2}]}`,
		"https://boards-api.greenhouse.io/v1/boards/acme":       `{"name":"Acme Inc"}`,
		"https://boards-api.greenhouse.io/v1/boards/empty/jobs": `{"jobs":[]}`,
		// A board whose jobs endpoint works but metadata lacks a name falls back to the slug.
		"https://boards-api.greenhouse.io/v1/boards/noname/jobs": `{"jobs":[{"id":7}]}`,
		"https://boards-api.greenhouse.io/v1/boards/noname":      `{}`,
	}

	cases := []struct {
		slug     string
		wantName string
		wantN    int
	}{
		{"acme", "Acme Inc", 2},
		{"empty", "", 0},
		{"noname", "noname", 1},
		{"gone", "", 0}, // absent from greenhouse (getter error) => skip, not failure
	}
	for _, tc := range cases {
		name, n, err := g.probe(context.Background(), getter, tc.slug)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.slug, err)
		}
		if name != tc.wantName || n != tc.wantN {
			t.Errorf("%s: got (%q,%d), want (%q,%d)", tc.slug, name, n, tc.wantName, tc.wantN)
		}
	}
}

// The lever/ashby/bamboohr provers carry no company name in their payloads, so a live
// board's name falls back to its slug; an empty or absent board is a ("",0,nil) skip.
func TestSlugFallbackProbers(t *testing.T) {
	cases := []struct {
		name   string
		p      prober
		getter fakeGetter
		live   string // a slug that returns jobs
		empty  string // a slug that returns an empty board
	}{
		{
			name: "lever",
			p:    leverProber{},
			getter: fakeGetter{
				"https://api.lever.co/v0/postings/acme?mode=json":  `[{"id":"a"},{"id":"b"},{"id":"c"}]`,
				"https://api.lever.co/v0/postings/empty?mode=json": `[]`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "ashby",
			p:    ashbyProber{},
			getter: fakeGetter{
				"https://api.ashbyhq.com/posting-api/job-board/acme":  `{"jobs":[{"id":"a"},{"id":"b"}]}`,
				"https://api.ashbyhq.com/posting-api/job-board/empty": `{"jobs":[]}`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "bamboohr",
			p:    bamboohrProber{},
			getter: fakeGetter{
				"https://acme.bamboohr.com/careers/list":  `{"result":[{"id":"1"}]}`,
				"https://empty.bamboohr.com/careers/list": `{"result":[]}`,
			},
			live: "acme", empty: "empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Live board: name == slug, jobs > 0.
			name, n, err := tc.p.probe(context.Background(), tc.getter, tc.live)
			if err != nil || name != tc.live || n == 0 {
				t.Errorf("live: got (%q,%d,%v), want (%q,>0,nil)", name, n, err, tc.live)
			}
			// Empty board.
			if name, n, err := tc.p.probe(context.Background(), tc.getter, tc.empty); err != nil || name != "" || n != 0 {
				t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
			// Absent board (getter error) => skip.
			if name, n, err := tc.p.probe(context.Background(), tc.getter, "gone"); err != nil || name != "" || n != 0 {
				t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
		})
	}
}
