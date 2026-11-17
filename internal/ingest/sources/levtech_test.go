package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// levtechTable flattens a Go value into Nuxt's devalue reference table the way the site's pages
// carry it: every object field and array element is an index, scalars sit in cells of their own,
// and the root is wrapped like the page's ["ShallowReactive", n] header.
func levtechTable(root any) []any {
	table := []any{nil}
	var add func(v any) int
	add = func(v any) int {
		switch x := v.(type) {
		case map[string]any:
			obj := make(map[string]any, len(x))
			idx := len(table)
			table = append(table, nil)
			for k, f := range x {
				obj[k] = add(f)
			}
			table[idx] = obj
			return idx
		case []any:
			idx := len(table)
			table = append(table, nil)
			refs := make([]any, 0, len(x))
			for _, e := range x {
				refs = append(refs, add(e))
			}
			table[idx] = refs
			return idx
		default:
			table = append(table, v)
			return len(table) - 1
		}
	}
	inner := add(root)
	table[0] = []any{"ShallowReactive", inner}
	return table
}

type levtechProj struct {
	ID       int
	Name     string
	Closed   bool
	Remote   int
	Contract string
	Station  string
	Pref     string
}

func levtechListPage(projects ...levtechProj) string {
	items := []any{}
	for _, p := range projects {
		if p.Contract == "" {
			p.Contract = "subcontractor"
		}
		items = append(items, map[string]any{
			"baseData": map[string]any{
				"contractType": p.Contract, "id": float64(p.ID), "name": p.Name,
				"payUpperLimit": float64(900000), "payType": "negotiations",
				"stationName": p.Station, "stationPrefectureName": p.Pref,
				"skills": []any{},
			},
			"isClosed":         p.Closed,
			"remoteWorkTypeId": float64(p.Remote),
		})
	}
	root := map[string]any{
		"data": map[string]any{
			"state":    map[string]any{"projects": []any{"decoy-not-a-project-list"}},
			"projects": map[string]any{"projects": items, "paginationMeta": map[string]any{"totalPages": float64(3183)}},
		},
	}
	b, _ := json.Marshal(levtechTable(root))
	return `<html><body><script type="application/json" id="__NUXT_DATA__">` + string(b) + `</script></body></html>`
}

func levtechDetailPage(id int, unit, value, locality string) string {
	ld := map[string]any{
		"@context": "http://schema.org", "@type": "JobPosting",
		"title":       "x",
		"description": "<span>・単価：900,000／月</span><br><h2>◆職務内容</h2><br><h3>AWS環境の構築</h3>",
		"datePosted":  "2026-11-17",
		"url":         levtechDetailURL(fmt.Sprint(id)),
		"baseSalary": map[string]any{"@type": "MonetaryAmount", "currency": "JPY",
			"value": map[string]any{"@type": "QuantitativeValue", "unitText": unit, "value": value}},
		"jobLocation": map[string]any{"@type": "Place", "address": map[string]any{
			"addressCountry": "JP", "addressLocality": locality, "addressRegion": "愛知県"}},
	}
	b, _ := json.Marshal(ld)
	return `<html><head><script type="application/ld+json">` + string(b) + `</script></head><body></body></html>`
}

// levtechFake serves a fixed set of pages by URL; anything else is a 404.
type levtechFake struct {
	mu    sync.Mutex
	pages map[string]string
	fail  map[string]bool
	asked []string
}

func (f *levtechFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.asked = append(f.asked, u)
	f.mu.Unlock()
	if f.fail[u] {
		return nil, &StatusError{Method: "GET", Code: 503, URL: u}
	}
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *levtechFake) details() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if strings.Contains(u, "/project/detail/") {
			n++
		}
	}
	return n
}

func newLevtechFake() *levtechFake {
	return &levtechFake{
		fail: map[string]bool{},
		pages: map[string]string{
			levtechPageURL(1): levtechListPage(
				levtechProj{ID: 101, Name: "【AWS】AWS環境構築", Remote: 1, Station: "名古屋", Pref: "愛知県"},
				levtechProj{ID: 102, Name: "【Java】基幹システム開発", Remote: 2, Station: "渋谷", Pref: "東京都"}),
			levtechPageURL(2): levtechListPage(
				levtechProj{ID: 102, Name: "【Java】基幹システム開発", Remote: 2},
				levtechProj{ID: 103, Name: "【UI】UIデザイン", Remote: 999999999, Contract: "temporary", Station: "新宿", Pref: "東京都"},
				levtechProj{ID: 900, Name: "closed", Closed: true}),
			levtechPageURL(3):       levtechListPage(levtechProj{ID: 901, Name: "closed", Closed: true}),
			levtechDetailURL("101"): levtechDetailPage(101, "MONTH", "770000", "名古屋市中村区"),
			levtechDetailURL("102"): levtechDetailPage(102, "MONTH", "900000", "渋谷区"),
			levtechDetailURL("103"): levtechDetailPage(103, "HOUR", "2800", "新宿区"),
		},
	}
}

func levtechByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestLevtechWalksOpenPagesAndMapsFields(t *testing.T) {
	f := newLevtechFake()
	jobs, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	got := levtechByID(jobs)
	if len(jobs) != 3 || len(got) != 3 {
		t.Fatalf("jobs = %d, want 3 (duplicate 102 collapsed, closed projects dropped): %+v", len(jobs), jobs)
	}
	a := got["101"]
	if a.Title != "【AWS】AWS環境構築" || a.Company != levtechCompany || a.URL != levtechDetailURL("101") {
		t.Errorf("101 identity = %+v", a)
	}
	if !a.Remote || a.WorkMode != "remote" || a.EmploymentType != "contract" || !a.IsTechHint {
		t.Errorf("101 facets: remote=%v mode=%q type=%q tech=%v", a.Remote, a.WorkMode, a.EmploymentType, a.IsTechHint)
	}
	if a.Location != "名古屋市中村区, 愛知県" || len(a.Countries) != 1 || a.Countries[0] != "jp" {
		t.Errorf("101 place = %q %v", a.Location, a.Countries)
	}
	if !strings.Contains(a.Description, "AWS環境の構築") {
		t.Errorf("101 body = %q", a.Description)
	}
	if a.SalaryMax == nil || *a.SalaryMax != 770000 || a.SalaryMin != nil || a.SalaryCurrency != "JPY" || a.SalaryPeriod != "month" {
		t.Errorf("101 salary = %v-%v %q %q", a.SalaryMin, a.SalaryMax, a.SalaryCurrency, a.SalaryPeriod)
	}
	if b := got["102"]; b.WorkMode != "hybrid" || b.Remote {
		t.Errorf("102 mode = %q remote=%v, want hybrid", b.WorkMode, b.Remote)
	}
	c := got["103"]
	if c.WorkMode != "" || c.EmploymentType != "" {
		t.Errorf("103 stated no arrangement and is a dispatch project: mode=%q type=%q", c.WorkMode, c.EmploymentType)
	}
	if c.SalaryMax == nil || *c.SalaryMax != 2800 || c.SalaryPeriod != "hour" {
		t.Errorf("103 salary = %v %q, want 2800 per hour", c.SalaryMax, c.SalaryPeriod)
	}
}

func TestLevtechSeenPostingCostsNoDetailRequest(t *testing.T) {
	f := newLevtechFake()
	seen := func(id string) bool { return id != "103" }
	jobs, err := NewLevtech(f).(levtech).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if n := f.details(); n != 1 {
		t.Errorf("detail requests = %d, want 1 (only the unseen posting)", n)
	}
	got := levtechByID(jobs)
	if !got["101"].SeenRefresh || !got["102"].SeenRefresh || got["103"].SeenRefresh || got["103"].Description == "" {
		t.Errorf("refresh flags wrong: %+v", got)
	}
}

func TestLevtechDetailGoneIsDroppedAndFailureDefers(t *testing.T) {
	f := newLevtechFake()
	delete(f.pages, levtechDetailURL("102")) // 404: the platform says it is gone
	f.fail[levtechDetailURL("103")] = true   // 503: unreadable, not gone
	jobs, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "101" {
		t.Errorf("jobs = %+v, want only 101", jobs)
	}
}

func TestLevtechAllDetailsFailingFailsTheCrawl(t *testing.T) {
	f := newLevtechFake()
	for _, id := range []string{"101", "102", "103"} {
		f.fail[levtechDetailURL(id)] = true
	}
	if _, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl that listed postings and read none of them must fail")
	}
}

func TestLevtechDetailBudgetBoundsARun(t *testing.T) {
	f := &levtechFake{fail: map[string]bool{}, pages: map[string]string{}}
	var projs []levtechProj
	for i := 1; i <= levtechDetailBudget+5; i++ {
		projs = append(projs, levtechProj{ID: i, Name: "p"})
		f.pages[levtechDetailURL(fmt.Sprint(i))] = levtechDetailPage(i, "MONTH", "700000", "渋谷区")
	}
	f.pages[levtechPageURL(1)] = levtechListPage(projs...)
	f.pages[levtechPageURL(2)] = levtechListPage(levtechProj{ID: 9999, Closed: true})
	jobs, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.details(); n != levtechDetailBudget || len(jobs) != levtechDetailBudget {
		t.Errorf("detail requests = %d, jobs = %d, want %d each", n, len(jobs), levtechDetailBudget)
	}
}

func TestLevtechListingFailuresFailTheCrawl(t *testing.T) {
	f := newLevtechFake()
	f.fail[levtechPageURL(2)] = true
	if _, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a failing later page must fail the crawl, not end it short")
	}

	f = newLevtechFake()
	f.pages[levtechPageURL(1)] = `<html><body>challenge</body></html>`
	if _, err := NewLevtech(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a page with no payload is not a results page")
	}
}

func TestLevtechIsBoardlessAggregator(t *testing.T) {
	s := NewLevtech(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("levtech must be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("levtech must be an aggregator")
	}
}
