package sources

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestHuntflowProvider(t *testing.T) {
	if got := NewHuntflow(nil).Provider(); got != "huntflow" {
		t.Errorf("Provider() = %q, want %q", got, "huntflow")
	}
}

// rawSlice parses a devalue payload string into the []json.RawMessage the decoder takes.
func rawSlice(t *testing.T, s string) []json.RawMessage {
	t.Helper()
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		t.Fatalf("parse devalue fixture: %v", err)
	}
	return raw
}

func TestHuntflowUnflattenResolvesReferences(t *testing.T) {
	// devalue: every value is a node in the array; object/array fields are indices into
	// it, scalars are stored once and shared by index (null at index 8 here).
	raw := rawSlice(t, `[
		{"data":1},{"vacancies":2},{"items":3},[4],
		{"id":5,"slug":6,"position":7,"city":8,"money":8,"division":8,"archived_at":8},
		26610,"senior-backend-developer-2","Senior Backend Developer",null
	]`)

	node, err := unflattenPayload(raw)
	if err != nil {
		t.Fatalf("unflattenPayload: %v", err)
	}
	got, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"data":{"vacancies":{"items":[{"archived_at":null,"city":null,"division":null,"id":26610,"money":null,"position":"Senior Backend Developer","slug":"senior-backend-developer-2"}]}}}`
	if string(got) != want {
		t.Errorf("unflatten =\n  %s\nwant\n  %s", got, want)
	}
}

func TestHuntflowUnflattenUnwrapsTypeTokens(t *testing.T) {
	// A devalue array whose first element is a string is a type wrapper (e.g. Nuxt's
	// ShallowReactive) and must resolve to its wrapped value, not to a literal array.
	raw := rawSlice(t, `[{"data":1},["ShallowReactive",2],"ok"]`)

	node, err := unflattenPayload(raw)
	if err != nil {
		t.Fatalf("unflattenPayload: %v", err)
	}
	got, _ := json.Marshal(node)
	if want := `{"data":"ok"}`; string(got) != want {
		t.Errorf("unflatten = %s, want %s", got, want)
	}
}

// hfListBody builds a devalue list payload for one vacancy with the given fields.
// archived is "null" for an open vacancy or a quoted timestamp for an archived one.
func hfListBody(id int, slug, position, archived string) string {
	return `[
		{"data":1},{"vacancies":2},{"items":3},[4],
		{"id":5,"slug":6,"position":7,"archived_at":8},
		` + strconv.Itoa(id) + `,"` + slug + `","` + position + `",` + archived + `
	]`
}

// hfDetailBody builds a devalue detail payload for one vacancy. money/city are "null"
// or a quoted string; the body HTML carries the description.
func hfDetailBody(id int, position, city, money, body string) string {
	return `[
		{"data":1},{"vacancy":2},
		{"id":3,"position":4,"city":5,"money":6,"intro":7,"body":8,"requirements":9,"conditions":9,"is_archived":10},
		` + strconv.Itoa(id) + `,"` + position + `",` + city + `,` + money + `,"","` + body + `","",false
	]`
}

func TestHuntflowFetchMapsListAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/vacancy/senior-backend-developer-2/_payload.json",
			hfDetailBody(26610, "Senior Backend Developer", "null", "null", "<p>Build systems.</p>")).
		route(".huntflow.io/_payload.json", hfListBody(26610, "senior-backend-developer-2", "Senior Backend Developer", "null"))

	jobs, err := NewHuntflow(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tripster", Provider: "huntflow", Board: "tripster",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "26610" {
		t.Errorf("ExternalID = %q, want 26610", j.ExternalID)
	}
	if j.Title != "Senior Backend Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://tripster.huntflow.io/vacancy/senior-backend-developer-2" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Tripster" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if !strings.Contains(j.Description, "Build systems.") {
		t.Errorf("Description = %q, want the body HTML", j.Description)
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (feed has no date)", j.PostedAt)
	}
}

func TestHuntflowFetchSkipsArchived(t *testing.T) {
	// Two list items; one archived. Only the open one is detailed and returned.
	list := `[
		{"data":1},{"vacancies":2},{"items":3},[4,9],
		{"id":5,"slug":6,"position":7,"archived_at":8},100,"open","Open Role",null,
		{"id":10,"slug":11,"position":12,"archived_at":13},200,"gone","Gone Role","2024-01-01"
	]`
	fake := (&routedHTTP{}).
		route(".huntflow.io/_payload.json", list).
		route("/vacancy/open/_payload.json", hfDetailBody(100, "Open Role", "null", "null", "<p>Hiring.</p>"))

	jobs, err := NewHuntflow(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "huntflow", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "100" {
		t.Fatalf("want only the open vacancy, got %d jobs", len(jobs))
	}
}

func TestHuntflowFetchFoldsSalaryIntoDescription(t *testing.T) {
	fake := (&routedHTTP{}).
		route(".huntflow.io/_payload.json", hfListBody(1, "lawyer", "Lawyer", "null")).
		route("/vacancy/lawyer/_payload.json",
			hfDetailBody(1, "Lawyer", "null", `"до 190 000 рублей"`, "<p>Do law.</p>"))

	jobs, _ := NewHuntflow(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Flowwow", Provider: "huntflow", Board: "flowwow",
	})
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "190 000") {
		t.Errorf("Description should fold in money, got %q", jobs[0].Description)
	}
}

func TestHuntflowFetchDetectsRussianRemote(t *testing.T) {
	fake := (&routedHTTP{}).
		route(".huntflow.io/_payload.json", hfListBody(1, "dev", "Developer", "null")).
		route("/vacancy/dev/_payload.json",
			hfDetailBody(1, "Developer", `"Москва / удалённо"`, "null", "<p>Code.</p>"))

	jobs, _ := NewHuntflow(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "huntflow", Board: "acme",
	})
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !jobs[0].Remote {
		t.Errorf("Remote = false, want true for Russian 'удалённо' location")
	}
}

func TestHuntflowFetchSkipsFailedDetail(t *testing.T) {
	// The second vacancy has no detail route -> its detail fetch errors and it is
	// skipped, but the first still comes through.
	list := `[
		{"data":1},{"vacancies":2},{"items":3},[4,9],
		{"id":5,"slug":6,"position":7,"archived_at":8},1,"good","Good",null,
		{"id":10,"slug":11,"position":12,"archived_at":13},2,"broken","Broken",null
	]`
	fake := (&routedHTTP{}).
		route("/vacancy/good/_payload.json", hfDetailBody(1, "Good", "null", "null", "<p>Ok.</p>")).
		route(".huntflow.io/_payload.json", list)

	jobs, err := NewHuntflow(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "huntflow", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("want only the good vacancy, got %d jobs", len(jobs))
	}
}
