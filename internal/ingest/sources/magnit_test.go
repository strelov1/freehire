package sources

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestMagnitProviderIsBoardless(t *testing.T) {
	s := NewMagnit(nil)
	if got := s.Provider(); got != "magnit" {
		t.Errorf("Provider() = %q, want magnit", got)
	}
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("magnit must be boardless: it is one employer's own careers API")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("magnit must hydrate: the listing carries no description")
	}
}

func TestMagnitFetchNewMapsListingAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/vacancy?", deelFixture(t, "magnit_list.json")).
		route("/api/v1/vacancy/2886", deelFixture(t, "magnit_vacancy_2886.json")).
		routeErr("/api/v1/vacancy/2921", &StatusError{Method: "GET", Code: http.StatusNotFound}).
		routeErr("/api/v1/vacancy/3085", &StatusError{Method: "GET", Code: http.StatusForbidden})

	jobs, err := NewMagnit(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Magnit Tech", Provider: "magnit"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["2921"]; ok {
		t.Error("a posting the API answers 404 for is gone and must be dropped")
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (2886 hydrated, 3085 list-only)", len(jobs))
	}

	j := byID["2886"]
	if j.Title != "Разработчик Fullstack Senior (Python, Vue.js)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://magnit.tech/vacancies/2886" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Magnit Tech" {
		t.Errorf("Company = %q", j.Company)
	}
	if !strings.Contains(j.Location, "Россия") || !strings.Contains(j.Location, "Москва и Краснодар") {
		t.Errorf("Location = %q, want the stated location and the work-format places", j.Location)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (other cities work remotely)", j.WorkMode)
	}
	if j.Category != "fullstack" {
		t.Errorf("Category = %q, want fullstack", j.Category)
	}
	if !slices.Contains(j.Skills, "python") || !slices.Contains(j.Skills, "vue") {
		t.Errorf("Skills = %v, want python and vue", j.Skills)
	}
	if !j.IsTechHint {
		t.Error("IsTechHint must be set: magnit.tech is the IT arm's own careers site")
	}
	for _, want := range []string{"автоматизации процесса качества", "<h3>Задачи</h3>", "<h3>Требования</h3>", "<h3>Мы предлагаем</h3>", "ДМС"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	if strings.Contains(j.Description, "<h3>Будет плюсом</h3>") {
		t.Error("an empty section must not leave its heading behind")
	}

	// A detail that failed without saying the posting is gone keeps the listing's fields.
	lo := byID["3085"]
	if lo.Title != "Главный разработчик 1С" || lo.Description != "" || lo.WorkMode != "remote" {
		t.Errorf("list-only fallback = %+v", lo)
	}
}

func TestMagnitFetchNewSeenSkipsDetail(t *testing.T) {
	fake := (&routedHTTP{}).route("/api/v1/vacancy?", deelFixture(t, "magnit_list.json"))
	jobs, err := NewMagnit(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Magnit Tech"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	// No detail route exists, so a detail request would surface as a list-only job without
	// SeenRefresh rather than as the liveness-only refresh asserted here.
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("%s: SeenRefresh = false, want a liveness-only refresh", j.ExternalID)
		}
	}
}

func TestMagnitDetailClosedIsDropped(t *testing.T) {
	list := `{"success":true,"meta":{"total":1,"has_more_pages":false},"results":[{"id":7,"title":"QA","work_formats":[]}]}`
	detail := `{"success":true,"results":{"id":7,"title":"QA","description":"<p>x</p>","status":"archived","is_published":true}}`
	fake := (&routedHTTP{}).route("/api/v1/vacancy?", list).route("/api/v1/vacancy/7", detail)
	jobs, err := NewMagnit(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs = %+v, want none (the detail reports the posting closed)", jobs)
	}
}

func TestMagnitPaginatesUntilNoMorePages(t *testing.T) {
	page := func(id string, more bool) string {
		m := "false"
		if more {
			m = "true"
		}
		return `{"success":true,"meta":{"total":2,"has_more_pages":` + m + `},"results":[{"id":` + id + `,"title":"T` + id + `"}]}`
	}
	fake := (&routedHTTP{}).route("page=1&", page("1", true)).route("page=2&", page("2", false))
	jobs, err := NewMagnit(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
}

func TestMagnitListFailureFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("/api/v1/vacancy?", &StatusError{Code: http.StatusForbidden})
	if _, err := NewMagnit(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("Fetch: want an error when the listing cannot be read")
	}
}

func TestMagnitAllDetailsFailingIsAnError(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/vacancy?", deelFixture(t, "magnit_list.json")).
		routeErr("/api/v1/vacancy/", &StatusError{Code: http.StatusForbidden})
	_, err := NewMagnit(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil {
		t.Fatal("FetchNew: want an error when every listed posting's detail failed")
	}
}

func TestMagnitWorkMode(t *testing.T) {
	cases := map[string]string{
		"Москва и Краснодар (гибрид/офис), другие города - удаленно": "remote",
		"Москва (гибрид/офис)":    "hybrid",
		"Краснодар (гибрид/офис)": "hybrid",
		"Всегда из офиса":         "onsite",
		"":                        "",
	}
	for in, want := range cases {
		var formats []magnitNamed
		if in != "" {
			formats = []magnitNamed{{Name: in}}
		}
		if got := magnitWorkMode(formats); got != want {
			t.Errorf("magnitWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	// The broadest arrangement a posting offers wins.
	both := []magnitNamed{{Name: "Всегда из офиса"}, {Name: "Москва (гибрид/офис)"}}
	if got := magnitWorkMode(both); got != "hybrid" {
		t.Errorf("magnitWorkMode(onsite+hybrid) = %q, want hybrid", got)
	}
}
