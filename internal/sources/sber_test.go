package sources

import (
	"context"
	"strings"
	"testing"
)

// sberVacancy builds one inline vacancy fragment (body fields included — no detail call).
func sberVacancy(reqID string, internalID int, title, company, city, pubDate, intro, duties, requirements, conditions string) string {
	return `{"requisitionId":"` + reqID + `","internalId":` + itoa(internalID) +
		`,"title":"` + title + `","company":"` + company +
		`","city":"` + city + `","publicationDate":"` + pubDate +
		`","introduction":"` + intro + `","duties":"` + duties +
		`","requirements":"` + requirements + `","conditions":"` + conditions + `"}`
}

// sberListPage wraps vacancies in the data/total/success envelope.
func sberListPage(total int, vacancies ...string) string {
	return `{"data":{"vacancies":[` + strings.Join(vacancies, ",") +
		`],"total":` + itoa(total) + `},"success":true}`
}

func TestSberProvider(t *testing.T) {
	if got := NewSber(nil).Provider(); got != "sber" {
		t.Errorf("Provider() = %q, want %q", got, "sber")
	}
}

func TestSberIsBoardless(t *testing.T) {
	if _, ok := NewSber(nil).(boardless); !ok {
		t.Error("sber should implement the boardless marker")
	}
}

func TestSberSkipTakePaginatesAndMapsInline(t *testing.T) {
	// total=300 with take 200: page 1 (skip 0) and page 2 (skip 200); after page 2,
	// skip+200=400 >= 300, so the loop stops. Bodies are inline — no detail call.
	fake := (&routedHTTP{}).
		route("skip=0", sberListPage(300,
			sberVacancy("uuid-111", 4522762, "Senior C++", `АО \"СберТех\"`, "г Москва",
				"2026-06-11T16:17:34.000Z",
				"intro text", "### Duties", "### Requirements", "### Conditions"),
		)).
		route("skip=200", sberListPage(300,
			sberVacancy("uuid-222", 555, "Backend", "", "г Санкт-Петербург",
				"2026-04-01T09:00:00.000Z", "i2", "d2", "r2", "c2"),
		))

	jobs, err := NewSber(fake).Fetch(context.Background(), CompanyEntry{Company: "Sber", Provider: "sber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across two skip/take pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["uuid-111"]
	if !ok {
		t.Fatal("vacancy uuid-111 missing")
	}
	if j.Title != "Senior C++" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != `АО "СберТех"` {
		t.Errorf("Company = %q, want vacancy company", j.Company)
	}
	// URL is built from the numeric internalId (the id the public page resolves), not the
	// requisitionId GUID (which 404s); the requisitionId stays the dedup ExternalID.
	if want := "https://rabota.sber.ru/search/4522762"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.ExternalID != "uuid-111" {
		t.Errorf("ExternalID = %q, want requisitionId uuid-111", j.ExternalID)
	}
	if j.Location != "г Москва" {
		t.Errorf("Location = %q, want city", j.Location)
	}
	for _, want := range []string{"intro text", "Duties", "Requirements", "Conditions"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 6 {
		t.Errorf("PostedAt = %v, want parsed 2026-06 publicationDate", j.PostedAt)
	}

	// uuid-222 has an empty vacancy company -> falls back to the configured entry company.
	if byID["uuid-222"].Company != "Sber" {
		t.Errorf("uuid-222 Company = %q, want fallback to entry company Sber", byID["uuid-222"].Company)
	}
}

func TestSberEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("skip=0", sberListPage(0))

	jobs, err := NewSber(fake).Fetch(context.Background(), CompanyEntry{Company: "Sber", Provider: "sber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
