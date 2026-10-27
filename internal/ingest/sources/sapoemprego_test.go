package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// sapoempregoFake serves the real fixtures (fetched 2026-10-27): the offers sitemap trimmed to a
// handful of entries and three posting pages trimmed to their JobPosting and summary list. The
// warehouse posting answers the site's search page (HTTP 200, no JobPosting), as an id the site
// no longer holds does; any other posting answers 404.
type sapoempregoFake struct {
	pageErr error

	mu    sync.Mutex
	pages []string
}

func (f *sapoempregoFake) GetXML(_ context.Context, u string, v any) error {
	if u != sapoempregoSitemap {
		return fmt.Errorf("sapoempregoFake: no sitemap %s", u)
	}
	raw, err := os.ReadFile("testdata/sapoemprego/offers.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *sapoempregoFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	files := map[string]string{
		"7bc821bd-0b4d-4149-a5cb-4235e7c729f7": "testdata/sapoemprego/hybrid.html",
		"4950592a-16be-4e11-9411-f9a18175a027": "testdata/sapoemprego/remote.html",
		"d14d1549-0494-477a-9ed3-87cb1c601f1c": "testdata/sapoemprego/hidden.html",
	}
	for id, path := range files {
		if strings.HasSuffix(u, "?id="+id) {
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return html.Parse(strings.NewReader(string(raw)))
		}
	}
	if strings.Contains(u, "operador-de-armazem") {
		return html.Parse(strings.NewReader(`<html><head><title>Ofertas de Emprego para: operador-de-armazem</title></head><body></body></html>`))
	}
	return nil, &StatusError{Method: "GET", Code: 404, URL: u}
}

func newTestSapoEmprego(f *sapoempregoFake) sapoemprego {
	s := NewSapoEmprego(f).(sapoemprego)
	s.limiter = noWait{}
	return s
}

func TestSapoEmpregoHydratesNewPostings(t *testing.T) {
	f := &sapoempregoFake{}
	jobs, err := newTestSapoEmprego(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool {
		return id == "2a6451d8-f466-4aac-a507-994b181f1b80"
	})
	if err != nil {
		t.Fatal(err)
	}
	// The four unseen postings are read; the repeated entry and the course are not postings.
	if len(f.pages) != 4 {
		t.Errorf("read %d pages, want 4: %v", len(f.pages), f.pages)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The stored posting is refreshed; the hidden employer and the two gone postings are not
	// yielded.
	if len(jobs) != 3 || !byID["2a6451d8-f466-4aac-a507-994b181f1b80"].SeenRefresh {
		t.Fatalf("jobs = %+v", jobs)
	}

	h := byID["7bc821bd-0b4d-4149-a5cb-4235e7c729f7"]
	if h.Title != "Consultor(a) ERP Primavera Júnior" || h.Company != "Basilaris" {
		t.Errorf("title/company = %q / %q", h.Title, h.Company)
	}
	if h.Location != "Castelo Branco" || !slices.Equal(h.Countries, []string{"pt"}) || !slices.Equal(h.FallbackCountries, []string{"pt"}) {
		t.Errorf("location %q, countries %v, fallback %v", h.Location, h.Countries, h.FallbackCountries)
	}
	if h.WorkMode != "hybrid" || h.Remote || h.EmploymentType != "full_time" || h.IsTechHint {
		t.Errorf("work mode %q remote %v employment %q tech %v", h.WorkMode, h.Remote, h.EmploymentType, h.IsTechHint)
	}
	if h.SalaryMin == nil || *h.SalaryMin != 15000 || *h.SalaryMax != 25000 || h.SalaryCurrency != "EUR" || h.SalaryPeriod != "year" {
		t.Errorf("salary %v-%v %s/%s", h.SalaryMin, h.SalaryMax, h.SalaryCurrency, h.SalaryPeriod)
	}
	if h.PostedAt == nil || h.PostedAt.Format("2006-01-02") != "2026-10-27" || !strings.Contains(h.Description, "ERP Primavera") {
		t.Errorf("posted %v, description %.80q", h.PostedAt, h.Description)
	}

	r := byID["4950592a-16be-4e11-9411-f9a18175a027"]
	if r.WorkMode != "remote" || !r.Remote || r.Location != "" || !slices.Equal(r.Countries, []string{"pt"}) {
		t.Errorf("remote posting: work mode %q remote %v location %q countries %v", r.WorkMode, r.Remote, r.Location, r.Countries)
	}
	if r.SalaryMin != nil {
		t.Errorf("an undefined salary (A definir) got one: %v", *r.SalaryMin)
	}
}

func TestSapoEmpregoTechIndustrySetsTheHint(t *testing.T) {
	ld := sapoempregoLD{Title: "Programador Java", Description: "<p>Java</p>", Industry: "Informática e Tecnologias"}
	ld.HiringOrganization.Name = "Acme"
	j, ok := sapoempregoJob(ld, sapoempregoSummary{workhome: "Presencial"}, sapoempregoPosting{id: "x"})
	if !ok || !j.IsTechHint || j.WorkMode != "onsite" || j.EmploymentType != "" {
		t.Errorf("job = %+v, ok %v", j, ok)
	}
}

// Postings arrive most recently touched first, so the fresh lane reads what the site touched
// since the last run before any backlog.
func TestSapoEmpregoListsMostRecentFirst(t *testing.T) {
	ps, err := newTestSapoEmprego(&sapoempregoFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.id[:8])
	}
	want := []string{"4950592a", "7bc821bd", "d14d1549", "0b6f1d7e", "2a6451d8"}
	if !slices.Equal(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}
}

// A run that reached no posting page at all is broken, not quiet.
func TestSapoEmpregoFailsWhenNoPageAnswers(t *testing.T) {
	f := &sapoempregoFake{pageErr: errors.New("connection reset")}
	if _, err := newTestSapoEmprego(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when every posting page failed")
	}
}

// Near the run's deadline hydration stops and the run returns what it has, so the pipeline still
// writes it; the pages left unread are not a failure.
func TestSapoEmpregoStopsHydratingNearTheDeadline(t *testing.T) {
	f := &sapoempregoFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestSapoEmprego(f).FetchNew(ctx, CompanyEntry{}, func(id string) bool {
		return id == "2a6451d8-f466-4aac-a507-994b181f1b80"
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 || len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Errorf("read %d pages, jobs %+v; want only the refresh", len(f.pages), jobs)
	}
}
