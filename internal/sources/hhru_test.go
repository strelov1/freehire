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

	"golang.org/x/net/html"
)

// hhFake routes search calls by their page param (returning a page's embedded-state HTML) and
// detail calls by the vacancy id in the URL, so one fake drives both stages of the crawl.
type hhFake struct {
	searchByPage map[int][]hhVacancy // page -> vacancies in that page's state
	detailByID   map[string]string   // vacancy id -> ld+json description
	detailErr    map[string]bool     // vacancy id -> GetHTML returns an error
	searchPages  []int               // search pages requested, in order
	detailHits   []string            // vacancy ids whose detail page was fetched, in order
}

func (f *hhFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	if strings.Contains(pu.Path, "/search/vacancy") {
		page, _ := strconv.Atoi(pu.Query().Get("page"))
		f.searchPages = append(f.searchPages, page)
		return html.Parse(strings.NewReader(hhSearchHTML(f.searchByPage[page])))
	}
	id := pu.Path[strings.LastIndex(pu.Path, "/")+1:]
	f.detailHits = append(f.detailHits, id)
	if f.detailErr[id] {
		return nil, errors.New("detail boom")
	}
	return html.Parse(strings.NewReader(hhDetailHTML(f.detailByID[id])))
}

// hhSearchHTML wraps the vacancy list in an HH-Lux-InitialState template, mirroring the real
// server-rendered search page. Marshaling hhState round-trips through the adapter's own decode.
func hhSearchHTML(vs []hhVacancy) string {
	var st hhState
	st.VacancySearchResult.Vacancies = vs
	b, _ := json.Marshal(st)
	return `<html><body><template style="display:none" id="HH-Lux-InitialState">` +
		string(b) + `</template></body></html>`
}

// hhDetailHTML wraps a JobPosting ld+json carrying the given description, as the vacancy page does.
func hhDetailHTML(desc string) string {
	if desc == "" {
		return `<html><body><p>no ld+json here</p></body></html>`
	}
	b, _ := json.Marshal(map[string]any{"@type": "JobPosting", "description": desc})
	return `<html><body><script type="application/ld+json">` + string(b) + `</script></body></html>`
}

// hhVac builds a minimal usable listing vacancy (id, title, company, desktop link).
func hhVac(id int64, name, company string) hhVacancy {
	var v hhVacancy
	v.VacancyID = id
	v.Name = name
	v.Company.VisibleName = company
	v.Links.Desktop = hhVacancyURL + strconv.FormatInt(id, 10)
	return v
}

func i64(n int64) *int64 { return &n }
func boolp(b bool) *bool { return &b }

