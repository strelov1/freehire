package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"

	"golang.org/x/net/html"
)

// unreadableMarkers returns the postings an adapter marked Unreadable — what it yields in
// place of a detail it could not read, so the crawl reports a hole in its evidence rather
// than an absence (see Job.Unreadable).
func unreadableMarkers(jobs []Job) []Job {
	var out []Job
	for _, j := range jobs {
		if j.Unreadable {
			out = append(out, j)
		}
	}
	return out
}

// readPostings is unreadableMarkers' complement: the postings the crawl actually read.
func readPostings(jobs []Job) []Job {
	var out []Job
	for _, j := range jobs {
		if !j.Unreadable {
			out = append(out, j)
		}
	}
	return out
}

// fakeHTTP is a test HTTPClient: it records the requested URL and decodes a canned
// body, so adapter tests exercise field mapping without the network. Shared by every
// adapter test in this package.
type fakeHTTP struct {
	body       string
	err        error
	gotURL     string
	gotHeaders map[string]string
}

func (f *fakeHTTP) GetJSON(_ context.Context, url string, v any) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

// GetJSONResolved reports the requested URL as the final one: fakeHTTP models a plain
// response, and a test that needs a redirect uses routedHTTP.routeRedirect instead.
func (f *fakeHTTP) GetJSONResolved(_ context.Context, url string, v any) (string, error) {
	f.gotURL = url
	if f.err != nil {
		return url, f.err
	}
	return url, json.Unmarshal([]byte(f.body), v)
}

func (f *fakeHTTP) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.gotURL = url
	f.gotHeaders = headers
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func (f *fakeHTTP) PostJSONWithHeaders(_ context.Context, url string, headers map[string]string, _, v any) error {
	f.gotURL = url
	f.gotHeaders = headers
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func (f *fakeHTTP) GetXML(_ context.Context, url string, v any) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return xml.Unmarshal([]byte(f.body), v)
}

func (f *fakeHTTP) PostJSON(_ context.Context, url string, _, v any) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func (f *fakeHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.gotURL = url
	if f.err != nil {
		return nil, f.err
	}
	return html.Parse(strings.NewReader(f.body))
}

func (f *fakeHTTP) GetTextWithHeaders(_ context.Context, url string, headers map[string]string) (string, error) {
	f.gotURL = url
	f.gotHeaders = headers
	if f.err != nil {
		return "", f.err
	}
	return f.body, nil
}

func intPtr(v int) *int { return &v }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
