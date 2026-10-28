package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// computrabajoFake serves the real fixtures (fetched 2026-10-28 from ar.computrabajo.com): the
// location filter trimmed to La Rioja (19 postings) and Tierra del Fuego, La Rioja's first page
// trimmed to four cards — Global Medica (hybrid, a salary), SSERRANO RRHH (a salary), Grupo Ceta
// and one hidden-employer card — the empty page the site answers past a listing's end, and the
// posting pages of the first two. Any other posting answers 404, as a gone one does.
type computrabajoFake struct {
	filterErr error

	mu   sync.Mutex
	urls []string
}

const (
	ctGlobalMedica = "092F54B5435B3F1A61373E686DCF3405"
	ctSserrano     = "DCEB98C7224FBA4861373E686DCF3405"
)

func (f *computrabajoFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.mu.Unlock()
	base := "https://ar.computrabajo.com"
	var path string
	switch {
	case u == base+"/ofertas-de-trabajo/?p=2":
		if f.filterErr != nil {
			return nil, f.filterErr
		}
		path = "testdata/computrabajo/provinces.html"
	case u == base+"/empleos-en-la-rioja":
		path = "testdata/computrabajo/province.html"
	case strings.HasPrefix(u, base+"/empleos-en-"):
		path = "testdata/computrabajo/empty.html"
	case strings.HasSuffix(u, ctGlobalMedica), strings.HasSuffix(u, ctSserrano):
		path = "testdata/computrabajo/posting_" + u[len(u)-32:] + ".html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *computrabajoFake) postingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, "/oferta-de-trabajo-de-") {
			n++
		}
	}
	return n
}

func newTestComputrabajo(f *computrabajoFake) computrabajo {
	s := NewComputrabajo(f).(computrabajo)
	s.limiter = noWait{}
	return s
}

var ctEntry = CompanyEntry{Provider: "computrabajo", Board: "ar"}

func TestComputrabajoHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &computrabajoFake{}
	jobs, err := newTestComputrabajo(f).FetchNew(context.Background(), ctEntry, func(id string) bool {
		return id == "70E96A343FE3F74B61373E686DCF3405"
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want two hydrated and one refreshed (the hidden employer dropped): %+v", len(jobs), jobs)
	}
	g := byID[ctGlobalMedica]
	if g.Title != "responsable de compras" || g.Company != "Global Medica" || g.Location != "La rioja, La Rioja" {
		t.Errorf("title/company/place = %q / %q / %q", g.Title, g.Company, g.Location)
	}
	if !strings.Contains(g.Description, "Global Médica S.A.") || !strings.Contains(g.Description, "Requerimientos") {
		t.Errorf("description = %.200q", g.Description)
	}
	if strings.Contains(g.Description, "Aptitudes") || strings.Contains(g.Description, "Palabras clave") || strings.Contains(g.Description, "Contrato por tiempo indeterminado") {
		t.Error("the skill chips or the fact tags leaked into the description")
	}
	if g.WorkMode != "hybrid" || g.EmploymentType != "full_time" {
		t.Errorf("work mode / employment = %q / %q", g.WorkMode, g.EmploymentType)
	}
	if g.SalaryMin == nil || *g.SalaryMin != 2500000 || *g.SalaryMax != 2500000 || g.SalaryCurrency != "ARS" || g.SalaryPeriod != "month" {
		t.Errorf("salary = %v %q %q", g.SalaryMin, g.SalaryCurrency, g.SalaryPeriod)
	}
	if g.ExperienceYearsMin == nil || *g.ExperienceYearsMin != 3 {
		t.Errorf("experience = %v, want 3 years", g.ExperienceYearsMin)
	}
	if len(g.Countries) != 1 || g.Countries[0] != "ar" || g.PostedAt == nil || g.PostedAt.Format("2006-01-02") != "2026-09-23" {
		t.Errorf("countries = %v posted = %v", g.Countries, g.PostedAt)
	}
	if r, ok := byID["70E96A343FE3F74B61373E686DCF3405"]; !ok || !r.SeenRefresh || r.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	if s := byID[ctSserrano]; s.Description == "" || s.Company != "SSERRANO RRHH" {
		t.Errorf("second posting = %+v", s)
	}
	if n := f.postingRequests(); n != 2 {
		t.Errorf("posting requests = %d, want only the two unseen named postings", n)
	}
}

func TestComputrabajoGateSkipsCoveredPostingsWithoutARequest(t *testing.T) {
	f := &computrabajoFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[c] = true
		}
		return m
	}}
	jobs, err := newTestComputrabajo(f).FetchNewGated(context.Background(), ctEntry, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if n := f.postingRequests(); n != 0 {
		t.Errorf("bought %d pages of covered postings", n)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d body-less jobs, want the three named postings", len(jobs))
	}
}

func TestComputrabajoBudgetAndBoard(t *testing.T) {
	f := &computrabajoFake{}
	s := newTestComputrabajo(f)
	s.maxNewPerRun = 0
	jobs, err := s.FetchNew(context.Background(), ctEntry, func(string) bool { return false })
	if err != nil || len(jobs) != 0 || f.postingRequests() != 0 {
		t.Errorf("a zero budget: %d jobs, %d requests, err %v", len(jobs), f.postingRequests(), err)
	}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Board: "argentina"}, func(string) bool { return false }); err == nil {
		t.Error("a board that is not a country subdomain was accepted")
	}
	f.filterErr = errors.New("403")
	if _, err := s.FetchNew(context.Background(), ctEntry, func(string) bool { return false }); err == nil {
		t.Error("an unreadable location filter did not fail the crawl")
	}
}

func TestComputrabajoWalksEachProvinceToItsEnd(t *testing.T) {
	f := &computrabajoFake{}
	if _, err := newTestComputrabajo(f).FetchNew(context.Background(), ctEntry, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://ar.computrabajo.com/ofertas-de-trabajo/?p=2",
		"https://ar.computrabajo.com/empleos-en-la-rioja",
		"https://ar.computrabajo.com/empleos-en-la-rioja?p=2",
		"https://ar.computrabajo.com/empleos-en-tierra-del-fuego",
	}
	if strings.Join(f.urls, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(f.urls, "\n"), strings.Join(want, "\n"))
	}
}

func TestComputrabajoSalary(t *testing.T) {
	for in, want := range map[string]int{
		"$ 2.500.000,00 (Mensual)":            2500000,
		"$ 1.500.000,00 (Mensual) + Comisión": 1500000,
		"$ 2.000,00 (Mensual)":                0,
		"$ 999.999,00 (Mensual)":              0,
		"$ 1.000.000,00 (Quincenal)":          0,
		"A convenir":                          0,
	} {
		got, ok := computrabajoSalary(in)
		if got != want || ok != (want > 0) {
			t.Errorf("computrabajoSalary(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
}
