package sources

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// tbankHTTP is a body-aware test JSONPoster for T-Bank: both the list and the detail are
// POSTs to fixed URLs, distinguished by the request body. The list fake paginates on the
// requested offset (page 1 → page 2, then isFinished); the detail fake routes the canned
// description[] on the request's urlSlug. A urlSlug in failSlugs errors so detail-isolation
// can be exercised.
type tbankHTTP struct {
	pages       map[int]string    // offset -> canned getVacancies payload
	detail      map[string]string // urlSlug -> canned getVacancyDescription payload
	failSlugs   map[string]bool   // urlSlugs whose detail request errors
	listCalls   int               // number of getVacancies requests served (loop-termination guard)
	lastListReq tbankListRequest  // the most recent getVacancies request body (filter assertions)
}

func (f *tbankHTTP) PostJSON(_ context.Context, url string, body, v any) error {
	switch {
	case strings.Contains(url, "getVacancies"):
		req, ok := body.(tbankListRequest)
		if !ok {
			return errors.New("tbankHTTP: list body is not a tbankListRequest")
		}
		f.listCalls++
		f.lastListReq = req
		if f.listCalls > 8 { // a non-advancing loop would spin forever; bound the test
			return errors.New("tbankHTTP: too many list calls (pagination did not terminate)")
		}
		raw, ok := f.pages[req.Pagination.Publisher.Offset]
		if !ok {
			return errors.New("tbankHTTP: no canned page for offset")
		}
		return json.Unmarshal([]byte(raw), v)
	case strings.Contains(url, "getVacancyDescription"):
		req, ok := body.(tbankDetailRequest)
		if !ok {
			return errors.New("tbankHTTP: detail body is not a tbankDetailRequest")
		}
		if f.failSlugs[req.URLSlug] {
			return errors.New("tbankHTTP: detail boom for " + req.URLSlug)
		}
		raw, ok := f.detail[req.URLSlug]
		if !ok {
			return errors.New("tbankHTTP: no canned detail for " + req.URLSlug)
		}
		return json.Unmarshal([]byte(raw), v)
	default:
		return errors.New("tbankHTTP: unknown url " + url)
	}
}

// tbankListPage builds one getVacancies payload with the given next-pagination offset and
// isFinished flag plus inline vacancy fragments.
func tbankListPage(nextOffset int, isFinished bool, vacancies ...string) string {
	fin := "false"
	if isFinished {
		fin = "true"
	}
	return `{"resultCode":"OK","payload":{"nextPagination":{"publisher":{"offset":` + itoa(nextOffset) +
		`,"isFinished":` + fin + `,"totalCount":2}},"vacancies":[` + strings.Join(vacancies, ",") + `]}}`
}

func tbankVacancy(title, subtitle, category, urlSlug, seoSlug string, tags ...string) string {
	ts := make([]string, len(tags))
	for i, t := range tags {
		ts[i] = `"` + t + `"`
	}
	return `{"title":"` + title + `","subtitle":"` + subtitle + `","category":"` + category +
		`","urlSlug":"` + urlSlug + `","seoSlug":"` + seoSlug + `","tags":[` + strings.Join(ts, ",") + `]}`
}

func TestTBankProvider(t *testing.T) {
	if got := NewTBank(nil).Provider(); got != "tbank" {
		t.Errorf("Provider() = %q, want %q", got, "tbank")
	}
}

func TestTBankIsBoardless(t *testing.T) {
	if _, ok := NewTBank(nil).(boardless); !ok {
		t.Error("tbank should implement the boardless marker")
	}
}

// The publisher list source treats an empty category filter as work-with-clients only, so
// the request MUST name every top-level category to reach IT and back-office vacancies.
func TestTBankListRequestsAllCategories(t *testing.T) {
	fake := &tbankHTTP{
		pages:  map[int]string{0: tbankListPage(0, true, tbankVacancy("Eng", "Москва", "tcareer_it", "s1", "eng"))},
		detail: map[string]string{"s1": `{"resultCode":"OK","payload":{"description":[{"key":"k","content":"<p>x</p>"}]}}`},
	}

	if _, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	cats, ok := fake.lastListReq.Filters["category"].([]string)
	if !ok {
		t.Fatalf("list filters.category = %#v, want []string of top-level categories", fake.lastListReq.Filters["category"])
	}
	for _, want := range []string{"tcareer_it", "tcareer_back_office", "tcareer_work_with_clients"} {
		if !slices.Contains(cats, want) {
			t.Errorf("list filters.category %v missing %q", cats, want)
		}
	}
}

