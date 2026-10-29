package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func jobschFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/jobsch/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jobschFake serves the sitemaps gzipped, as the sites do, and the search and posting pages as
// markup; a URL it does not know answers 404.
type jobschFake struct {
	t        *testing.T
	sitemaps map[string]string
	pages    map[string]string
	fail     map[string]bool
	mu       sync.Mutex
	gets     []string
}

func (f *jobschFake) record(u string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, u)
}

func (f *jobschFake) GetStream(_ context.Context, u, _ string, fn func(io.Reader) error) error {
	f.record(u)
	body, ok := f.sitemaps[u]
	if !ok {
		return &StatusError{Code: 404}
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(body)); err != nil {
		f.t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		f.t.Fatal(err)
	}
	return fn(&buf)
}

func (f *jobschFake) GetText(_ context.Context, u string) (string, error) {
	f.record(u)
	if f.fail[u] {
		return "", &StatusError{Code: 503}
	}
	body, ok := f.pages[u]
	if !ok {
		return "", &StatusError{Code: 404}
	}
	return body, nil
}

func (f *jobschFake) requested(u string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.gets {
		if g == u {
			return true
		}
	}
	return false
}

func jobschSitemap(ids ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for i, id := range ids {
		fmt.Fprintf(&b, `<url><loc>https://www.jobs.ch/de/stellenangebote/detail/%s/</loc><lastmod>2026-10-%02dT10:00:00+01:00</lastmod></url>`, id, 10+i)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// jobschSearchPage renders a search page whose state names the given cards, as {id, title,
// company} triples, in the page's own shape.
func jobschSearchPage(cards ...[3]string) string {
	var rows []string
	for _, c := range cards {
		rows = append(rows, fmt.Sprintf(`{"id":%q,"title":%q,"company":{"id":"1","name":%q,"logoImage":undefined},"place":"Zürich","locations":[{"city":"Zürich","countryCode":"CH"}],"initialPublicationDate":"2026-10-28T06:00:00+01:00"}`, c[0], c[1], c[2]))
	}
	return "<html><body><script>\n      __INIT__ = {\"vacancy\":{\"results\":{\"main\":{\"results\":[" + strings.Join(rows, ",") + "]}}}};\n      __LOAD_LAZY__ = false;\n</script></body></html>"
}

const (
	jobschDev    = "a4797cae-1c77-4e7d-9166-0369d8b600bf" // jobs.ch: IT, yearly salary, home office
	jobschApprt  = "b7ca4d07-9996-49da-aa3f-4beabe3db24e" // jobs.ch: apprenticeship, "yearly" 1,020
	jobschSales  = "87fb6bc7-d43f-409a-b38e-f6a80da7d082" // jobs.ch: five places
	jobschAnon   = "559dd3eb-ab58-445e-9b89-20b0aa0b4034" // jobs.ch: no employer
	jobschHourly = "06b66ea5-3df9-47ce-8b4d-8efa3fbc6b54" // jobs.ch: hourly salary, no top
	jobschSecu   = "3e5e3767-4e5c-429d-8925-21e0ec345eae" // jobup.ch only: IT
)

func jobschPostingURL(site int, id string) string { return fmt.Sprintf(jobschSites[site].posting, id) }

func jobschSearchURL(site, page int) string { return fmt.Sprintf(jobschSites[site].search, page) }

func newJobschFake(t *testing.T) *jobschFake {
	f := &jobschFake{t: t, pages: map[string]string{}, fail: map[string]bool{}}
	f.sitemaps = map[string]string{
		jobschSites[0].sitemap: jobschSitemap(jobschDev, jobschApprt, jobschSales, "11111111-1111-4111-8111-111111111111"),
		jobschSites[1].sitemap: strings.ReplaceAll(jobschSitemap(jobschSales, jobschSecu), "www.jobs.ch/de/stellenangebote", "www.jobup.ch/fr/emplois"),
	}
	for _, p := range []struct {
		site int
		id   string
	}{{0, jobschDev}, {0, jobschApprt}, {0, jobschSales}, {0, jobschAnon}, {0, jobschHourly}, {1, jobschSecu}} {
		f.pages[jobschPostingURL(p.site, p.id)] = jobschFixture(t, "posting_"+p.id[:8]+".html")
	}
	f.pages[jobschSearchURL(0, 1)] = jobschSearchPage(
		[3]string{jobschHourly, "Produktionsmitarbeiter", "ASSpro AG"},
		[3]string{"22222222-2222-4222-8222-222222222222", "Software Engineer", "Covered AG"},
		[3]string{jobschAnon, "Supply Chain Specialist", ""},
	)
	// A page past the last one redirects to page 1: it names nothing new.
	f.pages[jobschSearchURL(0, 2)] = f.pages[jobschSearchURL(0, 1)]
	f.pages[jobschSearchURL(1, 1)] = jobschSearchPage()
	return f
}

func newTestJobsch(f *jobschFake) jobsch {
	return jobsch{
		http:           f,
		limiter:        noWait{},
		freshPerRun:    jobschFreshPerRun,
		backlogPerRun:  jobschBacklogPerRun,
		searchPages:    jobschSearchPages,
		deadlineMargin: jobschDeadlineMargin,
		now:            func() time.Time { return time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC) },
	}
}

func jobschByID(jobs []Job) map[string]Job {
	out := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestJobschFetchRefreshesSeenGatesCardsAndHydratesTheRest(t *testing.T) {
	f := newJobschFake(t)
	seen := func(id string) bool { return id == jobschApprt }
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		return map[string]bool{"Covered AG": true}
	}}
	jobs, err := newTestJobsch(f).FetchNewGated(context.Background(), CompanyEntry{}, seen, gate)
	if err != nil {
		t.Fatal(err)
	}
	got := jobschByID(jobs)

	if j := got[jobschApprt]; !j.SeenRefresh || f.requested(jobschPostingURL(0, jobschApprt)) {
		t.Errorf("seen posting = %+v, page requested %v; want a refresh with no request", j, f.requested(jobschPostingURL(0, jobschApprt)))
	}
	covered := got["22222222-2222-4222-8222-222222222222"]
	if covered.Company != "Covered AG" || covered.Description != "" || f.requested(jobschPostingURL(0, "22222222-2222-4222-8222-222222222222")) {
		t.Errorf("covered posting = %+v; want it yielded body-less from its card, its page never bought", covered)
	}
	if _, ok := got[jobschAnon]; ok || f.requested(jobschPostingURL(0, jobschAnon)) {
		t.Error("a card naming no employer must be neither yielded nor bought")
	}
	// The posting both sites carry is read from jobs.ch; the jobup-only one from jobup.ch.
	if !f.requested(jobschPostingURL(0, jobschSales)) || f.requested(jobschPostingURL(1, jobschSales)) {
		t.Error("a posting on both sites must be read from jobs.ch only")
	}
	if j := got[jobschSecu]; j.URL != jobschPostingURL(1, jobschSecu) || j.Description == "" {
		t.Errorf("jobup-only posting = %+v; want it hydrated from jobup.ch", j)
	}
	// The posting a search card named but no sitemap yet lists is still hydrated.
	if j := got[jobschHourly]; j.Description == "" {
		t.Error("a search-named posting the sitemap does not list yet must be hydrated")
	}
	// A posting the sitemap lists that answers 404 is not yielded.
	if _, ok := got["11111111-1111-4111-8111-111111111111"]; ok {
		t.Error("a gone posting must not be yielded")
	}
	// The second search page named nothing new, so the walk stopped there.
	if f.requested(jobschSearchURL(0, 3)) {
		t.Error("the search walk must stop at a page naming nothing new")
	}
}

