package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// sitemapBoardFake serves fixture files by URL; an unknown URL answers 404, the way a closed
// posting does.
type sitemapBoardFake struct {
	t     *testing.T
	dir   string
	files map[string]string // URL → fixture name
	got   []string
}

func (f *sitemapBoardFake) read(url string) ([]byte, error) {
	f.got = append(f.got, url)
	name, ok := f.files[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	b, err := os.ReadFile("testdata/" + f.dir + "/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return b, nil
}

func (f *sitemapBoardFake) GetXML(_ context.Context, url string, v any) error {
	b, err := f.read(url)
	if err != nil {
		return err
	}
	return xml.Unmarshal(b, v)
}

func (f *sitemapBoardFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	b, err := f.read(url)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(b)))
}

func (f *sitemapBoardFake) fetched(url string) bool {
	for _, g := range f.got {
		if g == url {
			return true
		}
	}
	return false
}
