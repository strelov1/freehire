package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live torre.ai responses fetched 2026-10-15: the sitemap index and
// two of its sitemaps (cut to the postings below), and each posting page's <title> and ld+json
// blocks verbatim. post_closed.html is a closed posting the sitemap still named; its live page
// carries no JobPosting at all.

const (
	torreRemoteURL   = "https://torre.ai/post/KWN1DZKr-lingua-resources-senior-full-stack-engineer"
	torreOnsiteURL   = "https://torre.ai/post/xWpM2kAw-viisglobal-mechanical-design-engineer"
	torreAnywhereURL = "https://torre.ai/post/6WZzn86W-dayone-staffing-inc-speech-language-pathologist-school"
	torreUnicodeURL  = "https://torre.ai/post/lWgMzjyd-gotofrcom-sales-lead-b2b-consulting-business-growth"
	torreEscapedURL  = "https://torre.ai/post/kWR9eKQr-blue-recruitment-personnel-hgv-class-1-tramper-driver"
	torreClosedURL   = "https://torre.ai/post/qdPeQ9Pr-activeone-manual-qa-engineer"
)

type torreFake struct {
	t     *testing.T
	files map[string]string // URL → fixture name
	errs  map[string]error
	// mu guards got, which the detail workers append to concurrently.
	mu  sync.Mutex
	got []string
}

func newTorreFake(t *testing.T) *torreFake {
	return &torreFake{t: t, files: map[string]string{
		torreSitemapIndexURL:                   "sitemap-index.xml",
		"https://torre.ai/sitemap-en-1656.xml": "sitemap-en-1656.xml",
		"https://torre.ai/sitemap-es-155.xml":  "sitemap-es-155.xml",
		torreRemoteURL:                         "post_remote.html",
		torreOnsiteURL:                         "post_onsite.html",
		torreAnywhereURL:                       "post_anywhere.html",
		torreUnicodeURL:                        "post_unicode.html",
		torreEscapedURL:                        "post_escaped.html",
		torreClosedURL:                         "post_closed.html",
	}}
}

func (f *torreFake) read(url string) ([]byte, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	name, ok := f.files[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	b, err := os.ReadFile("testdata/torre/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return b, nil
}

func (f *torreFake) GetXML(_ context.Context, url string, v any) error {
	b, err := f.read(url)
	if err != nil {
		return err
	}
	return xml.Unmarshal(b, v)
}

func (f *torreFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	b, err := f.read(url)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(b)))
}

func torreFetch(t *testing.T, f *torreFake) map[string]Job {
	t.Helper()
	jobs, err := NewTorre(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestTorreListsEveryShardOnceAndSkipsAGoneShard(t *testing.T) {
	f := newTorreFake(t)
	list, err := NewTorre(f).(torre).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Six distinct postings; the Spanish shard repeats one the English shard names, and the
	// index's sitemap--2.xml answers 404.
	if len(list) != 6 {
		t.Fatalf("listed %d, want 6: %+v", len(list), list)
	}
	if list[0].ID != "KWN1DZKr" || list[0].URL != torreRemoteURL {
		t.Errorf("first = %+v", list[0])
	}
}

func TestTorreShardFailureFailsTheRun(t *testing.T) {
	f := newTorreFake(t)
	f.errs = map[string]error{"https://torre.ai/sitemap-es-155.xml": &StatusError{Method: "GET", Code: 503}}
	if _, err := NewTorre(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a shard that could not be read must fail the run, or the sweep closes its postings")
	}
	f.errs = map[string]error{torreSitemapIndexURL: errors.New("boom")}
	if _, err := NewTorre(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when the index fails")
	}
}

func TestTorreMapsTheJobPosting(t *testing.T) {
	jobs := torreFetch(t, newTorreFake(t))
	// The closed posting carries no JobPosting and is deferred.
	if len(jobs) != 5 {
		t.Fatalf("jobs = %d, want 5", len(jobs))
	}
	if _, ok := jobs["qdPeQ9Pr"]; ok {
		t.Error("a page without a JobPosting was stored")
	}

	on := jobs["xWpM2kAw"]
	if on.Title != "Mechanical Design Engineer" || on.Company != "ViisGlobal" || on.URL != torreOnsiteURL ||
		on.Location != "Butte, Montana, United States" || on.Remote || on.EmploymentType != "full_time" {
		t.Errorf("onsite: %+v", on)
	}
	if len(on.Countries) != 1 || on.Countries[0] != "us" {
		t.Errorf("onsite countries = %v", on.Countries)
	}
	if on.SalaryMin == nil || *on.SalaryMin != 90000 || on.SalaryMax == nil || *on.SalaryMax != 130000 ||
		on.SalaryCurrency != "USD" || on.SalaryPeriod != "year" {
		t.Errorf("onsite salary: %v-%v %s/%s", on.SalaryMin, on.SalaryMax, on.SalaryCurrency, on.SalaryPeriod)
	}
	if on.PostedAt == nil || on.PostedAt.Format("2006-01-02") != "2026-09-18" {
		t.Errorf("posted = %v", on.PostedAt)
	}

	rem := jobs["KWN1DZKr"]
	if !rem.Remote || rem.WorkMode != "remote" || !strings.HasPrefix(rem.Location, "Remote, Portugal; Spain") {
		t.Errorf("remote: %q remote=%v", rem.Location, rem.Remote)
	}
	if len(rem.Countries) != 16 || rem.Countries[0] != "pt" {
		t.Errorf("remote countries = %v", rem.Countries)
	}
	// A hidden salary carries the 1-1000 placeholder, never the employer's figures.
	if rem.SalaryMin != nil || rem.SalaryMax != nil || rem.SalaryCurrency != "" {
		t.Errorf("placeholder salary published: %v-%v %s", rem.SalaryMin, rem.SalaryMax, rem.SalaryCurrency)
	}

	anywhere := jobs["6WZzn86W"]
	if anywhere.Location != "Remote, Worldwide" || len(anywhere.Countries) != 0 {
		t.Errorf("anywhere: %q %v (the 49-country filler must not be read)", anywhere.Location, anywhere.Countries)
	}

	uni := jobs["lWgMzjyd"]
	if strings.Contains(uni.Description, `\u`) || strings.Contains(uni.Description, `\/`) ||
		!strings.Contains(uni.Description, "₹35,000") {
		t.Errorf("double-escaped body not restored: %.400s", uni.Description)
	}

	esc := jobs["kWR9eKQr"]
	if strings.Contains(esc.Description, "&lt;") || !strings.Contains(esc.Description, "Blue Recruitment Personnel is recruiting") {
		t.Errorf("entity-escaped markup not restored: %.300s", esc.Description)
	}
}

func TestTorreSeenPostingCostsNoDetail(t *testing.T) {
	f := newTorreFake(t)
	jobs, err := NewTorre(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("jobs = %d, want 6", len(jobs))
	}
	for _, u := range f.got {
		if strings.Contains(u, "/post/") {
			t.Errorf("seen posting fetched: %s", u)
		}
	}
}

func TestTorreReadingNoPostingPageFailsTheCrawl(t *testing.T) {
	f := newTorreFake(t)
	for _, u := range []string{torreRemoteURL, torreOnsiteURL, torreAnywhereURL, torreUnicodeURL, torreEscapedURL} {
		delete(f.files, u)
		f.errs = map[string]error{}
	}
	f.errs[torreClosedURL] = &StatusError{Method: "GET", Code: 503}
	if _, err := NewTorre(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when no new posting page could be read")
	}
}
