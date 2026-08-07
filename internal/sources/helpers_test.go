package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
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
