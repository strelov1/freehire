package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// trudvsemFake routes each vacancies request by its offset param, so one fake drives the
// whole region-shard pagination loop.
type trudvsemFake struct {
	byOffset   map[int]string            // offset -> response JSON, unwindowed ("" => empty page envelope)
	byWindow   map[string]map[int]string // modifiedTo -> offset -> response JSON, windowed
	errAt      map[int]error             // offset -> transport error (simulates a 500/timeout blip)
	offsets    []int                     // offsets requested, in order
	windows    []string                  // each request's modifiedTo ("" when unwindowed), in order
	gotRegions []string                  // region codes taken from the request path
}

func (f *trudvsemFake) GetJSON(_ context.Context, u string, v any) error {
	pu, _ := url.Parse(u)
	// .../vacancies/region/<code>?offset=&limit=
	if i := strings.LastIndex(pu.Path, "/region/"); i >= 0 {
		f.gotRegions = append(f.gotRegions, pu.Path[i+len("/region/"):])
	}
	offset, _ := strconv.Atoi(pu.Query().Get("offset"))
	window := pu.Query().Get("modifiedTo")
	f.offsets = append(f.offsets, offset)
	f.windows = append(f.windows, window)
	if err := f.errAt[offset]; err != nil && window == "" {
		return err
	}
	body := f.byOffset[offset]
	if window != "" {
		body = f.byWindow[window][offset]
	}
	if body == "" {
		body = `{"status":"200","meta":{"total":0},"results":{"vacancies":[]}}`
	}
	return json.Unmarshal([]byte(body), v)
}

func TestTrudvsemFetchMaps(t *testing.T) {
	const page = `{"status":"200","meta":{"total":3},"results":{"vacancies":[
	  {"vacancy":{"id":"AA-1","source":"Вакансия работодателя",
	    "region":{"region_code":"7700000000000","name":"Город Москва"},
	    "company":{"name":"ООО Рога и Копыта"},
	    "creation-date":"2026-07-15",
	    "job-name":"Программист",
	    "vac_url":"https://trudvsem.ru/vacancy/card/123/AA-1",
	    "employment":"Полная занятость",
	    "requirements":"Опыт от 3 лет.",
	    "duty":"Писать код."}},
	  {"vacancy":{"id":"","job-name":"No id","region":{"name":"X"},"company":{"name":"Y"}}},
	  {"vacancy":{"id":"CC-3","job-name":"  ","region":{"name":"X"},"company":{"name":"Y"}}},
	  {"vacancy":{"id":"DD-4","job-name":"Стажёр",
	    "region":{"region_code":"7700000000000","name":"Город Москва"},
	    "company":{"name":""},
	    "creation-date":"2026-07-10",
	    "vac_url":"https://trudvsem.ru/vacancy/card/9/DD-4",
	    "employment":"Стажировка",
	    "duty":"Учиться."}}
	]}}`
	fake := &trudvsemFake{byOffset: map[int]string{0: page}}
	jobs, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Работа России", Provider: "trudvsem", Board: "7700000000000",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The empty-id and blank-title vacancies are dropped; two map.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (empty id / blank title dropped)", len(jobs))
	}
	if len(fake.gotRegions) == 0 || fake.gotRegions[0] != "7700000000000" {
		t.Errorf("region path = %v, want first 7700000000000", fake.gotRegions)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["AA-1"]
	if !ok {
		t.Fatalf("AA-1 not mapped")
	}
	if j.URL != "https://trudvsem.ru/vacancy/card/123/AA-1" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Программист" || j.Company != "ООО Рога и Копыта" {
		t.Errorf("title/company wrong: %q / %q", j.Title, j.Company)
	}
	if j.Location != "Город Москва" {
		t.Errorf("Location = %q, want region name", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-15" {
		t.Errorf("PostedAt = %v, want 2026-07-15", j.PostedAt)
	}
	// duty + requirements are rebuilt into paragraph HTML.
	if d := j.Description; !strings.Contains(d, "Писать код") || !strings.Contains(d, "Опыт от 3 лет") || !strings.Contains(d, "<p>") {
		t.Errorf("Description = %q, want both duty and requirements in <p> structure", d)
	}
	// A vacancy with a blank employer name falls back to the entry's company (the hub name).
	if dd := byID["DD-4"]; dd.Company != "Работа России" {
		t.Errorf("DD-4 Company = %q, want fallback to entry company", dd.Company)
	}
	if dd := byID["DD-4"]; dd.EmploymentType != "internship" {
		t.Errorf("DD-4 EmploymentType = %q, want internship", dd.EmploymentType)
	}
}

