package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func xingFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/xing/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// xingFake serves the sitemaps gzipped, as the site does, and the posting pages from fixtures;
// a URL with no fixture answers 410 Gone.
type xingFake struct {
	t     *testing.T
	gz    map[string]string
	pages map[string]string
	mu    sync.Mutex
	gets  []string
}

func (f *xingFake) record(u string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, u)
}

func (f *xingFake) GetStream(_ context.Context, u, _ string, fn func(io.Reader) error) error {
	f.record(u)
	name, ok := f.gz[u]
	if !ok {
		return &StatusError{Code: 404}
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(xingFixture(f.t, name))); err != nil {
		f.t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		f.t.Fatal(err)
	}
	return fn(&buf)
}

func (f *xingFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.record(u)
	name, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Code: 410}
	}
	return html.Parse(strings.NewReader(xingFixture(f.t, name)))
}

const (
	xingTemplateURL = "https://www.xing.com/jobs/wien-senior-full-stack-developer-net-146728533"
	xingHourlyURL   = "https://www.xing.com/jobs/werthenstein-servierkraft-153322443"
	xingGoneURL     = "https://www.xing.com/jobs/frankfurt-main-regionalleitung-kindertageseinrichtungen-157462896"
	xingHTMLURL     = "https://www.xing.com/jobs/berlin-payments-architect-all-genders-158121457"
	xingRemoteURL   = "https://www.xing.com/jobs/karlsruhe-projektleiter-elektrotechnik-gebaeudetechnik-kalkulation-158694956"
)

func newXingFake(t *testing.T) *xingFake {
	return &xingFake{
		t: t,
		gz: map[string]string{
			xingSitemapIndex: "sitemap.xml",
			"https://www.xing.com/jobs/1-sitemap.xml.gz": "1-sitemap.xml",
			"https://www.xing.com/jobs/2-sitemap.xml.gz": "2-sitemap.xml",
		},
		pages: map[string]string{
			xingTemplateURL: "detail_template.html",
			xingHourlyURL:   "detail_hourly_ch.html",
			xingHTMLURL:     "detail_html.html",
			xingRemoteURL:   "detail_remote_multi.html",
		},
	}
}

func newTestXing(c xingHTTP) xing {
	return xing{http: c, limiter: noWait{}, freshPerRun: 100, backlogPerRun: 100, now: time.Now}
}

func TestXingPostingID(t *testing.T) {
	for loc, want := range map[string]int64{
		xingTemplateURL: 146728533,
		"https://www.xing.com/jobs/search?keywords=go":        0,
		"https://www.xing.com/jobs/berlin-softwareentwickler": 0,
		"https://www.xing.com/pages/acme-123":                 0,
	} {
		if got := xingPostingID(loc); got != want {
			t.Errorf("xingPostingID(%q) = %d, want %d", loc, got, want)
		}
	}
}

func TestXingFetchNewRefreshesSeenAndHydratesTheRest(t *testing.T) {
	f := newXingFake(t)
	seen := map[string]bool{"158121457": true}
	jobs, err := newTestXing(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// Five distinct postings: one seen, three hydrated, one gone (410, not yielded). The
	// posting both sitemaps list is read once, and the role-landing sitemap is never fetched.
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4: %+v", len(jobs), byID)
	}
	if r := byID["158121457"]; !r.SeenRefresh || r.URL != xingHTMLURL || r.Title != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	if _, ok := byID["157462896"]; ok {
		t.Error("a gone posting must not be yielded")
	}
	reads := 0
	for _, u := range f.gets {
		switch {
		case strings.Contains(u, "jobs_roles"):
			t.Errorf("fetched non-posting sitemap %s", u)
		case u == xingHTMLURL:
			t.Error("fetched the page of a seen posting")
		case u == xingHourlyURL:
			reads++
		}
	}
	if reads != 1 {
		t.Errorf("posting listed twice was read %d times, want 1", reads)
	}
}

func TestXingListFailsWhenAnySitemapFails(t *testing.T) {
	f := newXingFake(t)
	delete(f.gz, "https://www.xing.com/jobs/2-sitemap.xml.gz")
	if _, err := newTestXing(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when a posting sitemap cannot be read")
	}
}

func xingParse(t *testing.T, name, loc string) (Job, bool) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(xingFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return xingDetail(root, xingPosting{id: xingPostingID(loc), url: loc})
}

func TestXingDetailTemplatePosting(t *testing.T) {
	j, ok := xingParse(t, "detail_template.html", xingTemplateURL)
	if !ok {
		t.Fatal("not parsed")
	}
	if j.Title != "Senior Full Stack Developer .NET (w/m/d)" || j.Company != "Delegate Technology GmbH" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Wien, Bundesland Wien" || len(j.Countries) != 1 || j.Countries[0] != "at" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("work mode = %q remote = %v, want hybrid", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" || !j.IsTechHint {
		t.Errorf("employment = %q tech hint = %v", j.EmploymentType, j.IsTechHint)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 53802 || *j.SalaryMax != 53803 || j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2025, 11, 20, 5, 53, 52, 0, time.UTC)) {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if !strings.HasPrefix(j.Description, "<p><strong>Was macht das Delegate Team?</strong></p>") {
		t.Errorf("body = %.120s", j.Description)
	}
}

