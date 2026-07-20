package sources

import (
	"context"
	"strings"
	"testing"
)

// yandexCrowdVacancyJSON builds one vacancy object for the embedded catalogue. remotely and
// employment are the structured tag values the adapter maps; available gates taken-down postings.
func yandexCrowdVacancyJSON(id, title, description, url string, available bool, remotely, employment string) string {
	avail := "false"
	if available {
		avail = "true"
	}
	return `{"id":` + id +
		`,"title":"` + title + `"` +
		`,"description":"` + description + `"` +
		`,"url":"` + url + `"` +
		`,"payment":"от&nbsp;47&nbsp;000&nbsp;₽"` +
		`,"available":` + avail +
		`,"tags":{"hot":false,"experience":"до года","remotely":"` + remotely +
		`","direction":["поддержка"],"employment":["` + employment + `"]}}`
}

// yandexCrowdPage wraps direction groups in the <script id="data"> block the /vacancies page
// server-renders, mirroring the real host's shape (a JSON array of {vacancies:[…]} groups).
func yandexCrowdPage(groups ...string) string {
	return `<html><body><div class="lc-custom-html">` +
		`<script id="data" type="application/json">` +
		`[` + strings.Join(groups, ",") + `]` +
		`</script></div></body></html>`
}

func yandexCrowdGroupJSON(vacancies ...string) string {
	return `{"id":1,"direction-name":"Поддержка","direction-title":"support",` +
		`"vacancies":[` + strings.Join(vacancies, ",") + `]}`
}

func TestYandexCrowdProvider(t *testing.T) {
	if got := NewYandexCrowd(nil).Provider(); got != "yandexcrowd" {
		t.Errorf("Provider() = %q, want %q", got, "yandexcrowd")
	}
}

func TestYandexCrowdIsBoardless(t *testing.T) {
	if _, ok := NewYandexCrowd(nil).(boardless); !ok {
		t.Error("yandexcrowd must implement boardless: it is a single-company adapter with no per-tenant board")
	}
}

func TestYandexCrowdFetchKeepsAvailableAndMaps(t *testing.T) {
	// The catalogue is a stale list: only available:true vacancies still have a live page, so
	// the taken-down (available:false) one is dropped. External identity is the URL path slug;
	// the float id is ignored. remotely/employment tags map to WorkMode/EmploymentType.
	page := yandexCrowdPage(
		yandexCrowdGroupJSON(
			yandexCrowdVacancyJSON("1.1", "Аналитик по&nbsp;соцсетям", "Осваивайте профессию",
				"https://crowd.yandex.ru/back_office/analytics/analitik_smm", true, "удалённо", "полная"),
			yandexCrowdVacancyJSON("1.2", "Оператор колл-центра", "Помогайте клиентам",
				"https://crowd.yandex.ru/support/operator_local", true, "локально", "частичная"),
			yandexCrowdVacancyJSON("1.3", "Закрытая вакансия", "Её страница 404",
				"https://crowd.yandex.ru/support/finteh_chat", false, "удалённо", "полная"),
		),
	)
	fake := (&routedHTTP{}).route("/vacancies", page)

	jobs, err := NewYandexCrowd(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex Crowd", Provider: "yandexcrowd"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (available:false dropped)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["back_office/analytics/analitik_smm"]
	if !ok {
		t.Fatalf("missing job keyed by URL path; got ids %v", keysOf(byID))
	}
	if j.Company != "Yandex Crowd" {
		t.Errorf("Company = %q, want Yandex Crowd", j.Company)
	}
	if want := "https://crowd.yandex.ru/back_office/analytics/analitik_smm"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if strings.Contains(j.Title, "&nbsp;") || !strings.Contains(j.Title, "Аналитик") {
		t.Errorf("Title = %q, want entity-decoded", j.Title)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote (remotely=удалённо)", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (полная)", j.EmploymentType)
	}

	local, ok := byID["support/operator_local"]
	if !ok {
		t.Fatal("missing local operator job")
	}
	if local.Remote || local.WorkMode != "onsite" {
		t.Errorf("local: Remote=%v WorkMode=%q, want onsite (remotely=локально)", local.Remote, local.WorkMode)
	}
	if local.EmploymentType != "part_time" {
		t.Errorf("local EmploymentType = %q, want part_time (частичная)", local.EmploymentType)
	}
}

func TestYandexCrowdMissingCatalogueErrors(t *testing.T) {
	fake := (&routedHTTP{}).route("/vacancies", "<html><body>no data script</body></html>")

	if _, err := NewYandexCrowd(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex Crowd"}); err == nil {
		t.Fatal("Fetch: want error when the <script id=\"data\"> catalogue is absent")
	}
}

func keysOf(m map[string]Job) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