// Trudvsem files remote work as an EMPLOYMENT form, "Дистанционная (удаленная) работа" — the
// Labour Code's own category, picked from the board's closed list — so it is the posting's
// structured work mode. No other employment value or schedule states one: "Полный рабочий день"
// is hours, not a place. The record is trimmed from the live API (2026-10-30).
func TestTrudvsemRemoteEmploymentIsTheWorkMode(t *testing.T) {
	const page = `{"status":"200","meta":{"total":2},"results":{"vacancies":[
	  {"vacancy":{"id":"3386d350-1ed4-11f1-b26c-af1153b4c800","job-name":"Менеджер по продажам (удаленно)",
	    "region":{"region_code":"7700000000000","name":"Город Москва"},"company":{"name":"ООО \"МЕДИС\""},
	    "employment":"Дистанционная (удаленная) работа","schedule":"Полный рабочий день",
	    "duty":"Менеджер по входящим продажам (удалённо)"}},
	  {"vacancy":{"id":"cfbc6315-5df3-11ee-b174-d549be31d974","job-name":"инженер",
	    "region":{"region_code":"6600000000000","name":"Свердловская область"},"company":{"name":"МКУ \"СЗГХ\""},
	    "employment":"Полная занятость","schedule":"Полный рабочий день"}}
	]}}`
	jobs, err := NewTrudvsem(&trudvsemFake{byOffset: map[int]string{0: page}}).Fetch(context.Background(),
		CompanyEntry{Company: "Работа России", Provider: "trudvsem", Board: "7700000000000"})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("Fetch = %d jobs, %v", len(jobs), err)
	}
	if r := jobs[0]; r.WorkMode != "remote" || !r.Remote {
		t.Errorf("remote employment: WorkMode/Remote = %q/%v, want remote/true", r.WorkMode, r.Remote)
	}
	if o := jobs[1]; o.WorkMode != "" || o.Remote {
		t.Errorf("full-time employment: WorkMode/Remote = %q/%v, want empty/false", o.WorkMode, o.Remote)
	}
}

func TestTrudvsemPaginates(t *testing.T) {
	// offset is a 0-based page index: page 0 full => keep going, page 1 short => stop.
	full := trudvsemPage(trudvsemPageSize, 0, 100000)
	short := trudvsemPage(3, 100, 100000)
	fake := &trudvsemFake{byOffset: map[int]string{0: full, 1: short}}
	jobs, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "5200000000000"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.offsets, []int{0, 1}) {
		t.Errorf("requested pages = %v, want [0 1]", fake.offsets)
	}
	if len(jobs) != trudvsemPageSize+3 {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), trudvsemPageSize+3)
	}
}

func TestTrudvsemStopsAtTotal(t *testing.T) {
	// A first full page whose size already covers the total must not request page 1.
	full := trudvsemPage(trudvsemPageSize, 0, trudvsemPageSize)
	fake := &trudvsemFake{byOffset: map[int]string{0: full}}
	if _, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "5200000000000"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.offsets, []int{0}) {
		t.Errorf("requested pages = %v, want [0] (total reached)", fake.offsets)
	}
}

func TestTrudvsemStopsAtTotalMultiplePages(t *testing.T) {
	// total=200 is an exact multiple of the page size: pages 0 and 1 cover it, so the loop
	// must stop after page 1 rather than requesting page 2 (which the API 500s past the end).
	fake := &trudvsemFake{byOffset: map[int]string{
		0: trudvsemPage(trudvsemPageSize, 0, 200),
		1: trudvsemPage(trudvsemPageSize, 100, 200),
	}}
	jobs, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "5200000000000"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.offsets, []int{0, 1}) {
		t.Errorf("requested pages = %v, want [0 1] (no page 2 past the end)", fake.offsets)
	}
	if len(jobs) != 2*trudvsemPageSize {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), 2*trudvsemPageSize)
	}
}

