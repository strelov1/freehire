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