func TestTBankFetchPaginatesAndMapsMixedBlocks(t *testing.T) {
	// Page at offset 0 → not finished, next offset 20; page at offset 20 → finished.
	fake := &tbankHTTP{
		pages: map[int]string{
			0: tbankListPage(20, false,
				tbankVacancy("Backend Engineer", "Москва", "tcareer_it", "slug-1", "backend-engineer", "Удалённо", "Senior"),
			),
			20: tbankListPage(40, true,
				tbankVacancy("Sales Rep", "Казань", "tcareer_sales", "slug-2", "sales-rep", "Разъездной"),
			),
		},
		detail: map[string]string{
			// Mixed blocks: a string-content block and an array-content block.
			"slug-1": `{"resultCode":"OK","payload":{"description":[` +
				`{"title":"Описание","key":"shortDescription","content":"<ul><li>Build APIs.</li></ul><script>alert(1)</script>"},` +
				`{"title":"Что делать","key":"Что делать","content":[` +
				`{"description":"Писать сервисы","title":"Сервисы"},` +
				`{"description":"Ревьюить код","title":null}]}` +
				`]}}`,
			"slug-2": `{"resultCode":"OK","payload":{"description":[` +
				`{"title":"Условия","key":"Условия","content":"<p>Drive around.</p>"}` +
				`]}}`,
		},
	}

	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank", Provider: "tbank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one per page across pagination)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j1, ok := byID["slug-1"]
	if !ok {
		t.Fatal("slug-1 missing")
	}
	if j1.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "T-Bank" {
		t.Errorf("Company = %q, want T-Bank", j1.Company)
	}
	if want := "https://www.tbank.ru/career/it/vacancy/all/backend-engineer/slug-1/"; j1.URL != want {
		t.Errorf("URL = %q, want %q", j1.URL, want)
	}
	if j1.Location != "Москва" {
		t.Errorf("Location = %q, want subtitle", j1.Location)
	}
	// String-content block assembled, array-content block's titles + descriptions assembled,
	// script stripped by sanitize.
	if strings.Contains(j1.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j1.Description)
	}
	if !strings.Contains(j1.Description, "Build APIs.") {
		t.Errorf("Description missing string-block content: %q", j1.Description)
	}
	if !strings.Contains(j1.Description, "Писать сервисы") || !strings.Contains(j1.Description, "Сервисы") {
		t.Errorf("Description missing array-block content/title: %q", j1.Description)
	}
	if !j1.Remote {
		t.Error("Remote = false, want true (Удалённо tag)")
	}
	if j1.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil", j1.PostedAt)
	}

	if byID["slug-2"].Remote {
		t.Error("slug-2 Remote = true, want false (no remote tag)")
	}
}

func TestTBankFailedDetailDropsOnlyThatPosting(t *testing.T) {
	fake := &tbankHTTP{
		pages: map[int]string{
			0: tbankListPage(20, true,
				tbankVacancy("Kept", "Москва", "c", "ok", "kept"),
				tbankVacancy("Dropped", "Москва", "c", "bad", "dropped"),
			),
		},
		detail:    map[string]string{"ok": `{"resultCode":"OK","payload":{"description":[{"key":"k","content":"<p>ok</p>"}]}}`},
		failSlugs: map[string]bool{"bad": true},
	}

	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "ok" {
		t.Fatalf("got %v, want only the ok posting", jobs)
	}
}

// A stale gateway can return isFinished=false while no longer advancing the offset; the
// loop must terminate on a non-advancing offset rather than spin forever.
func TestTBankListTerminatesWhenOffsetStopsAdvancing(t *testing.T) {
	fake := &tbankHTTP{
		pages: map[int]string{
			// next offset 0 == current offset, never finished: the server is not advancing.
			0: tbankListPage(0, false, tbankVacancy("Stuck", "Москва", "c", "s1", "stuck")),
		},
		detail: map[string]string{"s1": `{"resultCode":"OK","payload":{"description":[{"key":"k","content":"<p>x</p>"}]}}`},
	}

	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1 (loop must stop when offset does not advance)", fake.listCalls)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
}

func TestTBankEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := &tbankHTTP{pages: map[int]string{0: tbankListPage(0, true)}}

	jobs, err := NewTBank(fake).Fetch(context.Background(), CompanyEntry{Company: "T-Bank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