func TestJobschFreshLaneComesFirstAndBudgetDefersTheRest(t *testing.T) {
	f := newJobschFake(t)
	s := newTestJobsch(f)
	s.freshPerRun, s.backlogPerRun = 1, 0
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	var hydrated []string
	for _, j := range jobs {
		if j.Description != "" {
			hydrated = append(hydrated, j.ExternalID)
		}
	}
	if len(hydrated) != 1 || hydrated[0] != jobschHourly {
		t.Errorf("hydrated %v; want only the newest search-named posting %s", hydrated, jobschHourly)
	}
}

func TestJobschSitemapFailureFailsTheCrawl(t *testing.T) {
	f := newJobschFake(t)
	delete(f.sitemaps, jobschSites[1].sitemap)
	if _, err := newTestJobsch(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when a sitemap cannot be read")
	}
}

func TestJobschSearchFailureFailsTheCrawl(t *testing.T) {
	f := newJobschFake(t)
	f.fail[jobschSearchURL(1, 1)] = true
	if _, err := newTestJobsch(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when a search page cannot be read")
	}
}

func TestJobschNoPostingAnsweredFailsTheCrawl(t *testing.T) {
	f := newJobschFake(t)
	for u := range f.pages {
		if strings.Contains(u, "/detail/") {
			f.fail[u] = true
		}
	}
	f.fail[jobschPostingURL(0, "11111111-1111-4111-8111-111111111111")] = true
	f.fail[jobschPostingURL(0, "22222222-2222-4222-8222-222222222222")] = true
	if _, err := newTestJobsch(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when no posting page answered")
	}
}

