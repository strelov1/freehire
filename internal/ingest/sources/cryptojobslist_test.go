package sources

import (
	"context"
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// cryptojobslistFixtures holds the trimmed live responses the tests serve.
//
//go:embed testdata/cryptojobslist
var cryptojobslistFixtures embed.FS

// cryptojobslistFake serves fixtures trimmed from the live site (2026-11-17): the jobs sitemap
// with four postings and a non-posting URL, and posting pages carrying the Next.js state. A
// posting with no page fixture answers 404, as a removed one does.
type cryptojobslistFake struct {
	pageErr error
	pages   map[string]string

	mu      sync.Mutex
	fetched []string
}

const (
	cjlSlugDirector = "director-of-research-at-stellar-development-foundation"
	cjlSlugContract = "smart-contract-auditor-rust-remote-at-sigintzero"
	cjlSlugFilled   = "filled-role-at-acme"
	cjlSlugSeen     = "old-seen-role-at-acme"
)

func (f *cryptojobslistFake) GetXML(_ context.Context, u string, v any) error {
	if u != cryptojobslistSitemapURL {
		return fmt.Errorf("cryptojobslistFake: no sitemap %s", u)
	}
	raw, err := cryptojobslistFixtures.ReadFile("testdata/cryptojobslist/sitemap.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *cryptojobslistFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	pages := f.pages
	if pages == nil {
		pages = map[string]string{
			cjlSlugDirector: "posting.html",
			cjlSlugContract: "contract.html",
			cjlSlugFilled:   "filled.html",
		}
	}
	file, ok := pages[strings.TrimPrefix(u, cryptojobslistBaseURL+cryptojobslistJobPrefix)]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := cryptojobslistFixtures.ReadFile("testdata/cryptojobslist/" + file)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestCryptoJobsList(f *cryptojobslistFake) cryptojobslist {
	s := NewCryptoJobsList(f, f).(cryptojobslist)
	s.limiter = noWait{}
	return s
}

func TestCryptoJobsListHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &cryptojobslistFake{}
	jobs, err := newTestCryptoJobsList(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == cjlSlugSeen })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// director + contract read; the filled posting is dropped; the seen one only refreshed.
	if len(jobs) != 3 {
		t.Fatalf("jobs = %+v, want two hydrated and one refreshed", jobs)
	}
	if r := byID[cjlSlugSeen]; !r.SeenRefresh || r.Description != "" || r.URL != cryptojobslistBaseURL+"/jobs/"+cjlSlugSeen {
		t.Errorf("seen posting = %+v", r)
	}
	for _, u := range f.fetched {
		if strings.HasSuffix(u, cjlSlugSeen) {
			t.Errorf("a stored posting's page was requested: %s", u)
		}
	}

	d := byID[cjlSlugDirector]
	if d.Title != "Director of Research" || d.Company != "Stellar Development Foundation" || d.Location != "" {
		t.Errorf("identity = %q / %q / %q", d.Title, d.Company, d.Location)
	}
	if d.URL != cryptojobslistBaseURL+"/jobs/"+cjlSlugDirector || !strings.Contains(d.Description, "open technology") {
		t.Errorf("url/description = %q / %q", d.URL, d.Description)
	}
	if !d.Remote || d.WorkMode != "remote" || d.EmploymentType != "full_time" {
		t.Errorf("remote/mode/type = %v / %q / %q", d.Remote, d.WorkMode, d.EmploymentType)
	}
	if d.SalaryMin == nil || *d.SalaryMin != 210000 || *d.SalaryMax != 365000 || d.SalaryCurrency != "USD" || d.SalaryPeriod != "year" {
		t.Errorf("salary = %v %v %q %q", d.SalaryMin, d.SalaryMax, d.SalaryCurrency, d.SalaryPeriod)
	}
	if d.PostedAt == nil || !d.PostedAt.Equal(time.Date(2026, 1, 21, 4, 0, 9, 745e6, time.UTC)) {
		t.Errorf("posted = %v", d.PostedAt)
	}

	c := byID[cjlSlugContract]
	if c.Company != "SigIntZero" || c.EmploymentType != "contract" || c.SalaryMin != nil || !strings.HasPrefix(c.Location, "New York") {
		t.Errorf("contract posting = %+v", c)
	}
}

func TestCryptoJobsListListsNewestFirstAndSkipsNonPostings(t *testing.T) {
	ps, err := newTestCryptoJobsList(&cryptojobslistFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.slug)
	}
	want := []string{cjlSlugDirector, cjlSlugContract, cjlSlugFilled, cjlSlugSeen}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("slugs = %v, want %v", got, want)
	}
}

// Every posting page failing is an outage, not a quiet source: the crawl must fail.
func TestCryptoJobsListFailsWhenNoPageAnswers(t *testing.T) {
	f := &cryptojobslistFake{pageErr: &StatusError{Method: "GET", Code: 403, URL: "x"}}
	_, err := newTestCryptoJobsList(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "none answered") {
		t.Fatalf("err = %v, want none answered", err)
	}
}

// A challenge page answers 200 without the state: no answer, never "the posting is gone".
func TestCryptoJobsListChallengePageIsNoAnswer(t *testing.T) {
	f := &cryptojobslistFake{pages: map[string]string{cjlSlugDirector: "blocked.html"}}
	s := newTestCryptoJobsList(f)
	_, ok, err := s.detail(context.Background(), cryptojobslistPosting{slug: cjlSlugDirector, url: cryptojobslistBaseURL + "/jobs/" + cjlSlugDirector})
	if ok || !errors.Is(err, errCryptojobslistNoPosting) {
		t.Fatalf("ok=%v err=%v, want errCryptojobslistNoPosting", ok, err)
	}
}

type cryptojobslistEmptySitemap struct{}

func (cryptojobslistEmptySitemap) GetXML(context.Context, string, any) error { return nil }

func TestCryptoJobsListEmptySitemapFails(t *testing.T) {
	s := newTestCryptoJobsList(&cryptojobslistFake{})
	s.sitemap = cryptojobslistEmptySitemap{}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a sitemap listing no posting must fail the crawl")
	}
}