func TestTrudvsemFailsOnMidWalkError(t *testing.T) {
	// The board-scoped sweep reads a successful Fetch as the region's WHOLE listing
	// (fullBoardListing), so a blip on page 1 after a full page 0 must fail the board rather
	// than hand back page 0 as if it were everything — the unread pages would close as gone.
	fake := &trudvsemFake{
		byOffset: map[int]string{0: trudvsemPage(trudvsemPageSize, 0, 100000)},
		errAt:    map[int]error{1: errors.New("status 500")},
	}
	if _, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "5200000000000"}); err == nil {
		t.Fatal("Fetch returned nil error on a mid-walk blip, want the board to fail")
	}
}

func TestTrudvsemClaimsFullBoardListing(t *testing.T) {
	if _, ok := NewTrudvsem(nil).(fullBoardListing); !ok {
		t.Fatal("trudvsem must claim fullBoardListing: Fetch walks every region to its end")
	}
}

// trudvsemPastCeiling is a region 50 postings past the API's 100-page ceiling: the unwindowed
// listing's 100 full pages, and the modifiedTo bound the adapter must ask for next — the
// oldest date_modify those pages carry (posting #9999), plus the overlap.
func trudvsemPastCeiling(newest time.Time) (total int, unwindowed map[int]string, window string) {
	total = trudvsemMaxPages*trudvsemPageSize + 50
	unwindowed = map[int]string{}
	for p := range trudvsemMaxPages {
		unwindowed[p] = trudvsemDatedPage(trudvsemPageSize, p*trudvsemPageSize, total, newest)
	}
	oldest := newest.Add(-time.Duration(trudvsemMaxPages*trudvsemPageSize-1) * time.Minute)
	return total, unwindowed, oldest.Add(trudvsemWindowOverlap).UTC().Format(time.RFC3339)
}

func TestTrudvsemSplitsRegionPastPageCap(t *testing.T) {
	// A region holding more than the 100-page ceiling is read in date windows: the unwindowed
	// walk yields the freshest 10k, then a window bounded by modifiedTo is walked to its end.
	// The overlap's repeats are dropped, so every posting appears once.
	newest := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	total, unwindowed, window := trudvsemPastCeiling(newest)
	fake := &trudvsemFake{
		byOffset: unwindowed,
		// The window re-reads the ten postings before #9999 (the overlap) and the 50 past it.
		byWindow: map[string]map[int]string{window: {
			0: trudvsemDatedPage(60, trudvsemMaxPages*trudvsemPageSize-10, 60, newest),
		}},
	}
	jobs, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "7700000000000"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		if ids[j.ExternalID] {
			t.Fatalf("posting %s returned twice", j.ExternalID)
		}
		ids[j.ExternalID] = true
	}
	if len(ids) != total {
		t.Fatalf("got %d postings, want %d", len(ids), total)
	}
	if got := fake.windows[len(fake.windows)-1]; got != window {
		t.Errorf("last request's modifiedTo = %q, want %q", got, window)
	}
}

func TestTrudvsemSplitFailsWithoutProgress(t *testing.T) {
	// A window no smaller than the one before it would walk the same ceiling forever: fail the
	// board instead of returning a listing that is not whole.
	newest := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	_, unwindowed, window := trudvsemPastCeiling(newest)
	fake := &trudvsemFake{byOffset: unwindowed, byWindow: map[string]map[int]string{window: unwindowed}}
	if _, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "7700000000000"}); err == nil {
		t.Fatal("Fetch returned nil error for a window that did not shrink, want an error")
	}
}

func TestTrudvsemSplitFailsWithoutModifyDate(t *testing.T) {
	// Past the ceiling the next window is bounded by the oldest date_modify read; with none
	// there is no bound to give, so the region cannot be read whole and the board fails.
	total := trudvsemMaxPages*trudvsemPageSize + 50
	unwindowed := map[int]string{}
	for p := range trudvsemMaxPages {
		unwindowed[p] = trudvsemPage(trudvsemPageSize, p*trudvsemPageSize, total)
	}
	fake := &trudvsemFake{byOffset: unwindowed}
	if _, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "7700000000000"}); err == nil {
		t.Fatal("Fetch returned nil error past the ceiling with no date_modify, want an error")
	}
}

