package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// fakeMTS is a body-aware test fake for MTS (the mtsHTTP roles): the list is a POST that paginates on the
// request body's offset, the detail is a GET keyed by the vacancy id in the URL, and both
// must carry the harvested x-api-key header. The fake records the header seen on each call
// so the test can assert it is sent. GetHTML serves a canned config page for the apiKey
// harvest. A detail id in failIDs errors so detail-isolation can be exercised.
type fakeMTS struct {
	configHTML string            // page served by GetHTML (carries apiKey)
	pages      map[int]string    // offset -> canned list payload
	detail     map[string]string // id -> canned detail payload
	failIDs    map[string]bool   // ids whose detail GET errors

	listKey string // x-api-key seen on the list POST
	// The adapter fans its detail fetches out across a worker pool, so this recorder is
	// written from several goroutines at once and needs the lock. The assertions read it
	// after Fetch has joined the pool, which is already ordered.
	mu        sync.Mutex
	detailKey string // x-api-key seen on a detail GET
}

func (f *fakeMTS) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return html.Parse(strings.NewReader(f.configHTML))
}

func (f *fakeMTS) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.detailKey = headers["x-api-key"]
	f.mu.Unlock()
	id := url[strings.LastIndex(url, "/")+1:]
	if f.failIDs[id] {
		return errors.New("fakeMTS: detail boom for " + id)
	}
	raw, ok := f.detail[id]
	if !ok {
		return errors.New("fakeMTS: no canned detail for " + id)
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *fakeMTS) PostJSONWithHeaders(_ context.Context, _ string, headers map[string]string, body, v any) error {
	f.listKey = headers["x-api-key"]
	req, ok := body.(mtsListRequest)
	if !ok {
		return errors.New("fakeMTS: list body is not a mtsListRequest")
	}
	raw, ok := f.pages[req.Offset]
	if !ok {
		return errors.New("fakeMTS: no canned page for offset")
	}
	return json.Unmarshal([]byte(raw), v)
}

// mtsListPage builds one list payload with the given pageInfo total and inline vacancy
// fragments.
func mtsListPage(total int, vacancies ...string) string {
	return `{"success":true,"data":{"pageInfo":{"limit":200,"offset":0,"total":` + itoa(total) +
		`},"vacancies":[` + strings.Join(vacancies, ",") + `]}}`
}

func mtsVacancy(id, name, brand, city, worktype string) string {
	return `{"id":"` + id + `","name":"` + name + `","info":{"brand":"` + brand +
		`","city":"` + city + `","worktype":"` + worktype + `"}}`
}

func mtsDetail(project, descr, req, cond string) string {
	return `{"success":true,"data":{"vacancy":{"detailText":{"descriptionOfProject":"` + project +
		`","description":"` + descr + `","requirements":"` + req + `","conditions":"` + cond + `"}}}}`
}

const mtsConfigHTML = `<html><body><script>window.__NUXT__=(function(){return {config:{public:{legacyApiBase:"https://api.job.mts.ru/v1",apiKey:"KEY-abc.def-123_xyz",cackleWid:42}}}}())</script></body></html>`

func TestMTSProvider(t *testing.T) {
	if got := NewMTS(nil).Provider(); got != "mts" {
		t.Errorf("Provider() = %q, want %q", got, "mts")
	}
}

func TestMTSIsBoardless(t *testing.T) {
	if _, ok := NewMTS(nil).(boardless); !ok {
		t.Error("mts should implement the boardless marker")
	}
}