func TestJobschStopsHydratingNearTheDeadline(t *testing.T) {
	f := newJobschFake(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestJobsch(f).FetchNew(ctx, CompanyEntry{}, func(id string) bool { return id == jobschApprt })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Description != "" {
			t.Fatalf("hydrated %s inside the deadline margin", j.ExternalID)
		}
	}
	if f.requested(jobschPostingURL(0, jobschDev)) {
		t.Error("no posting page may be bought inside the deadline margin")
	}
}

func jobschDetailOf(t *testing.T, site int, id string) (Job, bool) {
	t.Helper()
	s := newTestJobsch(newJobschFake(t))
	job, ok, err := s.detail(context.Background(), jobschPosting{id: id, site: site})
	if err != nil {
		t.Fatal(err)
	}
	return job, ok
}

func TestJobschDetailITPostingWithSalaryAndHomeOffice(t *testing.T) {
	j, ok := jobschDetailOf(t, 0, jobschDev)
	if !ok {
		t.Fatal("posting not mapped")
	}
	if j.Title != "Web-/Software-Entwickler:in Frontend und/oder Backend" || j.Company != "go4web AG" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://www.jobs.ch/de/stellenangebote/detail/"+jobschDev+"/" {
		t.Errorf("url = %q", j.URL)
	}
	if !j.IsTechHint {
		t.Error("category 106 must set IsTechHint")
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("work mode = %q; want hybrid from the working-from-home benefit", j.WorkMode)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 70000 || *j.SalaryMax != 90000 || j.SalaryCurrency != "CHF" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.Location != "Luzern" || strings.Join(j.Countries, ",") != "ch" || strings.Join(j.FallbackCountries, ",") != "ch" {
		t.Errorf("location = %q, countries %v, fallback %v", j.Location, j.Countries, j.FallbackCountries)
	}
	if j.EmploymentType != "" {
		t.Errorf("employment type = %q; a permanent 80-100%% position states no hours", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format(time.RFC3339) != "2026-09-24T15:26:22Z" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Frontend, Backend, Fullstack") {
		t.Errorf("description = %.200q", j.Description)
	}
}

func TestJobschDetailDropsAYearlySalaryTooSmallToBeAYear(t *testing.T) {
	j, ok := jobschDetailOf(t, 0, jobschApprt)
	if !ok {
		t.Fatal("posting not mapped")
	}
	if j.SalaryMin != nil || j.SalaryPeriod != "" {
		t.Errorf("salary = %v %s; want none for YEAR 1,020-1,550", j.SalaryMin, j.SalaryPeriod)
	}
	if j.EmploymentType != "" || j.IsTechHint {
		t.Errorf("apprenticeship: type %q, tech %v", j.EmploymentType, j.IsTechHint)
	}
}

func TestJobschDetailHourlySalaryWithoutATop(t *testing.T) {
	j, ok := jobschDetailOf(t, 0, jobschHourly)
	if !ok {
		t.Fatal("posting not mapped")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 32 || *j.SalaryMax != 32 || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %v-%v %s", j.SalaryMin, j.SalaryMax, j.SalaryPeriod)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q; want full_time for a permanent 100%% position", j.EmploymentType)
	}
}