func TestHHFetchMapsListingAndSkipsAds(t *testing.T) {
	dev := hhVac(101, "Go Developer", "Контур")
	dev.Area.Name = "Новосибирск"
	dev.Employment.Type = "FULL"
	dev.WorkFormats = []struct {
		Elements []string `json:"workFormatsElement"`
	}{{Elements: []string{"REMOTE", "HYBRID"}}}
	dev.PublicationTime.Value = "2026-07-06T20:42:48.702+03:00"
	dev.Compensation = hhCompensation{From: i64(200000), To: i64(300000), CurrencyCode: "RUR", Gross: boolp(true)}

	ad := hhVac(999, "Promoted elsewhere", "AdCo")
	ad.IsAdv = true

	fake := &hhFake{searchByPage: map[int][]hhVacancy{0: {dev, ad}}}
	jobs, err := NewHH(fake).Fetch(context.Background(), CompanyEntry{Provider: "hh", Board: "96"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (ad skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "101" || j.Title != "Go Developer" || j.Company != "Контур" {
		t.Errorf("id/title/company wrong: %q / %q / %q", j.ExternalID, j.Title, j.Company)
	}
	if j.URL != "https://hh.ru/vacancy/101" || j.Location != "Новосибирск" {
		t.Errorf("url/location wrong: %q / %q", j.URL, j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("workFormats REMOTE → Remote=%v WorkMode=%q, want true/remote", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-06" {
		t.Errorf("PostedAt = %v, want 2026-07-06", j.PostedAt)
	}
	// Fetch is list-only: the salary the list carries is folded in, but there is no body yet.
	if !strings.Contains(j.Description, "200000–300000 RUR") || !strings.Contains(j.Description, "до вычета налогов") {
		t.Errorf("salary paragraph missing from description: %q", j.Description)
	}
}

func TestHHWorkModeAndEmploymentMapping(t *testing.T) {
	wf := func(els ...string) []struct {
		Elements []string `json:"workFormatsElement"`
	} {
		return []struct {
			Elements []string `json:"workFormatsElement"`
		}{{Elements: els}}
	}
	cases := []struct {
		formats  []string
		wantMode string
	}{
		{[]string{"ON_SITE", "HYBRID"}, "hybrid"},
		{[]string{"ON_SITE"}, "onsite"},
		{[]string{"REMOTE", "ON_SITE"}, "remote"},
		{nil, ""},
	}
	for _, c := range cases {
		v := hhVac(1, "T", "Co")
		v.WorkFormats = wf(c.formats...)
		if c.formats == nil {
			v.WorkFormats = nil
		}
		if got := v.workMode(); got != c.wantMode {
			t.Errorf("workMode(%v) = %q, want %q", c.formats, got, c.wantMode)
		}
	}
	for typ, want := range map[string]string{"FULL": "full_time", "PART": "part_time", "PROJECT": "contract", "PROBATION": ""} {
		if got := hhEmploymentType(typ); got != want {
			t.Errorf("hhEmploymentType(%q) = %q, want %q", typ, got, want)
		}
	}
}

func TestHHCleansCompanyOrgSuffix(t *testing.T) {
	v := hhVac(1, "T", "HeadHunter::Analytics/Data Science")
	j, ok := v.toJob()
	if !ok || j.Company != "HeadHunter" {
		t.Errorf("company = %q, want HeadHunter (org-path suffix stripped)", j.Company)
	}
	// A clean name (no "::") is untouched.
	if got := hhCompanyName("МТС Банк. Головной офис"); got != "МТС Банк. Головной офис" {
		t.Errorf("clean company altered: %q", got)
	}
}

func TestHHDropsUnusablePostings(t *testing.T) {
	noCompany := hhVac(1, "T", "")
	noID := hhVac(0, "T", "Co")
	fake := &hhFake{searchByPage: map[int][]hhVacancy{0: {hhVac(2, "Ok", "Co"), noCompany, noID}}}
	jobs, err := NewHH(fake).Fetch(context.Background(), CompanyEntry{Board: "96"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "2" {
		t.Fatalf("want only the usable posting; got %d jobs", len(jobs))
	}
}

func TestHHFetchNewHydratesOnlyNew(t *testing.T) {
	newV := hhVac(101, "New role", "Co")
	seenV := hhVac(202, "Seen role", "Co")
	failV := hhVac(303, "Detail fails", "Co")
	fake := &hhFake{
		searchByPage: map[int][]hhVacancy{0: {newV, seenV, failV}},
		detailByID:   map[string]string{"101": "<p>Full body for the new role.</p>"},
		detailErr:    map[string]bool{"303": true},
	}
	seen := func(id string) bool { return id == "202" }
	jobs, err := NewHH(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "96"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The new posting is hydrated with its body; the seen one is a SeenRefresh with no detail fetch.
	if d := byID["101"].Description; !strings.Contains(d, "Full body for the new role") {
		t.Errorf("new posting not hydrated: %q", d)
	}
	if byID["101"].SeenRefresh {
		t.Error("new posting must not be marked SeenRefresh")
	}
	if !byID["202"].SeenRefresh {
		t.Error("seen posting must be marked SeenRefresh")
	}
	// Only the new and the (attempted) failing posting hit the detail page; the seen one is skipped.
	if slices.Contains(fake.detailHits, "202") {
		t.Errorf("seen posting should not fetch detail; detail hits = %v", fake.detailHits)
	}
	// A detail failure still emits the list-only job (never dropped).
	if _, ok := byID["303"]; !ok {
		t.Error("posting with failing detail should still be emitted list-only")
	}
}

func TestHHPaginatesAndStops(t *testing.T) {
	full := make([]hhVacancy, hhPageSize)
	for i := range full {
		full[i] = hhVac(int64(1000+i), "T", "Co")
	}
	short := []hhVacancy{hhVac(1, "T", "Co"), hhVac(2, "T", "Co")}
	fake := &hhFake{searchByPage: map[int][]hhVacancy{0: full, 1: short}} // page 2 absent → empty → stop
	jobs, err := NewHH(fake).Fetch(context.Background(), CompanyEntry{Board: "96"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.searchPages, []int{0, 1, 2}) {
		t.Errorf("requested pages = %v, want [0 1 2] (0/1 full/short, 2 empty stops)", fake.searchPages)
	}
	if len(jobs) != hhPageSize+2 {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), hhPageSize+2)
	}
}

func TestHHFirstPageFailureIsBoardError(t *testing.T) {
	fake := &hhFake{searchByPage: map[int][]hhVacancy{}} // page 0 yields empty state, not an error
	// An empty first page is not an error — it is a role with no fresh vacancies.
	jobs, err := NewHH(fake).Fetch(context.Background(), CompanyEntry{Board: "96"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty first page: jobs=%d err=%v, want 0/nil", len(jobs), err)
	}
}

func TestHHProviderRegisteredAndAggregator(t *testing.T) {
	if got := NewHH(nil).Provider(); got != "hh" {
		t.Errorf("Provider() = %q, want hh", got)
	}
	if _, ok := All(nil)["hh"]; !ok {
		t.Error("All() should register provider hh")
	}
	if !slices.Contains(FilterableProviders(), "hh") {
		t.Error("FilterableProviders() should include hh")
	}
	if !slices.Contains(AggregatorProviders(All(nil)), "hh") {
		t.Error("AggregatorProviders() should include hh (multi-company aggregator)")
	}
}

func TestHHBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/hh.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/hh.yml fails validation: %v", err)
	}
}
