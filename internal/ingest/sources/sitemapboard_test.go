package sources

import (
	"context"
	"encoding/xml"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// sitemapBoardFake serves fixture files by URL; an unknown URL answers 404, the way a closed
// posting does.
type sitemapBoardFake struct {
	t                   *testing.T
	dir                 string
	files               map[string]string // URL → fixture name
	missingDetailStatus int               // defaults to 404
	mu                  sync.Mutex
	got                 []string
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
		if f.missingDetailStatus != 0 {
			return nil, &StatusError{Method: "GET", Code: f.missingDetailStatus, URL: url}
		}
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

func TestSitemapBoardFailsWhenEveryDetailIsUnreadable(t *testing.T) {
	f := &sitemapBoardFake{
		t: t, dir: "getinit", files: map[string]string{getinitSitemapURL: "sitemap.xml"},
		missingDetailStatus: http.StatusServiceUnavailable,
	}
	if _, err := fastGetInIT(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl whose every page failed must not report success to board health")
	}
}

func TestSitemapBoardAcceptsConfirmedGoneDetails(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := &sitemapBoardFake{
				t: t, dir: "getinit", files: map[string]string{getinitSitemapURL: "sitemap.xml"},
				missingDetailStatus: status,
			}
			jobs, err := fastGetInIT(f).Fetch(context.Background(), CompanyEntry{})
			if err != nil || len(jobs) != 0 {
				t.Fatalf("got %d jobs and %v, want empty success for confirmed removals", len(jobs), err)
			}
		})
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

func TestSitemapBoardPostingKeepsAPostingWhoseSalaryIsABareNumber(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<script type="application/ld+json">{"@type":"JobPosting","title":"Data Engineer","hiringOrganization":{"name":"Acme"},"baseSalary":150000}</script>`))
	if err != nil {
		t.Fatal(err)
	}
	var p sitemapBoardPosting
	if !ldJobPosting(root, &p) || p.Title != "Data Engineer" {
		t.Fatalf("posting with a numeric baseSalary was dropped: %+v", p)
	}
	if p.BaseSalary.amount != nil {
		t.Errorf("numeric baseSalary decoded as an amount: %+v", p.BaseSalary.amount)
	}
}

func TestLDJobPostingKeepsNoFieldsFromARejectedBlock(t *testing.T) {
	rejected := `<script type="application/ld+json">{"@type":"JobPosting","baseSalary":{"currency":"USD","value":{"minValue":9000}},"title":42}</script>`
	for name, accepted := range map[string]string{
		"numeric baseSalary": `{"@type":"JobPosting","title":"Data Engineer","baseSalary":150000}`,
		"no baseSalary":      `{"@type":"JobPosting","title":"Data Engineer"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root, err := html.Parse(strings.NewReader(rejected + `<script type="application/ld+json">` + accepted + `</script>`))
			if err != nil {
				t.Fatal(err)
			}
			var p sitemapBoardPosting
			if !ldJobPosting(root, &p) || p.Title != "Data Engineer" {
				t.Fatalf("accepted block not decoded: %+v", p)
			}
			if p.BaseSalary.amount != nil {
				t.Errorf("salary leaked from the rejected block: %+v", p.BaseSalary.amount)
			}
		})
	}
}