func TestJobschDetailManyPlaces(t *testing.T) {
	j, ok := jobschDetailOf(t, 0, jobschSales)
	if !ok {
		t.Fatal("posting not mapped")
	}
	if !strings.HasPrefix(j.Location, "Zürich; Aarau; Luzern; Basel; Bern") {
		t.Errorf("location = %q", j.Location)
	}
	if strings.Join(j.Countries, ",") != "ch" {
		t.Errorf("countries = %v", j.Countries)
	}
}

func TestJobschDetailDropsAnAnonymousEmployer(t *testing.T) {
	if _, ok := jobschDetailOf(t, 0, jobschAnon); ok {
		t.Error("a posting naming no employer must be dropped")
	}
}

func TestJobschDetailJobupITPosting(t *testing.T) {
	j, ok := jobschDetailOf(t, 1, jobschSecu)
	if !ok {
		t.Fatal("posting not mapped")
	}
	if !j.IsTechHint {
		t.Error("jobup's category 702 must set IsTechHint")
	}
	if j.Company != "MGI Consultants SA" || j.Location != "Genève" || j.Countries != nil || strings.Join(j.FallbackCountries, ",") != "ch" {
		t.Errorf("job = %q at %q, countries %v, fallback %v", j.Company, j.Location, j.Countries, j.FallbackCountries)
	}
}

func TestJobschSearchFixtures(t *testing.T) {
	for i, name := range []string{"search_jobs.html", "search_jobup.html"} {
		f := &jobschFake{t: t, pages: map[string]string{jobschSearchURL(i, 1): jobschFixture(t, name)}}
		s := newTestJobsch(f)
		s.searchPages = 1
		cards, err := s.search(context.Background(), jobschSites[i])
		if err != nil {
			t.Fatal(err)
		}
		if len(cards) != 3 {
			t.Fatalf("%s: %d cards", name, len(cards))
		}
		for _, c := range cards {
			if c.ID == "" || c.Title == "" || c.company() == "" || c.InitialPublication == "" {
				t.Errorf("%s: card %+v", name, c)
			}
		}
	}
}

func TestJsUndefinedToNull(t *testing.T) {
	in := `{"a":undefined,"b":[undefined,undefined],"c":"x:undefined,","d":"q\"undefined","undefinedKey":1,"e":xundefined}`
	want := `{"a":null,"b":[null,null],"c":"x:undefined,","d":"q\"undefined","undefinedKey":1,"e":xundefined}`
	if got := jsUndefinedToNull(in); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestJobschListOrdersSearchNamedFirstThenSitemapNewest(t *testing.T) {
	f := newJobschFake(t)
	postings, err := newTestJobsch(f).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range postings {
		ids = append(ids, p.id[:8])
	}
	// Search-named first in page order, then the sitemap's by lastmod, newest first.
	want := []string{"06b66ea5", "22222222", "559dd3eb", "11111111", "87fb6bc7", "b7ca4d07", "3e5e3767", "a4797cae"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v; want %v", ids, want)
	}
	sites := map[string]int{}
	for _, p := range postings {
		sites[p.id[:8]] = p.site
	}
	if sites["87fb6bc7"] != 0 || sites["3e5e3767"] != 1 {
		t.Errorf("sites = %v", sites)
	}
}
