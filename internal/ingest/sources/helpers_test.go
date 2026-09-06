package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestCountryFromCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"alpha-2 lowercase", "ae", []string{"ae"}},
		{"alpha-2 uppercase", "AE", []string{"ae"}},
		{"alpha-3 as Ashby sends it", "USA", []string{"us"}},
		{"empty", "", nil},
		{"unresolved code outside the curated set", "ZZZ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countryFromCode(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("countryFromCode(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// A detail request that failed tells the crawl one of two very different things, and the whole
// stale-job sweep hangs off which: 404/410 is the platform stating the posting is gone, and
// anything else is this crawl having failed to see a posting that may well still be there.
func TestDetailUnreadable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the platform reports the posting gone", &StatusError{Code: 404}, false},
		{"the platform reports the posting withdrawn", &StatusError{Code: 410}, false},
		{"the origin refuses this address", &StatusError{Code: 403}, true},
		{"rate limited past the client's own retries", &StatusError{Code: 429}, true},
		{"the origin is broken", &StatusError{Code: 502}, true},
		{"an unexpected status says nothing about the posting", &StatusError{Code: 401}, true},
		{"a transport failure", errors.New("dial tcp: i/o timeout"), true},
		{"a status wrapped by an adapter", fmt.Errorf("detail: %w", &StatusError{Code: 404}), false},
		{"a decode failure", fmt.Errorf("sources: decode %s: %w", "https://x/y", errors.New("EOF")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detailUnreadable(tc.err); got != tc.want {
				t.Errorf("detailUnreadable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

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
