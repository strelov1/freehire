package sources

import (
	"context"
	"strings"
	"testing"
)

func TestDodoProvider(t *testing.T) {
	if got := NewDodo(nil).Provider(); got != "dodo" {
		t.Errorf("Provider() = %q, want %q", got, "dodo")
	}
}

func TestDodoIsBoardless(t *testing.T) {
	if _, ok := NewDodo(nil).(boardless); !ok {
		t.Error("dodo should implement the boardless marker")
	}
}

func TestDodoFetchFlattensGroupsAndAssemblesDetail(t *testing.T) {
	// Two groups, three items total; each item's body comes from its content blocks,
	// keeping only the four body block types.
	fake := (&routedHTTP{}).
		route("/api/v1/vacancies", `{"success":true,"data":[
			{"speciality":"R&D","slug":"r-d","items":[
				{"id":7805,"position":"Brand Chef","brand":"Dodo Pizza","vacancy_location":"Дубай","work_format":["Гибрид"]}
			]},
			{"speciality":"IT","slug":"it","items":[
				{"id":42,"position":"Backend","brand":"Drinkit","vacancy_location":"Удалённо","work_format":["Удалённо"]},
				{"id":43,"position":"NoBrand","brand":"","vacancy_location":"Москва","work_format":["В офисе"]}
			]}
		]}`).
		route("/api/v1/pages/vacancy/7805", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_main","data":{"position":"Brand Chef"}},
			{"type":"vacancy_text","data":{"text":"<p>Lead the kitchen.</p>"}},
			{"type":"vacancy_image","data":{"image_url":"x.png"}},
			{"type":"vacancy_expectation","data":{"title":"We expect","text":"<ul><li>5 years</li></ul>"}},
			{"type":"vacancy_you_will","data":{"title":"You will","text":"<ul><li>cook</li></ul>"}},
			{"type":"vacancy_benefits","data":{"title":"Benefits","text":"<ul><li>relocation</li></ul>"}}
		]}}}`).
		route("/api/v1/pages/vacancy/42", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_text","data":{"text":"<p>Write services.</p><script>alert(1)</script>"}}
		]}}}`).
		route("/api/v1/pages/vacancy/43", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_text","data":{"text":"<p>Office job.</p>"}}
		]}}}`)

	jobs, err := NewDodo(fake).Fetch(context.Background(), CompanyEntry{Company: "Dodo", Provider: "dodo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two groups", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["7805"]
	if !ok {
		t.Fatal("vacancy 7805 missing")
	}
	if j.Title != "Brand Chef" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Dodo Pizza" {
		t.Errorf("Company = %q, want brand", j.Company)
	}
	if want := "https://dodoteam.ru/vacancy/7805"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Дубай" {
		t.Errorf("Location = %q, want vacancy_location", j.Location)
	}
	for _, want := range []string{"Lead the kitchen.", "5 years", "cook", "relocation"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "image_url") || strings.Contains(j.Description, "x.png") {
		t.Errorf("Description should exclude non-body blocks, got %q", j.Description)
	}
	if j.Remote {
		t.Error("7805 Remote = true, want false (Гибрид)")
	}
}

func TestDodoFetchRemoteAndBrandFallback(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/vacancies", `{"success":true,"data":[
			{"speciality":"IT","slug":"it","items":[
				{"id":42,"position":"Backend","brand":"","vacancy_location":"Удалённо","work_format":["Удалённо"]}
			]}
		]}`).
		route("/api/v1/pages/vacancy/42", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_text","data":{"text":"<p>Write services.</p>"}}
		]}}}`)

	jobs, err := NewDodo(fake).Fetch(context.Background(), CompanyEntry{Company: "Dodo", Provider: "dodo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Dodo" {
		t.Errorf("Company = %q, want fallback to entry company", j.Company)
	}
	if !j.Remote {
		t.Error("Remote = false, want true (work_format Удалённо)")
	}
}

func TestDodoFetchSkipsFailedDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/vacancies", `{"success":true,"data":[
			{"speciality":"IT","slug":"it","items":[
				{"id":42,"position":"Kept","brand":"Dodo Pizza","vacancy_location":"Москва","work_format":["В офисе"]},
				{"id":99,"position":"Broken","brand":"Dodo Pizza","vacancy_location":"Москва","work_format":["В офисе"]}
			]}
		]}`).
		route("/api/v1/pages/vacancy/42", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_text","data":{"text":"<p>ok</p>"}}
		]}}}`)

	jobs, err := NewDodo(fake).Fetch(context.Background(), CompanyEntry{Company: "Dodo", Provider: "dodo"})
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "42" {
		t.Fatalf("want only 42 to survive, got %d jobs", len(jobs))
	}
}

func TestDodoEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/api/v1/vacancies", `{"success":true,"data":[]}`)

	jobs, err := NewDodo(fake).Fetch(context.Background(), CompanyEntry{Company: "Dodo", Provider: "dodo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// The API names Dodo's own tech unit by its team label, "Engineering"; stored as is it reads as
// a company called Engineering. Its site calls itself Dodo Engineering.
func TestDodoEngineeringBrandIsNamedAsTheCompany(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/vacancies", `{"success":true,"data":[
			{"speciality":"Engineering","slug":"speciality-engineering","items":[
				{"id":8068,"position":"Аналитик данных","brand":"Engineering","vacancy_location":"Москва","work_format":["Удалёнка"]}
			]}
		]}`).
		route("/api/v1/pages/vacancy/8068", `{"success":true,"data":{"page":{"content":[
			{"type":"vacancy_text","data":{"text":"<p>Analyse data.</p>"}}
		]}}}`)

	jobs, err := NewDodo(fake).Fetch(context.Background(), CompanyEntry{Company: "Dodo", Provider: "dodo"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch = %v, %v", jobs, err)
	}
	if got := jobs[0].Company; got != "Dodo Engineering" {
		t.Errorf("Company = %q, want Dodo Engineering", got)
	}
}
