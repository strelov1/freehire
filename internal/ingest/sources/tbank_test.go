package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"sync"
	"testing"
)

// tbankFake is the test transport for T-Bank: sitemaps are GETs routed by URL, and each
// vacancy's detail is a POST routed by the urlSlug in its body. A urlSlug in failSlugs errors
// so detail isolation can be exercised; a sitemap URL absent from xml errors.
type tbankFake struct {
	xml       map[string]string // sitemap URL -> canned XML
	detail    map[string]string // urlSlug -> canned getVacancyDescription payload
	failSlugs map[string]bool

	mu        sync.Mutex // details are fetched concurrently
	detailReq []tbankDetailRequest
}

func (f *tbankFake) GetXML(_ context.Context, url string, v any) error {
	raw, ok := f.xml[url]
	if !ok {
		return errors.New("tbankFake: no sitemap at " + url)
	}
	return xml.Unmarshal([]byte(raw), v)
}

func (f *tbankFake) PostJSON(_ context.Context, url string, body, v any) error {
	if !strings.Contains(url, "getVacancyDescription") {
		return errors.New("tbankFake: unexpected POST " + url)
	}
	req, ok := body.(tbankDetailRequest)
	if !ok {
		return errors.New("tbankFake: detail body is not a tbankDetailRequest")
	}
	f.mu.Lock()
	f.detailReq = append(f.detailReq, req)
	f.mu.Unlock()
	if f.failSlugs[req.URLSlug] {
		return errors.New("tbankFake: detail boom for " + req.URLSlug)
	}
	raw, ok := f.detail[req.URLSlug]
	if !ok {
		return errors.New("tbankFake: no canned detail for " + req.URLSlug)
	}
	return json.Unmarshal([]byte(raw), v)
}

const tbankTestShardBase = "https://www.tbank.ru/storage/career/sitemaps/"

func tbankIndex(shards ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, s := range shards {
		b.WriteString("<sitemap><loc>" + tbankTestShardBase + s + "</loc></sitemap>")
	}
	b.WriteString("</sitemapindex>")
	return b.String()
}

func tbankShard(urls ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, u := range urls {
		b.WriteString("<url><loc>" + u + "</loc></url>")
	}
	b.WriteString("</urlset>")
	return b.String()
}

func tbankVacancyURL(section, city, seo, id string) string {
	return "https://www.tbank.ru/career/" + section + "/vacancy/" + city + "/" + seo + "/" + id + "/"
}

func tbankDetail(title, status string, tags ...string) string {
	ts := make([]string, len(tags))
	for i, t := range tags {
		ts[i] = `{"text":"` + t + `"}`
	}
	return `{"resultCode":"OK","payload":{"title":"` + title + `","status":"` + status + `","tags":[` +
		strings.Join(ts, ",") + `],"description":[` +
		`{"title":"Описание","content":"<ul><li>Build APIs.</li></ul><script>alert(1)</script>"},` +
		`{"title":"Что делать","content":[{"description":"Писать сервисы","title":"Сервисы"},{"description":"Ревьюить код","title":null}]}` +
		`]}}`
}

func TestTBankProvider(t *testing.T) {
	if got := NewTBank(nil).Provider(); got != "tbank" {
		t.Errorf("Provider() = %q, want tbank", got)
	}
}

func TestTBankIsBoardless(t *testing.T) {
	if _, ok := NewTBank(nil).(boardless); !ok {
		t.Error("tbank should be boardless")
	}
}

// The listing comes from the careers sitemaps, one shard per section; a vacancy listed once per
// city it is open in is one posting, its category taken from the section and its cities joined.
func TestTBankFetchReadsSitemapsAndMapsDetail(t *testing.T) {
	be := tbankVacancyURL("it", "moscow", "backend-engineer", "11111111-1111-1111-1111-111111111111")
	fake := &tbankFake{
		xml: map[string]string{
			tbankSitemapIndexURL: tbankIndex("all.xml", "catalog.xml", "it_0.xml", "back_office_0.xml", "publisher_0.xml"),
			tbankTestShardBase + "it_0.xml": tbankShard(be,
				tbankVacancyURL("it", "kazan", "backend-engineer", "11111111-1111-1111-1111-111111111111")),
			tbankTestShardBase + "back_office_0.xml": tbankShard(
				tbankVacancyURL("back-office", "moscow", "accountant", "22222222-2222-2222-2222-222222222222")),
			tbankTestShardBase + "publisher_0.xml": tbankShard(
				tbankVacancyURL("service", "ufa", "predstavitel", "33333333-3333-3333-3333-333333333333")),
		},
		detail: map[string]string{
			"11111111-1111-1111-1111-111111111111": tbankDetail("Backend Engineer", "published", "Удаленно", "Senior"),
			"22222222-2222-2222-2222-222222222222": tbankDetail("Accountant", "published", "Офис"),
			"33333333-3333-3333-3333-333333333333": tbankDetail("Представитель", "published"),
		},
	}
	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank", Provider: "tbank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the two-city vacancy counted once)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j := byID["11111111-1111-1111-1111-111111111111"]
	if j.Title != "Backend Engineer" || j.Company != "T-Bank" || j.URL != be {
		t.Errorf("job = %+v, want the detail's title, the configured company and the first listed URL", j)
	}
	if j.Location != "Moscow, Kazan" {
		t.Errorf("Location = %q, want both cities", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true (the Удаленно tag)")
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "Build APIs.") ||
		!strings.Contains(j.Description, "Писать сервисы") {
		t.Errorf("Description = %q, want both block shapes assembled and the script stripped", j.Description)
	}
	cats := map[string]string{}
	for _, r := range fake.detailReq {
		cats[r.URLSlug] = r.Options.Category
	}
	want := map[string]string{
		"11111111-1111-1111-1111-111111111111": "tcareer_it",
		"22222222-2222-2222-2222-222222222222": "tcareer_back_office",
		"33333333-3333-3333-3333-333333333333": "tcareer_work_with_clients",
	}
	for id, c := range want {
		if cats[id] != c {
			t.Errorf("detail for %s asked category %q, want %q", id, cats[id], c)
		}
	}
}

