package sources

import (
	"context"
	"strings"
	"testing"
)

func TestHalykProvider(t *testing.T) {
	s := NewHalyk(nil)
	if s.Provider() != "halyk" {
		t.Fatalf("provider %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Fatal("halyk must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Fatal("halyk must prove a full listing")
	}
	if !FullBoardListingProviders(All(nil))["halyk"] {
		t.Fatal("halyk missing from FullBoardListingProviders")
	}
}

func TestHalykExternalIDIgnoresPath(t *testing.T) {
	if halykExternalID("GoLang разработчик", "Алматы") != halykExternalID("  golang   разработчик ", "алматы") {
		t.Fatal("id must fold case and whitespace")
	}
	if halykExternalID("GoLang разработчик", "Алматы") == halykExternalID("GoLang разработчик", "Астана") {
		t.Fatal("city is part of the identity")
	}
}

func TestHalykFetch(t *testing.T) {
	const list = `<html><body>
		<p>Всего: 2 вакансий</p>
		<a href="https://halykbank.kz/about/career/vacancies-inner/9" class="block">
			<div class="flex-x"><svg></svg><div>Алматы</div></div>
			<div class="text-lg font-bold">Red Team Expert</div>
		</a>
		<a href="/about/career/vacancies-inner/1" class="block">
			<div class="flex-x"><div>Астана</div></div>
			<div class="text-lg font-bold">GoLang разработчик</div>
		</a>
	</body></html>`
	const red = `<html><body>
		<div class="mb-6">
			<div class="text-gray-500 mb-2">Алматы</div>
			<div class="text-3xl font-bold mb-2">Red Team Expert</div>
		</div>
		<div class="vacanices-inner-cie-item mb-6">
			<h5>Основные обязанности:</h5>
			Проведение атак<br />
		</div>
	</body></html>`
	const goPage = `<html><body>
		<div class="text-3xl font-bold">GoLang разработчик</div>
		<div class="vacanices-inner-cie-item"><h5>Требования:</h5>Go</div>
	</body></html>`

	client := &routedHTTP{}
	client.route("vacancies-inner/2", red)
	client.route("vacancies-inner/9", red)
	client.route("vacancies-inner/1", goPage)
	client.route("https://halykbank.kz/about/career/vacancies", list)

	entry := CompanyEntry{Company: "Halyk Bank", Provider: "halyk"}
	jobs, err := NewHalyk(client).Fetch(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs %d", len(jobs))
	}
	if jobs[0].ExternalID != halykExternalID("Red Team Expert", "Алматы") {
		t.Fatalf("id follows the path number: %s", jobs[0].ExternalID)
	}
	if jobs[0].ExternalID == "9" || jobs[1].ExternalID == "1" {
		t.Fatal("path number must not be the external id")
	}
	if jobs[0].URL != "https://halykbank.kz/about/career/vacancies-inner/9" {
		t.Fatalf("url %s", jobs[0].URL)
	}
	if jobs[1].URL != "https://halykbank.kz/about/career/vacancies-inner/1" {
		t.Fatalf("relative url resolved to %s", jobs[1].URL)
	}
	if jobs[0].Company != "Halyk Bank" || jobs[0].Location != "Алматы" || jobs[1].Location != "Астана" {
		t.Fatalf("company/location %+v %+v", jobs[0], jobs[1])
	}
	if !strings.Contains(jobs[0].Description, "Проведение атак") || !strings.Contains(jobs[1].Description, "Требования") {
		t.Fatalf("bodies %q %q", jobs[0].Description, jobs[1].Description)
	}

	swapped := strings.Replace(list, "vacancies-inner/9", "vacancies-inner/2", 1)
	swapped = strings.Replace(swapped, "vacancies-inner/1", "vacancies-inner/9", 1)
	client.routes[1].body = goPage
	client.routes[3].body = swapped
	again, err := NewHalyk(client).Fetch(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].ExternalID != jobs[0].ExternalID || again[1].ExternalID != jobs[1].ExternalID {
		t.Fatalf("reshuffled paths minted new ids: %s %s", again[0].ExternalID, again[1].ExternalID)
	}
	if again[0].URL == jobs[0].URL {
		t.Fatal("the stored url must follow the path the crawl just opened")
	}
}

func TestHalykFetchRejectsAShortList(t *testing.T) {
	page := `<html><body><p>Всего: 2 вакансий</p>
		<a href="/about/career/vacancies-inner/1"><div class="flex-x">Алматы</div><div class="text-lg font-bold">A</div></a>
	</body></html>`
	client := &routedHTTP{}
	client.route("https://halykbank.kz/about/career/vacancies", page)
	_, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{Company: "Halyk Bank"})
	if err == nil || !strings.Contains(err.Error(), "states 2") {
		t.Fatalf("err %v", err)
	}
}

func TestHalykFetchRejectsASharedIdentity(t *testing.T) {
	page := `<html><body><p>Всего: 2 вакансий</p>
		<a href="/about/career/vacancies-inner/1"><div class="flex-x">Алматы</div><div class="text-lg font-bold">A</div></a>
		<a href="/about/career/vacancies-inner/2"><div class="flex-x">Алматы</div><div class="text-lg font-bold">A</div></a>
	</body></html>`
	client := &routedHTTP{}
	client.route("vacancies-inner/1", `<div class="vacanices-inner-cie-item">Same duties</div>`)
	client.route("vacancies-inner/2", `<div class="vacanices-inner-cie-item"><p> same   duties </p></div>`)
	client.route("https://halykbank.kz/about/career/vacancies", page)
	_, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{Company: "Halyk Bank"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err %v", err)
	}
}

func TestHalykEmptyBodyIsUnreadable(t *testing.T) {
	list := `<html><body><p>Всего: 1 вакансий</p>
		<a href="https://halykbank.kz/about/career/vacancies-inner/4">
			<div class="flex-x">Алматы</div>
			<div class="text-lg font-bold">Кассир</div>
		</a></body></html>`
	client := &routedHTTP{}
	client.route("vacancies-inner/4", `<html><body><div class="text-3xl font-bold">Кассир</div></body></html>`)
	client.route("https://halykbank.kz/about/career/vacancies", list)
	jobs, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{Company: "Halyk Bank"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || !jobs[0].Unreadable || jobs[0].ExternalID != halykExternalID("Кассир", "Алматы") {
		t.Fatalf("jobs %+v", jobs)
	}
}

func TestHalykGonePageIsDropped(t *testing.T) {
	list := `<html><body><p>Всего: 1 вакансий</p>
		<a href="https://halykbank.kz/about/career/vacancies-inner/4">
			<div class="flex-x">Алматы</div>
			<div class="text-lg font-bold">Кассир</div>
		</a></body></html>`
	client := &routedHTTP{}
	client.routeErr("vacancies-inner/4", &StatusError{Code: 404})
	client.route("https://halykbank.kz/about/career/vacancies", list)
	_, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{Company: "Halyk Bank"})
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Fatalf("err %v", err)
	}
}
