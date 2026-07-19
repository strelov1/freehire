package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// fakeHabrCareer is a route-aware test client for the two Habr endpoints: the paged listing
// JSON (/api/frontend/vacancies?page=N, via GetJSONWithHeaders) and a per-vacancy detail page
// (/vacancies/<id>, via GetHTML). It records the pages requested, the headers sent on the
// listing, and the detail ids fetched, so tests can assert pagination, the API headers, and the
// per-vacancy detail fan-out. Detail fetches run concurrently, so its recorders are mutexed.
type fakeHabrCareer struct {
	pages     map[int]string // listing body keyed by requested page
	details   map[int]string // detail HTML keyed by vacancy id
	failList  bool           // every listing request fails
	failPage  map[int]bool   // a specific page's listing request fails
	detailErr map[int]bool   // a specific vacancy's detail request fails

	mu          sync.Mutex
	gotPages    []int
	gotDetails  []int
	listHeaders map[string]string
}

var (
	habrPageRE   = regexp.MustCompile(`[?&]page=(\d+)`)
	habrDetailRE = regexp.MustCompile(`/vacancies/(\d+)`)
)

func (f *fakeHabrCareer) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	page := 1
	if m := habrPageRE.FindStringSubmatch(url); m != nil {
		page, _ = strconv.Atoi(m[1])
	}
	f.mu.Lock()
	f.gotPages = append(f.gotPages, page)
	f.listHeaders = headers
	f.mu.Unlock()
	if f.failList || f.failPage[page] {
		return errors.New("fakeHabrCareer: list boom")
	}
	raw, ok := f.pages[page]
	if !ok {
		p := strconv.Itoa(page)
		raw = `{"list":[],"meta":{"currentPage":` + p + `,"totalPages":` + p + `}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *fakeHabrCareer) GetHTML(_ context.Context, url string) (*html.Node, error) {
	id := 0
	if m := habrDetailRE.FindStringSubmatch(url); m != nil {
		id, _ = strconv.Atoi(m[1])
	}
	f.mu.Lock()
	f.gotDetails = append(f.gotDetails, id)
	f.mu.Unlock()
	if f.detailErr[id] {
		return nil, errors.New("fakeHabrCareer: detail boom")
	}
	raw, ok := f.details[id]
	if !ok {
		raw = `<html><body><h1>no posting</h1></body></html>` // a page without JobPosting ld+json
	}
	return html.Parse(strings.NewReader(raw))
}

func habrFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func newHabrFake(t *testing.T) *fakeHabrCareer {
	return &fakeHabrCareer{
		pages: map[int]string{
			1: habrFixture(t, "habr_vacancies_p1.json"),
			2: habrFixture(t, "habr_vacancies_p2.json"),
		},
		details: map[int]string{
			1000166598: habrFixture(t, "habr_vacancy_detail.html"),
		},
		failPage:  map[int]bool{},
		detailErr: map[int]bool{},
	}
}

func jobByID(jobs []Job, id string) (Job, bool) {
	for _, j := range jobs {
		if j.ExternalID == id {
			return j, true
		}
	}
	return Job{}, false
}

func TestHabrCareerBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/habrcareer.yml")
	if err != nil {
		t.Fatalf("LoadConfig(sources/habrcareer.yml): %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/habrcareer.yml fails registry validation: %v", err)
	}
}

func TestHabrCareerProvider(t *testing.T) {
	if got := NewHabrCareer(nil).Provider(); got != "habr_career" {
		t.Errorf("Provider() = %q, want habr_career", got)
	}
}

func TestHabrCareerIsBoardlessAggregator(t *testing.T) {
	s := NewHabrCareer(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("habr_career must be boardless (no per-tenant board id)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("habr_career must be an aggregator (stays in the source facet)")
	}
}

func TestHabrCareerFetchPaginatesAndMaps(t *testing.T) {
	fake := newHabrFake(t)
	fake.detailErr[1000167078] = true // detail fails → vacancy still yielded, empty description

	jobs, err := NewHabrCareer(fake).Fetch(context.Background(), CompanyEntry{Provider: "habr_career"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	if !slices.Equal(fake.gotPages, []int{1, 2}) {
		t.Errorf("requested pages = %v, want [1 2]", fake.gotPages)
	}

	// Vacancy with a full detail page: identity, fields, sanitized description, date.
	j, ok := jobByID(jobs, "1000166598")
	if !ok {
		t.Fatal("vacancy 1000166598 missing")
	}
	if j.URL != "https://career.habr.com/vacancies/1000166598" {
		t.Errorf("URL = %q, want canonical vacancy URL (dedups with linksource)", j.URL)
	}
	if j.Title != "Специалист по работе с документами" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "ИТ Контакт" {
		t.Errorf("Company = %q, want the listing company.title", j.Company)
	}
	if j.Location != "Санкт-Петербург" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("Remote=%v WorkMode=%q, want false/empty for remoteWork:false", j.Remote, j.WorkMode)
	}
	// publishedDate.date 2026-05-29T10:00:00+03:00 → 07:00:00Z; NOT the detail basic-date.
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 5, 29, 7, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-29T07:00:00Z from listing publishedDate.date", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<li>Работа с документами</li>") {
		t.Errorf("Description not sanitized from detail JobPosting: %q", j.Description)
	}

	// Remote vacancy whose detail fetch failed: still yielded, empty description, remote signal.
	r, ok := jobByID(jobs, "1000167078")
	if !ok {
		t.Fatal("vacancy 1000167078 missing — a failed detail must not drop it")
	}
	if !r.Remote || r.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote for remoteWork:true", r.Remote, r.WorkMode)
	}
	if r.Location != "Минск, Удалённо" {
		t.Errorf("Location = %q, want distinct locations joined", r.Location)
	}
	if r.Description != "" {
		t.Errorf("Description = %q, want empty when detail fetch failed", r.Description)
	}

	// Vacancy whose detail page has no JobPosting: still yielded, empty description, empty company.
	q, ok := jobByID(jobs, "1000160000")
	if !ok {
		t.Fatal("vacancy 1000160000 missing — a page without JobPosting must not drop it")
	}
	if q.Description != "" {
		t.Errorf("Description = %q, want empty when detail has no JobPosting", q.Description)
	}
	if q.Company != "" || q.Location != "" {
		t.Errorf("Company=%q Location=%q, want empty for the no-company/no-location edge", q.Company, q.Location)
	}
}

func TestHabrCareerSendsAPIHeaders(t *testing.T) {
	fake := newHabrFake(t)
	if _, err := NewHabrCareer(fake).Fetch(context.Background(), CompanyEntry{Provider: "habr_career"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := fake.listHeaders["Accept"]; got != "application/json" {
		t.Errorf("Accept header = %q, want application/json", got)
	}
	if got := fake.listHeaders["Referer"]; got != "https://career.habr.com/vacancies" {
		t.Errorf("Referer header = %q, want https://career.habr.com/vacancies", got)
	}
}

func TestHabrCareerFirstPageErrorIsBoardError(t *testing.T) {
	fake := newHabrFake(t)
	fake.failPage = map[int]bool{1: true}
	if _, err := NewHabrCareer(fake).Fetch(context.Background(), CompanyEntry{Provider: "habr_career"}); err == nil {
		t.Fatal("want an error when the first listing page fails")
	}
}

// A later-page failure is a truncated crawl, not a natural end. habr_career is a fullCatalog
// source whose unseen jobs the sweep closes by source, so a partial listing returned as success
// would mass-close every posting past the failed page. The crawl must error instead, so Failed>0
// steers the sweep back to the safe company-scoped close.
func TestHabrCareerLaterPageErrorIsBoardError(t *testing.T) {
	fake := newHabrFake(t)
	fake.failPage = map[int]bool{2: true}
	if _, err := NewHabrCareer(fake).Fetch(context.Background(), CompanyEntry{Provider: "habr_career"}); err == nil {
		t.Fatal("want an error when a later listing page fails (a truncated fullCatalog crawl must not look complete)")
	}
}

// FullCatalogProviders drives the sweep's source-scoped close; the whole-catalogue aggregators
// (habr_career, geekjob) must be in it and a per-company board like greenhouse must not, or a
// vanished company's jobs never retire.
func TestFullCatalogProviders(t *testing.T) {
	got := FullCatalogProviders(All(nil))
	for _, want := range []string{"habr_career", "geekjob"} {
		if !slices.Contains(got, want) {
			t.Errorf("FullCatalogProviders() = %v, want it to contain %q", got, want)
		}
	}
	if slices.Contains(got, "greenhouse") {
		t.Error("FullCatalogProviders() must not contain a per-company board like greenhouse")
	}
}

func TestHabrCareerIsProxied(t *testing.T) {
	// Qrator challenges habr's per-vacancy detail HTML from the prod datacenter IP (the listing
	// JSON passes, but the description parse fails), so the crawl must egress through the proxy.
	if _, ok := proxiedProviders["habr_career"]; !ok {
		t.Error("habr_career must be in proxiedProviders (Qrator blocks detail fetches from the prod datacenter IP)")
	}
}

func TestParseHabrPostingSingleObjectJobLocation(t *testing.T) {
	// schema.org allows jobLocation as a single Place object, not only an array — Habr emits
	// this form for many (often remote) vacancies. Modeling it as array-only made json.Unmarshal
	// fail on the whole JobPosting, so the description was dropped and the vacancy stored empty.
	// The decoder must accept both shapes.
	const page = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Fullstack Developer (MERN / NestJS)",
 "description":"<p>Node.js and React</p>",
 "hiringOrganization":{"@type":"Organization","name":"Creative Code"},
 "jobLocation":{"@type":"Place","address":"Russia"},
 "jobLocationType":"TELECOMMUTE","employmentType":"FULL_TIME"}
</script></head><body></body></html>`
	node, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p, ok := ParseHabrPosting(node)
	if !ok {
		t.Fatal("ParseHabrPosting ok=false on a single-object jobLocation — the whole posting was dropped")
	}
	if !strings.Contains(p.Description, "Node.js") {
		t.Errorf("Description = %q, want the posting body", p.Description)
	}
	if p.Company != "Creative Code" {
		t.Errorf("Company = %q, want Creative Code", p.Company)
	}
	if p.Location != "Russia" {
		t.Errorf("Location = %q, want Russia from the single Place", p.Location)
	}
}

func TestParseHabrPostingArrayJobLocation(t *testing.T) {
	// The array shape must keep working after the single-object fix.
	const page = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Backend Engineer","description":"<p>Go</p>",
 "hiringOrganization":{"@type":"Organization","name":"Acme"},
 "jobLocation":[{"@type":"Place","address":"Berlin"}]}
</script></head><body></body></html>`
	node, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p, ok := ParseHabrPosting(node)
	if !ok {
		t.Fatal("ParseHabrPosting ok=false on an array jobLocation")
	}
	if p.Location != "Berlin" {
		t.Errorf("Location = %q, want Berlin", p.Location)
	}
}