// A representative vacancy is listed in hundreds of cities; the location names the first few
// and how many more, rather than hundreds of names.
func TestTBankManyCitiesAreSummarised(t *testing.T) {
	id := "44444444-4444-4444-4444-444444444444"
	var urls []string
	for _, c := range []string{"moscow", "kazan", "ufa", "perm", "omsk"} {
		urls = append(urls, tbankVacancyURL("service", c, "predstavitel", id))
	}
	fake := &tbankFake{
		xml: map[string]string{
			tbankSitemapIndexURL:                   tbankIndex("publisher_0.xml"),
			tbankTestShardBase + "publisher_0.xml": tbankShard(urls...),
		},
		detail: map[string]string{id: tbankDetail("Представитель", "published")},
	}
	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch = %v, %v", jobs, err)
	}
	if got := jobs[0].Location; got != "Moscow, Kazan, Ufa and 2 more" {
		t.Errorf("Location = %q", got)
	}
}

func TestTBankFailedOrUnpublishedDetailDropsOnlyThatPosting(t *testing.T) {
	ok, bad, closed := "55555555-5555-5555-5555-555555555555", "66666666-6666-6666-6666-666666666666", "77777777-7777-7777-7777-777777777777"
	fake := &tbankFake{
		xml: map[string]string{
			tbankSitemapIndexURL: tbankIndex("it_0.xml"),
			tbankTestShardBase + "it_0.xml": tbankShard(
				tbankVacancyURL("it", "moscow", "a", ok), tbankVacancyURL("it", "moscow", "b", bad),
				tbankVacancyURL("it", "moscow", "c", closed)),
		},
		detail:    map[string]string{ok: tbankDetail("Kept", "published"), closed: tbankDetail("Closed", "not-found")},
		failSlugs: map[string]bool{bad: true},
	}
	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != ok {
		t.Errorf("jobs = %+v, want only the published, readable one", jobs)
	}
}

// T-Bank is swept: a shard that cannot be read must fail the crawl rather than drop its slice.
func TestTBankUnreadableShardFailsTheCrawl(t *testing.T) {
	fake := &tbankFake{xml: map[string]string{
		tbankSitemapIndexURL:            tbankIndex("it_0.xml", "back_office_0.xml"),
		tbankTestShardBase + "it_0.xml": tbankShard(tbankVacancyURL("it", "moscow", "a", "88888888-8888-8888-8888-888888888888")),
	}}
	if _, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when a vacancy shard cannot be read")
	}
}

// An empty listing is a failure, not a quiet success: the old list API went on answering
// "OK" with zero vacancies after T-Bank moved to a filter it no longer understood, and the
// provider looked healthy while collecting nothing.
func TestTBankEmptyListingIsAnError(t *testing.T) {
	fake := &tbankFake{xml: map[string]string{
		tbankSitemapIndexURL:            tbankIndex("it_0.xml"),
		tbankTestShardBase + "it_0.xml": tbankShard(),
	}}
	if _, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error for a listing with no vacancies")
	}
}

// The sitemaps list a nationwide vacancy in ~290 cities, alphabetically, including a few
// id-shaped slugs that name no city. The location leads with the capitals and drops the ids, so
// "Moscow, Saint Petersburg, Abakan and 3 more" rather than "Abakan, 1fb30227 C758…".
func TestTBankLocationLeadsWithCapitalsAndDropsIDSlugs(t *testing.T) {
	got := tbankLocation([]string{"abakan", "1fb30227-c758-4b1a-814b-eab9c8e7a582", "almetyevsk",
		"minsk", "moscow", "omsk", "saint-petersburg"})
	if want := "Moscow, Saint Petersburg, Abakan and 3 more"; got != want {
		t.Errorf("tbankLocation = %q, want %q", got, want)
	}
}
