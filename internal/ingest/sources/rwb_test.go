package sources

import (
	"context"
	"strings"
	"testing"
)

// rwbListItem builds one list item fragment. employmentTitles are joined into the
// employment_types array, whose titles drive the remote flag.
func rwbListItem(id int, name, city string, employmentTitles ...string) string {
	ets := make([]string, len(employmentTitles))
	for i, t := range employmentTitles {
		ets[i] = `{"id":` + itoa(i+1) + `,"title":"` + t + `"}`
	}
	return `{"id":` + itoa(id) + `,"name":"` + name + `","city_title":"` + city +
		`","employment_types":[` + strings.Join(ets, ",") + `]}`
}

// rwbListPage wraps items in the status/data/range envelope.
func rwbListPage(count, limit, offset int, items ...string) string {
	return `{"status":200,"data":{"items":[` + strings.Join(items, ",") +
		`],"range":{"count":` + itoa(count) + `,"limit":` + itoa(limit) + `,"offset":` + itoa(offset) + `}}}`
}

// rwbDetail builds a detail envelope. The *_arr slices are assembled into the body.
func rwbDetail(description string, duties, requirements, conditions []string) string {
	arr := func(xs []string) string {
		qs := make([]string, len(xs))
		for i, x := range xs {
			qs[i] = `"` + x + `"`
		}
		return "[" + strings.Join(qs, ",") + "]"
	}
	return `{"data":{"description":"` + description +
		`","duties_arr":` + arr(duties) +
		`,"requirements_arr":` + arr(requirements) +
		`,"conditions_arr":` + arr(conditions) + `}}`
}

func TestRWBProvider(t *testing.T) {
	if got := NewRWB(nil).Provider(); got != "rwb" {
		t.Errorf("Provider() = %q, want %q", got, "rwb")
	}
}

func TestRWBIsBoardless(t *testing.T) {
	if _, ok := NewRWB(nil).(boardless); !ok {
		t.Error("rwb should implement the boardless marker")
	}
}

func TestRWBOffsetPaginatesAndMapsDetail(t *testing.T) {
	// count=75 with limit 50: page 1 (offset 0) carries one item, page 2 (offset 50)
	// carries the second; after page 2, offset+50=100 >= 75, so the loop stops.
	fake := (&routedHTTP{}).
		route("offset=0", rwbListPage(75, 50, 0,
			rwbListItem(111, "Backend Engineer", "Москва", "Удаленно"),
		)).
		route("offset=50", rwbListPage(75, 50, 50,
			rwbListItem(222, "Driver", "Подольск", "Офис"),
		)).
		route("/vacancies/111", rwbDetail("<p>About WB.</p>",
			[]string{"Build services;"}, []string{"Go experience;"}, []string{"Remote-friendly."})).
		route("/vacancies/222", rwbDetail("<p>Warehouse.</p>",
			[]string{"Drive forklift;"}, []string{"License;"}, []string{"Shifts."}))

	jobs, err := NewRWB(fake).Fetch(context.Background(), CompanyEntry{Company: "Wildberries", Provider: "rwb"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across two offset pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["111"]
	if !ok {
		t.Fatal("vacancy 111 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Wildberries" {
		t.Errorf("Company = %q, want Wildberries", j.Company)
	}
	if want := "https://career.rwb.ru/vacancies/111"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q, want city_title", j.Location)
	}
	for _, want := range []string{"About WB.", "Build services;", "Go experience;", "Remote-friendly."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if !j.Remote {
		t.Error("111 Remote = false, want true (employment type 'Удаленно')")
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (no date field)", j.PostedAt)
	}

	if byID["222"].Remote {
		t.Error("222 Remote = true, want false (Офис only)")
	}
}

func TestRWBSkipsFailedDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", rwbListPage(1, 200, 0,
			rwbListItem(111, "Kept", "Москва", "Офис"),
			rwbListItem(222, "Broken", "Москва", "Офис"),
		)).
		route("/vacancies/111", rwbDetail("<p>ok</p>", nil, nil, nil))

	jobs, err := NewRWB(fake).Fetch(context.Background(), CompanyEntry{Company: "Wildberries", Provider: "rwb"})
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("want only 111 to survive, got %d jobs", len(jobs))
	}
}

func TestRWBEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("offset=0", rwbListPage(0, 200, 0))

	jobs, err := NewRWB(fake).Fetch(context.Background(), CompanyEntry{Company: "Wildberries", Provider: "rwb"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestRWBListingUsesFiftyItemPages(t *testing.T) {
	items := make([]string, 50)
	for i := range items {
		items[i] = rwbListItem(i+1, "Engineer", "Moscow")
	}
	fake := (&routedHTTP{}).
		route("limit=50&offset=0", rwbListPage(51, 50, 0, items...)).
		route("limit=50&offset=50", rwbListPage(51, 50, 50, rwbListItem(51, "Engineer", "Moscow")))
	itemsRead, err := (rwb{http: fake}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsRead) != 51 || itemsRead[50].ID != 51 {
		t.Fatalf("listing = %d items, want all 51", len(itemsRead))
	}
}
