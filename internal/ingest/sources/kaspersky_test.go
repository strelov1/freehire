package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// kasperskyPage wraps a listing payload in the RSC flight push a careers.kaspersky.ru page
// carries, behind the decoy the real page also carries: its i18n messages hold a "vacancies"
// key whose value is a string, so a reader keyed on the bare field name finds that first.
func kasperskyPage(vacancies string, total int) string {
	flight := `10:["$","$L11",null,{"messages":{"Breadcrumbs":{"vacancies":"Вакансии"}}}]` + "\n" +
		`6:[["$","$L42",null,{"vacancies":` + vacancies + `,"totalCount":` + strconv.Itoa(total) + `}]]` + "\n"
	q, _ := json.Marshal(flight)
	return `<html><body><script>self.__next_f.push([1,` + string(q) + `])</script></body></html>`
}

// kasperskyVacancy is one listing item in the site's own shape.
func kasperskyVacancy(id, title string) string {
	return `{"jobReqId":"` + id + `","titles":[{"title":"` + title + `","locale":"en_GB"}],` +
		`"skills":[],"cities":[{"code":"25373","name":"Москва","engName":"Moscow"}],"categories":[]}`
}

func kasperskyEntry() CompanyEntry { return CompanyEntry{Company: "Kaspersky", Provider: "kaspersky"} }

func TestKasperskyProviderIsBoardless(t *testing.T) {
	s := NewKaspersky(nil)
	if got := s.Provider(); got != "kaspersky" {
		t.Errorf("Provider() = %q, want kaspersky", got)
	}
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("kaspersky must be boardless: it is one employer's own careers site")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("kaspersky must hydrate: the listing carries no description")
	}
}

func TestKasperskyFetchNewMapsListingAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/vacancies?page=0", deelFixture(t, "kaspersky_list.html")).
		route("/vacancies?page=1", kasperskyPage(`[]`, 85)).
		route("/vacancy/25969", deelFixture(t, "kaspersky_vacancy.html")).
		routeRedirect("/vacancy/25358", "https://careers.kaspersky.ru/vacancies").
		routeErr("/vacancy/668bbf4ab63848f9dd858476", &StatusError{Code: http.StatusBadGateway})

	jobs, err := NewKaspersky(fake).(HydratingSource).FetchNew(context.Background(), kasperskyEntry(),
		func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["25358"]; ok {
		t.Error("a posting whose page redirects to the listing is gone and must be dropped")
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (25969 hydrated, the hex id list-only)", len(jobs))
	}

	j := byID["25969"]
	if j.Title != "Senior developer Go (OSMP)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://careers.kaspersky.ru/vacancy/25969" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Kaspersky" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Russia, Moscow" {
		t.Errorf("Location = %q, want the cities' English names", j.Location)
	}
	if !slices.Contains(j.Skills, "go") {
		t.Errorf("Skills = %v, want go", j.Skills)
	}
	if j.Category != "" {
		t.Errorf("Category = %q, want none (a language category names no discipline)", j.Category)
	}
	for _, want := range []string{"Платформа OSMP", "Чем предстоит заниматься", "Опыт работы с Docker/K8s."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	if strings.Contains(j.Description, "Мы предлагаем") {
		t.Error("the site-wide benefits block is boilerplate and must stay out of the body")
	}

	sec := byID["668bbf4ab63848f9dd858476"]
	if sec.Category != "security" {
		t.Errorf("Category = %q, want security", sec.Category)
	}
	if sec.Description != "" || sec.Title != "Security Researcher (GReAT)" {
		t.Errorf("list-only fallback = %+v", sec)
	}
}

func TestKasperskyWalksPagesUntilTotal(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/vacancies?page=0", kasperskyPage(`[`+kasperskyVacancy("1", "A")+`,`+kasperskyVacancy("2", "B")+`]`, 3)).
		route("/vacancies?page=1", kasperskyPage(`[`+kasperskyVacancy("3", "C")+`]`, 3)).
		routeErr("/vacancies?page=2", &StatusError{Code: http.StatusInternalServerError})
	jobs, err := NewKaspersky(fake).Fetch(context.Background(), kasperskyEntry())
	if err != nil {
		t.Fatalf("Fetch: %v (the walk must stop at totalCount, not ask for page 2)", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
}

func TestKasperskyFirstPageFailureFailsLaterPageEndsWalk(t *testing.T) {
	first := (&routedHTTP{}).routeErr("/vacancies?page=0", &StatusError{Code: http.StatusForbidden})
	if _, err := NewKaspersky(first).Fetch(context.Background(), kasperskyEntry()); err == nil {
		t.Error("Fetch: want an error when the first listing page fails")
	}

	later := (&routedHTTP{}).
		route("/vacancies?page=0", kasperskyPage(`[`+kasperskyVacancy("1", "A")+`]`, 5)).
		routeErr("/vacancies?page=1", &StatusError{Code: http.StatusBadGateway})
	jobs, err := NewKaspersky(later).Fetch(context.Background(), kasperskyEntry())
	if err != nil || len(jobs) != 1 {
		t.Errorf("Fetch = %d jobs, %v; want the first page's posting and no error", len(jobs), err)
	}
}

func TestKasperskyListingWithoutPayloadIsAnError(t *testing.T) {
	fake := (&routedHTTP{}).route("/vacancies?page=0", `<html><body>maintenance</body></html>`)
	if _, err := NewKaspersky(fake).Fetch(context.Background(), kasperskyEntry()); err == nil {
		t.Fatal("Fetch: a page without the listing payload must fail loudly, not read as empty")
	}
}

func TestKasperskyFetchNewSeenSkipsDetail(t *testing.T) {
	fake := (&routedHTTP{}).route("/vacancies?page=0", deelFixture(t, "kaspersky_list.html")).
		route("/vacancies?page=1", kasperskyPage(`[]`, 85))
	jobs, err := NewKaspersky(fake).(HydratingSource).FetchNew(context.Background(), kasperskyEntry(),
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("%s: SeenRefresh = false, want a liveness-only refresh", j.ExternalID)
		}
	}
}

func TestKasperskyAllDetailsFailingIsAnError(t *testing.T) {
	fake := (&routedHTTP{}).route("/vacancies?page=0", deelFixture(t, "kaspersky_list.html")).
		route("/vacancies?page=1", kasperskyPage(`[]`, 85)).
		routeErr("/vacancy/", &StatusError{Code: http.StatusForbidden})
	_, err := NewKaspersky(fake).(HydratingSource).FetchNew(context.Background(), kasperskyEntry(),
		func(string) bool { return false })
	if err == nil {
		t.Fatal("FetchNew: want an error when every listed posting's detail failed")
	}
}

func TestKasperskyCategory(t *testing.T) {
	cat := func(names ...string) []kasperskyNamed {
		var out []kasperskyNamed
		for _, n := range names {
			out = append(out, kasperskyNamed{EngName: n})
		}
		return out
	}
	cases := []struct {
		in   []kasperskyNamed
		want string
	}{
		{cat("Information Security"), "security"},
		{cat("Testing Automation | SDET"), "qa"},
		{cat("DevOps"), "devops"},
		{cat("Development GO"), ""},
		{cat("Finance", "Legal"), ""}, // two disciplines: the source names no single one
		{cat("Testing Manual", "Testing Automation | SDET"), "qa"},
		{cat("Internship", "DevOps"), "devops"},
	}
	for _, c := range cases {
		if got := kasperskyCategory(c.in); got != c.want {
			t.Errorf("kasperskyCategory(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	if !kasperskyIsInternship(cat("Internship")) || kasperskyIsInternship(cat("DevOps")) {
		t.Error("kasperskyIsInternship must read the Internship category only")
	}
}
