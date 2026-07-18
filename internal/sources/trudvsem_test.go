package sources

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// trudvsemFake routes each vacancies request by its offset param, so one fake drives the
// whole region-shard pagination loop.
type trudvsemFake struct {
	byOffset   map[int]string // offset -> response JSON ("" => empty page envelope)
	offsets    []int          // offsets requested, in order
	gotRegions []string       // region codes taken from the request path
}

func (f *trudvsemFake) GetJSON(_ context.Context, u string, v any) error {
	pu, _ := url.Parse(u)
	// .../vacancies/region/<code>?offset=&limit=
	if i := strings.LastIndex(pu.Path, "/region/"); i >= 0 {
		f.gotRegions = append(f.gotRegions, pu.Path[i+len("/region/"):])
	}
	offset, _ := strconv.Atoi(pu.Query().Get("offset"))
	f.offsets = append(f.offsets, offset)
	body := f.byOffset[offset]
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

func TestTrudvsemBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/trudvsem.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/trudvsem.yml fails validation: %v", err)
	}
}
