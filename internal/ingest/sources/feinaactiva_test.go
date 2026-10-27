package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// feinaactivaFake serves the real listing page (fetched 2026-10-27) trimmed to three offers — a
// named employer, a hidden one and a relayed CIDO call — and answers every later page empty, as
// the site does past its end.
type feinaactivaFake struct {
	err   error
	pages []string
}

func (f *feinaactivaFake) GetJSON(_ context.Context, u string, v any) error {
	f.pages = append(f.pages, u)
	if u != fmt.Sprintf(feinaactivaListURL, 1) {
		return json.Unmarshal([]byte(`{"total":3,"included":[]}`), v)
	}
	if f.err != nil {
		return f.err
	}
	raw, err := os.ReadFile("testdata/feinaactiva/page1.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func newTestFeinaActiva(f *feinaactivaFake) feinaactiva {
	s := NewFeinaActiva(f).(feinaactiva)
	s.limiter = noWait{}
	return s
}

func TestFeinaActivaMapsOffers(t *testing.T) {
	f := &feinaactivaFake{}
	jobs, err := newTestFeinaActiva(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	// The hidden employer's offer is dropped; the total is reached on page 1, so no page 2.
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want two", jobs)
	}
	if len(f.pages) != 1 {
		t.Errorf("pages read = %v, want only page 1", f.pages)
	}
	j := jobs[0]
	if j.ExternalID != "FA92325157" || j.URL != feinaactivaBaseURL+"/search/offers/detail/FA92325157" {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Title != "FISIOTERAPEUTA" || j.Company != "RESIDENCIA EIXAMPLE EL PRAT, S.L." {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "El Prat de Llobregat, Barcelona" {
		t.Errorf("location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "es" {
		t.Errorf("countries = %v", j.Countries)
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 850 || *j.SalaryMax != 900 || j.SalaryPeriod != "month" || j.SalaryCurrency != "EUR" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !strings.Contains(j.Description, "Sesiones individuales") || !strings.Contains(j.Description, "Requisits") {
		t.Errorf("description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-09-21" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	cido := jobs[1]
	if strings.HasSuffix(cido.Title, "CIDO") {
		t.Errorf("CIDO suffix kept: %q", cido.Title)
	}
	if strings.Contains(cido.Description, "Veure convocatòria") {
		t.Errorf("the empty duties line leaked into the body: %q", cido.Description)
	}
	if cido.Location != "Barcelona" {
		t.Errorf("cido location = %q", cido.Location)
	}
}

func TestFeinaActivaFirstPageFailingFailsTheCrawl(t *testing.T) {
	f := &feinaactivaFake{err: errors.New("boom")}
	if _, err := newTestFeinaActiva(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestFeinaActivaPlace(t *testing.T) {
	cases := map[string]string{
		"GIRONA - BAIX EMPORDÀ - 17100 BISBAL D'EMPORDÀ, LA": "La Bisbal d'Empordà, Girona",
		"BARCELONA - VALLÈS OCCIDENTAL - 8201 SABADELL":      "Sabadell, Barcelona",
		"LLEIDA - SEGRIÀ - 25003 LLEIDA":                     "Lleida",
		"BARCELONA - VALLÈS ORIENTAL - 08160 MONTMELÓ":       "Montmeló, Barcelona",
	}
	for in, want := range cases {
		if got, ok := feinaactivaPlace(in); !ok || got != want {
			t.Errorf("place(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := feinaactivaPlace("CATALUNYA"); ok {
		t.Error("read a place not in the API's shape")
	}
}

func TestFeinaActivaHiddenEmployer(t *testing.T) {
	hidden := []string{"Empresa de serveis de neteja", "Empresa del sector de les TIC", "Empresa pública", "Oficina de Treball del SOC"}
	for _, n := range hidden {
		if !feinaactivaHiddenEmployer(n) {
			t.Errorf("%q read as a company", n)
		}
	}
	named := []string{"RESIDENCIA EIXAMPLE EL PRAT, S.L.", "Empresa Municipal d'Aigües, S.A.", "PASSO DE CUINAR S.L.", "Ajuntament de Sabadell"}
	for _, n := range named {
		if feinaactivaHiddenEmployer(n) {
			t.Errorf("%q read as hidden", n)
		}
	}
}

func TestFeinaActivaSalary(t *testing.T) {
	lo, hi, period, ok := feinaactivaSalary("Salari mensual brut 1695.43")
	if !ok || lo != 1695 || hi != 1695 || period != "month" {
		t.Errorf("single figure = %d-%d %s %v", lo, hi, period, ok)
	}
	if _, _, _, ok := feinaactivaSalary("Jornada completa"); ok {
		t.Error("read a salary from a non-salary condition")
	}
}
