package sources

import (
	"context"
	"strings"
	"testing"
)

// alfaCities is the cities-options response: a dict of {id -> text} the adapter
// resolves cityId against.
const alfaCities = `{"optionLists":{"cities":[
	{"id":"0100","text":"Москва"},
	{"id":"0169","text":"Санкт-Петербург"}
]}}`

// alfaListPage builds one skip/take list page with the given total and inline items.
func alfaListPage(total int, items ...string) string {
	return `{"total":` + itoa(total) + `,"skip":0,"take":100,"items":[` + strings.Join(items, ",") + `]}`
}

func alfaItemJSON(id, name, cityID, slug, createdAt, description string) string {
	return `{"id":"` + id + `","name":"` + name + `","cityId":"` + cityID +
		`","slug":"` + slug + `","createdAt":"` + createdAt + `","description":"` + description + `"}`
}

func TestAlfaBankProvider(t *testing.T) {
	if got := NewAlfaBank(nil).Provider(); got != "alfabank" {
		t.Errorf("Provider() = %q, want %q", got, "alfabank")
	}
}

func TestAlfaBankIsBoardless(t *testing.T) {
	if _, ok := NewAlfaBank(nil).(boardless); !ok {
		t.Error("alfabank should implement the boardless marker")
	}
}

func TestAlfaBankFetchPaginatesAndMaps(t *testing.T) {
	// total=150 -> two list pages (skip=0, skip=100). City resolved from the dict;
	// remote inferred from the /remote-job/ slug segment.
	fake := (&routedHTTP{}).
		route("listId=cities", alfaCities).
		route("skip=0", alfaListPage(150,
			alfaItemJSON("36000", "Разработчик", "0100", "/moskva/remote-job/razrabotchik_36000",
				"2026-06-11T14:23:51.953575Z", "<p>Build things.</p><script>alert(1)</script>"),
		)).
		route("skip=100", alfaListPage(150,
			alfaItemJSON("36001", "Аналитик", "0169", "/spb/office-job/analitik_36001",
				"2026-05-01T09:00:00Z", "<p>Analyze things.</p>"),
		))

	jobs, err := NewAlfaBank(fake).Fetch(context.Background(), CompanyEntry{Company: "Alfa-Bank", Provider: "alfabank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across two pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["36000"]
	if !ok {
		t.Fatal("vacancy 36000 missing")
	}
	if j.Title != "Разработчик" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Alfa-Bank" {
		t.Errorf("Company = %q, want Alfa-Bank", j.Company)
	}
	if want := "https://job.alfabank.ru/moskva/remote-job/razrabotchik_36000"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q, want resolved city Москва", j.Location)
	}
	if !strings.Contains(j.Description, "Build things.") || strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true (/remote-job/ slug segment)")
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 6 {
		t.Errorf("PostedAt = %v, want parsed 2026-06 createdAt", j.PostedAt)
	}

	second := byID["36001"]
	if second.Location != "Санкт-Петербург" {
		t.Errorf("36001 Location = %q, want Санкт-Петербург", second.Location)
	}
	if second.Remote {
		t.Error("36001 Remote = true, want false (office-job slug, non-remote city)")
	}
}

func TestAlfaBankUnknownCityIsEmpty(t *testing.T) {
	fake := (&routedHTTP{}).
		route("listId=cities", alfaCities).
		route("skip=0", alfaListPage(1,
			alfaItemJSON("1", "Job", "9999", "/x/office-job/job_1", "2026-05-01T09:00:00Z", "<p>x</p>"),
		))

	jobs, err := NewAlfaBank(fake).Fetch(context.Background(), CompanyEntry{Company: "Alfa-Bank", Provider: "alfabank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].Location != "" {
		t.Errorf("Location = %q, want empty for unknown cityId", jobs[0].Location)
	}
}

func TestAlfaBankStopsOnEmptyPageDespiteHigherTotal(t *testing.T) {
	// Total is misreported as 250 (larger than the single row actually served). The empty
	// second page must terminate the crawl; without the guard the loop advances to skip=200
	// (unrouted here → the fake errors), so a clean single-job return proves it stopped.
	fake := (&routedHTTP{}).
		route("listId=cities", alfaCities).
		route("skip=0", alfaListPage(250,
			alfaItemJSON("1", "Job", "0100", "/moskva/office-job/job_1", "2026-05-01T09:00:00Z", "<p>x</p>"),
		)).
		route("skip=100", alfaListPage(250)) // empty items, but skip+take (200) still < 250

	jobs, err := NewAlfaBank(fake).Fetch(context.Background(), CompanyEntry{Company: "Alfa-Bank", Provider: "alfabank"})
	if err != nil {
		t.Fatalf("Fetch: %v (crawl did not stop on the empty page and reached an unrouted skip)", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (empty page must stop the crawl)", len(jobs))
	}
}

func TestAlfaBankEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).
		route("listId=cities", alfaCities).
		route("skip=0", alfaListPage(0))

	jobs, err := NewAlfaBank(fake).Fetch(context.Background(), CompanyEntry{Company: "Alfa-Bank", Provider: "alfabank"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
