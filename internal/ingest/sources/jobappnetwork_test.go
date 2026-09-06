package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// jobappnetworkFake is a test transport for the jobappnetwork adapter: the listing is a single
// POST endpoint, and the "from" offset in the request body picks which canned page to answer.
// An unrequested offset returns an empty hit list, the natural end-of-walk response; an offset
// listed in failAt answers a transport error instead, so a later-page failure can be told apart
// from a later page simply running out of hits.
type jobappnetworkFake struct {
	pages    map[int]string // "from" offset -> _search response JSON
	failAt   map[int]bool   // "from" offset -> answer a transport error instead of a page
	postFail bool
	bodies   []map[string]any // every request body sent, in order
}

func (f *jobappnetworkFake) PostJSON(_ context.Context, _ string, body, v any) error {
	if f.postFail {
		return errors.New("jobappnetworkFake: boom")
	}
	b, _ := body.(map[string]any)
	f.bodies = append(f.bodies, b)
	from, _ := b["from"].(int)
	if f.failAt[from] {
		return errors.New("jobappnetworkFake: page failed")
	}
	raw, ok := f.pages[from]
	if !ok {
		raw = `{"hits":{"total":0,"hits":[]}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestJobAppNetworkProvider(t *testing.T) {
	if got := NewJobAppNetwork(nil).Provider(); got != "jobappnetwork" {
		t.Errorf("Provider() = %q, want %q", got, "jobappnetwork")
	}
}

func TestJobAppNetworkFetchMapsPosting(t *testing.T) {
	fake := &jobappnetworkFake{pages: map[int]string{
		0: `{"hits":{"total":1,"hits":[
			{"_source":{"jobId":7216553,"title":"Shift Leader","description":"<p>Lead the shift.</p><script>x()<\/script>","address":{"city":"Houston","stateOrProvince":"TX","country":"US"},"clientId":20448,"internalOrExternal":"externalOnly","createdDate":"2022-02-16"}}
		]}}`,
	}}

	jobs, err := NewJobAppNetwork(fake).Fetch(context.Background(),
		CompanyEntry{Company: "HZ Coffee Group LLC", Board: "20448"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "7216553" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "Shift Leader" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "HZ Coffee Group LLC" {
		t.Errorf("Company = %q, want the board's configured company", j.Company)
	}
	if j.Location != "Houston, TX, US" {
		t.Errorf("Location = %q", j.Location)
	}
	if want := "https://apply.jobappnetwork.com/clients/20448/posting/7216553/"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("Countries = %v, want [us]", j.Countries)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt not parsed")
	}
	if want := "<p>Lead the shift."; j.Description == "" || !strings.Contains(j.Description, want) {
		t.Errorf("Description = %q, want it to contain %q", j.Description, want)
	}
	if strings.Contains(j.Description, "x()") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
}

func TestJobAppNetworkQueryFiltersClientAndExternalOnly(t *testing.T) {
	fake := &jobappnetworkFake{pages: map[int]string{0: `{"hits":{"total":0,"hits":[]}}`}}
	if _, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "20448"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.bodies) != 1 {
		t.Fatalf("got %d requests, want 1", len(fake.bodies))
	}
	raw, err := json.Marshal(fake.bodies[0])
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	body := string(raw)
	for _, want := range []string{`"clientId":20448`, `"internalOrExternal":"externalOnly"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %q: %s", want, body)
		}
	}
}

func TestJobAppNetworkFetchPaginatesToTotal(t *testing.T) {
	fake := &jobappnetworkFake{pages: map[int]string{
		0: `{"hits":{"total":2,"hits":[
			{"_source":{"jobId":1,"title":"One","description":"d","address":{"country":"US"},"createdDate":"2022-01-01"}}
		]}}`,
		100: `{"hits":{"total":2,"hits":[
			{"_source":{"jobId":2,"title":"Two","description":"d","address":{"country":"US"},"createdDate":"2022-01-01"}}
		]}}`,
	}}
	jobs, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (paginated to hits.total)", len(jobs))
	}
}

// hits.total is claimed exact (see AGENTS.md), but the walk must not trust it blindly: an empty
// page ends the walk even when total overclaims what is left.
func TestJobAppNetworkFetchStopsOnEmptyPageDespiteTotal(t *testing.T) {
	fake := &jobappnetworkFake{pages: map[int]string{
		0: `{"hits":{"total":1000,"hits":[
			{"_source":{"jobId":1,"title":"Only","description":"d","address":{"country":"US"},"createdDate":"2022-01-01"}}
		]}}`,
		// offset 100 is left unconfigured, which the fake answers as an empty hit list.
	}}
	jobs, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (stop at the empty page, not chase an overclaimed total)", len(jobs))
	}
}

// Only the FIRST page failing is a board-level error; a later page failing ends the walk with
// whatever was already gathered.
func TestJobAppNetworkFetchLaterPageFailureKeepsPartialResults(t *testing.T) {
	if _, err := NewJobAppNetwork(&jobappnetworkFake{failAt: map[int]bool{0: true}}).
		Fetch(context.Background(), CompanyEntry{Board: "1"}); err == nil {
		t.Error("want an error when the first listing page fails")
	}

	first := make([]string, jobappnetworkPageSize)
	for i := range first {
		first[i] = fmt.Sprintf(`{"_source":{"jobId":%d,"title":"role","description":"d","address":{"country":"US"},"createdDate":"2022-01-01"}}`, i+1)
	}
	partial := &jobappnetworkFake{
		pages: map[int]string{
			0: fmt.Sprintf(`{"hits":{"total":1000,"hits":[%s]}}`, strings.Join(first, ",")),
		},
		failAt: map[int]bool{jobappnetworkPageSize: true}, // page 2 fails rather than running dry
	}
	jobs, err := NewJobAppNetwork(partial).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != jobappnetworkPageSize {
		t.Errorf("got %d jobs, want the %d gathered before the failure", len(jobs), jobappnetworkPageSize)
	}
}

func TestJobAppNetworkFetchEmptyBoard(t *testing.T) {
	fake := &jobappnetworkFake{pages: map[int]string{0: `{"hits":{"total":0,"hits":[]}}`}}
	jobs, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestJobAppNetworkFetchTransportErrorFailsBoard(t *testing.T) {
	fake := &jobappnetworkFake{postFail: true}
	if _, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "1"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestJobAppNetworkBoardValidation(t *testing.T) {
	cases := []struct {
		board   string
		wantErr bool
	}{
		{"20448", false},
		{"", true},
		{"abc", true},
		{"0", true},
		{"-5", true},
		{"20448a", true},
	}
	for _, c := range cases {
		_, err := parseJobAppNetworkBoard(c.board)
		if (err != nil) != c.wantErr {
			t.Errorf("parseJobAppNetworkBoard(%q) error = %v, wantErr %v", c.board, err, c.wantErr)
		}
	}
}

func TestJobAppNetworkMalformedBoardIssuesNoRequest(t *testing.T) {
	fake := &jobappnetworkFake{}
	if _, err := NewJobAppNetwork(fake).Fetch(context.Background(), CompanyEntry{Board: "not-a-number"}); err == nil {
		t.Fatal("Fetch: want error for malformed board, got nil")
	}
	if len(fake.bodies) != 0 {
		t.Errorf("issued %d requests for a malformed board, want 0", len(fake.bodies))
	}
}

func TestJobAppNetworkRegisteredInAll(t *testing.T) {
	if _, ok := All(nil)["jobappnetwork"]; !ok {
		t.Fatal(`All(nil)["jobappnetwork"] missing`)
	}
}