// trudvsemDatedPage is trudvsemPage with a date_modify on each vacancy, in the API's own
// format: posting #k was modified k minutes before newest, so the listing is newest-first.
func trudvsemDatedPage(n, base, total int, newest time.Time) string {
	msk := time.FixedZone("MSK", 3*3600)
	items := make([]string, n)
	for i := range items {
		k := base + i
		modified := newest.Add(-time.Duration(k) * time.Minute).In(msk).Format("2006-01-02T15:04:05-0700")
		items[i] = `{"vacancy":{"id":"R-` + strconv.Itoa(k) + `","job-name":"T","region":{"name":"Город Москва"},` +
			`"company":{"name":"Co"},"creation-date":"2026-07-15","date_modify":"` + modified + `"}}`
	}
	return `{"status":"200","meta":{"total":` + strconv.Itoa(total) + `},"results":{"vacancies":[` + strings.Join(items, ",") + `]}}`
}

func TestTrudvsemFailsWhenFirstPageErrors(t *testing.T) {
	// A blip on page 0 harvested nothing, so the region is a genuine failure the pipeline must
	// see (board_health) rather than a silent empty success.
	fake := &trudvsemFake{errAt: map[int]error{0: errors.New("status 500")}}
	if _, err := NewTrudvsem(fake).Fetch(context.Background(), CompanyEntry{Board: "5200000000000"}); err == nil {
		t.Fatal("Fetch returned nil error when page 0 failed, want an error")
	}
}

// trudvsemPage builds a response of n vacancies with ids offset by base and the given total.
func trudvsemPage(n, base, total int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"vacancy":{"id":"R-` + strconv.Itoa(base+i) +
			`","job-name":"T","region":{"name":"Нижегородская область"},"company":{"name":"Co"},"creation-date":"2026-07-15"}}`
	}
	return `{"status":"200","meta":{"total":` + strconv.Itoa(total) + `},"results":{"vacancies":[` + strings.Join(items, ",") + `]}}`
}

func TestTrudvsemProviderRegistered(t *testing.T) {
	if got := NewTrudvsem(nil).Provider(); got != "trudvsem" {
		t.Errorf("Provider() = %q, want trudvsem", got)
	}
	if _, ok := All(nil)["trudvsem"]; !ok {
		t.Error("All() should register provider trudvsem")
	}
	if !slices.Contains(FilterableProviders(), "trudvsem") {
		t.Error("FilterableProviders() should include trudvsem")
	}
}

// The employer's own bounds are read from the numbers, never the text: the text reads
// "от N" for a real range too. A zero bound is unstated, and a posting with neither
// states no salary at all.
func TestTrudvsemReadsTheStatedSalary(t *testing.T) {
	const page = `{"status":"200","meta":{"total":3},"results":{"vacancies":[
	  {"vacancy":{"id":"a","job-name":"Инженер","salary":"от 280000","salary_min":280000,"salary_max":380000,"currency":"«руб.»"}},
	  {"vacancy":{"id":"b","job-name":"Водитель","salary":"от 180000","salary_min":180000,"salary_max":0,"currency":"«руб.»"}},
	  {"vacancy":{"id":"c","job-name":"Кладовщик","salary":"от 0","salary_min":0,"salary_max":0,"currency":"«руб.»"}}
	]}}`
	jobs, err := NewTrudvsem(&trudvsemFake{byOffset: map[int]string{0: page}}).Fetch(context.Background(),
		CompanyEntry{Company: "Работа России", Provider: "trudvsem", Board: "7700000000000"})
	if err != nil || len(jobs) != 3 {
		t.Fatalf("Fetch = %d jobs, %v", len(jobs), err)
	}
	if r := jobs[0]; r.SalaryMin == nil || *r.SalaryMin != 280000 || r.SalaryMax == nil || *r.SalaryMax != 380000 ||
		r.SalaryCurrency != "RUB" || r.SalaryPeriod != "month" {
		t.Errorf("range = %v-%v %q/%q, want 280000-380000 RUB/month", r.SalaryMin, r.SalaryMax, r.SalaryCurrency, r.SalaryPeriod)
	}
	if r := jobs[1]; r.SalaryMin == nil || *r.SalaryMin != 180000 || r.SalaryMax != nil {
		t.Errorf("one-sided = %v-%v, want 180000-nil", r.SalaryMin, r.SalaryMax)
	}
	if r := jobs[2]; r.SalaryMin != nil || r.SalaryMax != nil || r.SalaryCurrency != "" || r.SalaryPeriod != "" {
		t.Errorf("unstated = %v-%v %q/%q, want no salary", r.SalaryMin, r.SalaryMax, r.SalaryCurrency, r.SalaryPeriod)
	}
}
