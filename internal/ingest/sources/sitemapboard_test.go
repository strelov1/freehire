package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// sitemapBoardFake serves fixture files by URL; an unknown URL answers 404, the way a closed
// posting does.
type sitemapBoardFake struct {
	t     *testing.T
	dir   string
	files map[string]string // URL → fixture name
	mu    sync.Mutex
	got   []string
}

func (f *sitemapBoardFake) read(url string) ([]byte, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.got {
		if g == url {
			return true
		}
	}
	return false
}

func TestSitemapBoardFailsWhenNoRequestedPageYieldsAPosting(t *testing.T) {
	// The sitemap reads but every posting page answers 404: an outage, not an empty catalogue.
	f := &sitemapBoardFake{t: t, dir: "getinit", files: map[string]string{getinitSitemapURL: "sitemap.xml"}}
	if _, err := fastGetInIT(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl whose every page failed must not report success to board health")
	}
}

func TestSitemapBoardPlacesReadsEachPlacesAddressShapeOnItsOwn(t *testing.T) {
	var p sitemapBoardPlaces
	raw := `[{"address":{"addressLocality":"Berlin","addressCountry":"DE"}},
	         {"address":[{"addressLocality":"Hamburg","addressCountry":"DE"},{"addressLocality":"Wien","addressCountry":"AT"}]},
	         {"address":"nowhere"}]`
	if err := p.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	post := sitemapBoardPosting{JobLocation: p}
	if got := strings.Join(post.localities(), ","); got != "Berlin,Hamburg,Wien" {
		t.Errorf("localities = %q", got)
	}
	if got := strings.Join(countriesFromCodes(post.countryCodes()), ","); got != "de,at" {
		t.Errorf("countries = %q", got)
	}
}