func TestMTSAPIKeyExtraction(t *testing.T) {
	root, err := html.Parse(strings.NewReader(mtsConfigHTML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := mtsExtractAPIKey(root)
	if got != "KEY-abc.def-123_xyz" {
		t.Errorf("mtsExtractAPIKey = %q, want KEY-abc.def-123_xyz", got)
	}
}

func TestMTSAPIKeyExtractionMissing(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(`<html><body><script>window.x=1</script></body></html>`))
	if got := mtsExtractAPIKey(root); got != "" {
		t.Errorf("mtsExtractAPIKey = %q, want empty when absent", got)
	}
}

func TestMTSFetchPaginatesMapsAndSendsKey(t *testing.T) {
	// total=400 forces two pages at the fixed 200 page size: offset 0 then offset 200.
	fake := &fakeMTS{
		configHTML: mtsConfigHTML,
		pages: map[int]string{
			0: mtsListPage(400,
				mtsVacancy("100", "Backend Engineer", "МТС Диджитал", "Москва", "Удалённо"),
			),
			200: mtsListPage(400,
				mtsVacancy("200", "Sales Manager", "", "Казань", "В офисе"),
			),
		},
		detail: map[string]string{
			"100": mtsDetail("Проект А", "Делать сервисы", "Знать Go", "ДМС"),
			"200": mtsDetail("", "Продавать", "Опыт продаж", "Бонусы"),
		},
	}

	jobs, err := NewMTS(fake).Fetch(context.Background(), CompanyEntry{Company: "MTS", Provider: "mts"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one per page to total)", len(jobs))
	}
	if fake.listKey != "KEY-abc.def-123_xyz" {
		t.Errorf("list x-api-key = %q, want harvested key", fake.listKey)
	}
	if fake.detailKey != "KEY-abc.def-123_xyz" {
		t.Errorf("detail x-api-key = %q, want harvested key", fake.detailKey)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j1, ok := byID["100"]
	if !ok {
		t.Fatal("vacancy 100 missing")
	}
	if j1.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "МТС Диджитал" {
		t.Errorf("Company = %q, want info.brand", j1.Company)
	}
	if want := "https://job.mts.ru/vacancy/100"; j1.URL != want {
		t.Errorf("URL = %q, want %q", j1.URL, want)
	}
	if j1.Location != "Москва" {
		t.Errorf("Location = %q, want info.city", j1.Location)
	}
	if !strings.Contains(j1.Description, "Делать сервисы") || !strings.Contains(j1.Description, "ДМС") {
		t.Errorf("Description not assembled from detailText: %q", j1.Description)
	}
	if !j1.Remote {
		t.Error("Remote = false, want true (worktype Удалённо)")
	}
	if j1.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil", j1.PostedAt)
	}

	j2 := byID["200"]
	if j2.Company != "MTS" {
		t.Errorf("200 Company = %q, want fallback to entry company MTS (empty brand)", j2.Company)
	}
	if j2.Remote {
		t.Error("200 Remote = true, want false (В офисе)")
	}
}

func TestMTSFailedDetailDropsOnlyThatPosting(t *testing.T) {
	fake := &fakeMTS{
		configHTML: mtsConfigHTML,
		pages: map[int]string{
			0: mtsListPage(1,
				mtsVacancy("1", "Kept", "MTS", "Москва", "В офисе"),
				mtsVacancy("2", "Dropped", "MTS", "Москва", "В офисе"),
			),
		},
		detail:  map[string]string{"1": mtsDetail("", "ok", "", "")},
		failIDs: map[string]bool{"2": true},
	}

	jobs, err := NewMTS(fake).Fetch(context.Background(), CompanyEntry{Company: "MTS"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestMTSHarvestFailureErrors(t *testing.T) {
	// Config page without an apiKey → harvest fails → Fetch errors so the board is isolated.
	fake := &fakeMTS{configHTML: `<html><body><script>window.x=1</script></body></html>`}
	if _, err := NewMTS(fake).Fetch(context.Background(), CompanyEntry{Company: "MTS"}); err == nil {
		t.Fatal("Fetch: want error when apiKey harvest fails, got nil")
	}
}

func TestMTSEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := &fakeMTS{configHTML: mtsConfigHTML, pages: map[int]string{0: mtsListPage(0)}}

	jobs, err := NewMTS(fake).Fetch(context.Background(), CompanyEntry{Company: "MTS"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
