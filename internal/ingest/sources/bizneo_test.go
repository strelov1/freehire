package sources

import (
	"context"
	"os"
	"strings"
	"testing"
)

// bizneoFixture reads a page captured verbatim from grupo-euroformac.careers.ats.bizneo.cloud
// (2026-10-27: 19 postings over two listing pages, and an empty third) or, for the JobPosting
// variant, from somos-grupo-mas. The S3 signatures of the image URLs are redacted.
func bizneoFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/bizneo/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const bizneoEuroformac = "grupo-euroformac.careers.ats.bizneo.cloud"

func bizneoEuroformacHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, slug := range []string{
		"bolsa-de-trabajo-permanente-docentes-fp-formacion-profesional",
		"bolsa-de-empleo-permanente-docentes-toda-espana-f4b00531-ba57-4b9f-bc37-ec1278a5010a",
	} {
		r.route(bizneoEuroformac+"/jobs/"+slug, bizneoFixture(t, "job_"+slug+".html"))
	}
	return r.route("/jobs?page=1", bizneoFixture(t, "list_p1.html")).
		route("/jobs?page=2", bizneoFixture(t, "list_p2.html")).
		route("/jobs?page=3", bizneoFixture(t, "list_p3.html"))
}

var bizneoEuroformacEntry = CompanyEntry{Company: "Grupo Euroformac", Provider: "bizneo", Board: bizneoEuroformac}

func TestBizneoProvider(t *testing.T) {
	s, ok := All(nil)["bizneo"]
	if !ok || s.Provider() != "bizneo" {
		t.Fatal("All() must register bizneo")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("bizneo must be a HydratingSource: the body costs a page request")
	}
}

func TestBizneoListsEveryPageAndRefreshesSeenPostings(t *testing.T) {
	fake := bizneoEuroformacHTTP(t)
	jobs, err := NewBizneo(fake).(HydratingSource).FetchNew(context.Background(), bizneoEuroformacEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// The site states "Tenemos 19 ofertas"; the walk stops at the empty third page.
	if len(jobs) != 19 || fake.calls != 3 {
		t.Fatalf("jobs/requests = %d/%d, want the 19 cards from three listing pages", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Title == "" || j.URL != "https://"+bizneoEuroformac+"/jobs/"+j.ExternalID {
			t.Errorf("job = %+v, want an identity-only SeenRefresh with its card title", j)
		}
	}
}

func TestBizneoMapsAPageWithoutJobPosting(t *testing.T) {
	fake := bizneoEuroformacHTTP(t)
	slug := "bolsa-de-trabajo-permanente-docentes-fp-formacion-profesional"
	jobs, err := NewBizneo(fake).(HydratingSource).FetchNew(context.Background(), bizneoEuroformacEntry,
		func(id string) bool { return id != slug })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var g Job
	for _, j := range jobs {
		if j.ExternalID == slug {
			g = j
		}
	}
	if g.SeenRefresh || g.Unreadable {
		t.Fatalf("job = %+v, want it hydrated", g)
	}
	if g.Title != "BOLSA DE TRABAJO PERMANENTE – DOCENTES FP (FORMACIÓN PROFESIONAL)" || g.Company != "Grupo Euroformac" {
		t.Errorf("Title/Company = %q/%q", g.Title, g.Company)
	}
	if g.Location != "Madrid" || g.WorkMode != "remote" || !g.Remote {
		t.Errorf("Location/WorkMode = %q/%q, want the side card's Madrid and Teletrabajo/Remoto", g.Location, g.WorkMode)
	}
	if !strings.Contains(g.Description, "Desarrollo de Aplicaciones Web") || strings.Contains(g.Description, "Aplica ahora") {
		t.Errorf("Description = %.200q", g.Description)
	}
	if g.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil: this template states no date", g.PostedAt)
	}
}

func TestBizneoReadsTheJobPostingWhenPresent(t *testing.T) {
	const host = "somos-grupo-mas.careers.ats.bizneo.cloud"
	slug := "auxiliar-de-compras-561c3b68-504a-4a97-b220-020be92f6059"
	fake := (&routedHTTP{}).route(host+"/jobs/"+slug, bizneoFixture(t, "job_mas_auxiliar-de-compras.html"))
	j, ok := NewBizneo(fake).(bizneo).detail(context.Background(), CompanyEntry{Company: "Somos Grupo MAS", Board: host},
		bizneoCard{slug: slug, url: "https://" + host + "/jobs/" + slug, title: "AUXILIAR DE COMPRAS", mode: "onsite"})
	if !ok || j.Unreadable {
		t.Fatalf("detail = %+v, %v", j, ok)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("PostedAt = %v, want the JobPosting's 2026-09-01", j.PostedAt)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "es" || j.EmploymentType != "full_time" {
		t.Errorf("Countries/EmploymentType = %v/%q", j.Countries, j.EmploymentType)
	}
	if j.WorkMode != "onsite" || !strings.Contains(j.Description, "Aprovisionamiento") {
		t.Errorf("WorkMode = %q, Description = %.120q", j.WorkMode, j.Description)
	}
}

func TestBizneoRefusesAForeignBoard(t *testing.T) {
	for _, board := range []string{"acme", "groundforce.bizneo.com", "evil.com/x.bizneo.cloud"} {
		if _, err := NewBizneo(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q: want an error", board)
		}
	}
}
