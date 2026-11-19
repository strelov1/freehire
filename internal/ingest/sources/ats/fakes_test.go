package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// fakeHTTP is a test JSONGetter: it records the requested URL and decodes a canned body,
// or fails with err. sources' own fakeHTTP serves the adapters still in that package.
type fakeHTTP struct {
	body   string
	err    error
	gotURL string
}

func (f *fakeHTTP) GetJSON(_ context.Context, url string, v any) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

// routedHTTP is a test JSONPoster that answers each request with the body of the first route
// whose match is a substring of the URL, and fails a URL no route matches.
type routedHTTP struct {
	mu     sync.Mutex
	routes []struct{ match, body string }
}

func (r *routedHTTP) route(match, body string) *routedHTTP {
	r.routes = append(r.routes, struct{ match, body string }{match, body})
	return r
}

func (r *routedHTTP) PostJSON(_ context.Context, url string, _, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return json.Unmarshal([]byte(rt.body), v)
		}
	}
	return fmt.Errorf("routedHTTP: no route for %s", url)
}