// A template split into sections keeps each under the employer's heading, and leaves out the
// standing company blurb.
func TestXingDetailTemplateSections(t *testing.T) {
	const loc = "https://www.xing.com/jobs/berlin-ober-bauleiter-elektro-fernmeldetechnik-industrieprojekte-152149420"
	j, ok := xingParse(t, "detail_template_sections.html", loc)
	if !ok {
		t.Fatal("not parsed")
	}
	if j.Company != "Drees & Sommer SE" || j.IsTechHint {
		t.Errorf("company = %q tech = %v", j.Company, j.IsTechHint)
	}
	for _, h := range []string{"<h3>Stellenbeschreibung</h3>", "<h3>Qualifikationen</h3>", "<h3>Zusätzliche Informationen</h3>"} {
		if !strings.Contains(j.Description, h) {
			t.Errorf("body lacks %s", h)
		}
	}
	if strings.Contains(j.Description, "Unternehmensbeschreibung") || strings.Contains(j.Description, "lebenswerte Zukunft") {
		t.Error("the company description section must be left out")
	}
}

func TestXingDetailHTMLPostingDropsTheEstimate(t *testing.T) {
	j, ok := xingParse(t, "detail_html.html", xingHTMLURL)
	if !ok {
		t.Fatal("not parsed")
	}
	if j.Company != "Accenture Dienstleistungen GmbH" || j.Location != "Berlin, Land Berlin" {
		t.Errorf("company/location = %q / %q", j.Company, j.Location)
	}
	if j.Description == "" || j.WorkMode != "hybrid" || !j.IsTechHint {
		t.Errorf("body empty = %v, work mode = %q, tech = %v", j.Description == "", j.WorkMode, j.IsTechHint)
	}
	// SalaryEstimate is XING's guess, not the employer's figure.
	if j.SalaryMin != nil || j.SalaryCurrency != "" {
		t.Errorf("estimate stored as salary: %v %s", j.SalaryMin, j.SalaryCurrency)
	}
}

func TestXingDetailHourlySalaryAndDefaultRemoteOption(t *testing.T) {
	j, ok := xingParse(t, "detail_hourly_ch.html", xingHourlyURL)
	if !ok {
		t.Fatal("not parsed")
	}
	if len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("countries = %v", j.Countries)
	}
	// "26 CHF – 28 CHF" is an hourly wage XING files as YEAR; it is not stored.
	if j.SalaryMin != nil {
		t.Errorf("hourly figure stored as yearly salary %d", *j.SalaryMin)
	}
	// NON_REMOTE is the form's default, not a statement that the work is on site.
	if j.WorkMode != "" || j.IsTechHint {
		t.Errorf("work mode = %q tech = %v", j.WorkMode, j.IsTechHint)
	}
}

func TestXingDetailRemoteMultiLocation(t *testing.T) {
	j, ok := xingParse(t, "detail_remote_multi.html", xingRemoteURL)
	if !ok {
		t.Fatal("not parsed")
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("work mode = %q remote = %v", j.WorkMode, j.Remote)
	}
	if !strings.HasPrefix(j.Location, "Karlsruhe, Baden-Württemberg; Berlin, Land Berlin; ") {
		t.Errorf("location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("countries = %v, want one de for ten German places", j.Countries)
	}
}

func TestXingDetailGonePosting(t *testing.T) {
	if _, ok := xingParse(t, "detail_gone.html", xingGoneURL); ok {
		t.Error("an UnauthorizedJob page must not yield a posting")
	}
}

func TestXingLanes(t *testing.T) {
	var unseen []xingPosting
	for id := int64(100); id > 0; id-- {
		unseen = append(unseen, xingPosting{id: id})
	}
	ids := func(ps []xingPosting) []int64 {
		out := make([]int64, len(ps))
		for i, p := range ps {
			out[i] = p.id
		}
		return out
	}
	hour := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	a := ids(xingLanes(unseen, 3, 2, hour))
	b := ids(xingLanes(unseen, 3, 2, hour.Add(time.Hour)))
	if len(a) != 5 || a[0] != 100 || a[1] != 99 || a[2] != 98 {
		t.Fatalf("fresh lane = %v, want the three newest first", a)
	}
	if a[3] == b[3] {
		t.Errorf("backlog window did not move between hours: %v vs %v", a, b)
	}
	if got := xingLanes(unseen[:4], 3, 2, hour); len(got) != 4 {
		t.Errorf("a short backlog must be taken whole, got %d", len(got))
	}
}

// A sitemap whose gzip stream is cut short fails the crawl even when the XML it held so far
// is complete: a short list would let the sweep close the postings it lost.
func TestXingTruncatedSitemapFails(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(xingFixture(t, "1-sitemap.xml"))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	cut := buf.Bytes()[:buf.Len()-4] // drop the trailing size field
	s := newTestXing(truncatedStream(cut))
	if _, err := s.sitemap(context.Background(), "https://www.xing.com/jobs/1-sitemap.xml.gz"); err == nil {
		t.Fatal("want an error for a truncated gzip sitemap")
	}
}

type truncatedStream []byte

func (b truncatedStream) GetStream(_ context.Context, _, _ string, fn func(io.Reader) error) error {
	return fn(bytes.NewReader(b))
}

func (truncatedStream) GetHTML(context.Context, string) (*html.Node, error) { return nil, nil }
