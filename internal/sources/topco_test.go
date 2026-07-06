package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// topcoHTTP is a route-aware test client. top.co has two request shapes: the careers listing
// (GetHTML, a Next.js page whose flight embeds the vacancies array) and a per-posting RSC data
// endpoint (GetTextWithHeaders, whose raw flight carries the body/requirements/conditions). The
// fake routes detail requests by the id in the /careers/{id} path and records the RSC header so
// a test can assert it was sent.
type topcoHTTP struct {
	listBody   string            // careers page HTML (with __next_f flight)
	details    map[string]string // detail flight text keyed by posting id
	listErr    bool
	detailErr  map[string]bool
	gotHeaders map[string]string
	gotDetails []string
}

var topcoDetailRE = regexp.MustCompile(`/careers/(\w+)`)

func (f *topcoHTTP) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	if f.listErr {
		return nil, errors.New("topcoHTTP: list boom")
	}
	return html.Parse(strings.NewReader(f.listBody))
}

func (f *topcoHTTP) GetTextWithHeaders(_ context.Context, url string, headers map[string]string) (string, error) {
	f.gotHeaders = headers
	id := ""
	if m := topcoDetailRE.FindStringSubmatch(url); m != nil {
		id = m[1]
	}
	f.gotDetails = append(f.gotDetails, id)
	if f.detailErr[id] {
		return "", errors.New("topcoHTTP: detail boom")
	}
	return f.details[id], nil
}

// topcoPage wraps a flight body into the self.__next_f.push([1,"…"]) <script> shape a top.co
// page server-renders, JSON-string-escaped exactly as Next.js emits it.
func topcoPage(flightBody string) string {
	esc, _ := json.Marshal(flightBody)
	return `<html><body><script>self.__next_f.push([1,` + string(esc) + `])</script></body></html>`
}

// topcoFlight embeds a vacancies JSON array inside a plausible flight body (surrounded by other
// RSC content, so bracketSlice must isolate the array).
func topcoFlight(vacanciesJSON string) string {
	return `["$","div",null,{"products":[{"name":"Wallet"}],"vacancies":` + vacanciesJSON + `}],"footer":1`
}

func TestTopcoProvider(t *testing.T) {
	if got := NewTopco(nil).Provider(); got != "topco" {
		t.Errorf("Provider() = %q, want %q", got, "topco")
	}
}

func TestTopcoRegisteredInAllBoardlessAggregator(t *testing.T) {
	s, ok := All(nil)["topco"]
	if !ok {
		t.Fatal(`All(nil)["topco"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("topco should be boardless (one careers page, no board id)")
	}
	if _, isAggregator := s.(aggregator); !isAggregator {
		t.Error("topco should be an aggregator (many portfolio companies)")
	}
	if !slices.Contains(FilterableProviders(), "topco") {
		t.Error("FilterableProviders() should include the topco aggregator")
	}
}

func TestTopcoNoFlightIsError(t *testing.T) {
	fake := &topcoHTTP{listBody: `<html><body>no flight</body></html>`}
	if _, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when the page carries no flight payload")
	}
}

func TestTopcoNoVacanciesIsError(t *testing.T) {
	fake := &topcoHTTP{listBody: topcoPage(`["$","div",null,{"children":"nothing"}]`)}
	if _, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when the flight has no vacancies array")
	}
}

func TestTopcoMapsVacancy(t *testing.T) {
	vacancies := `[{"id":"45804","position":"Chief Technology Officer / CTO","location":null,
		"level":"Head","jobFunction":"Software Engineering","company":{"name":"Mira"},
		"body":"$undefined","requirements":"$undefined","conditions":"$undefined"}]`
	fake := &topcoHTTP{
		listBody: topcoPage(topcoFlight(vacancies)),
		details: map[string]string{
			"45804": `2:["$","main",null,{"position":"Chief Technology Officer / CTO",` +
				`"body":"<p>Lead the team.</p><script>x()</script>",` +
				`"requirements":"<ul><li>5+ years.</li></ul>",` +
				`"conditions":"<ul><li>Equity.</li></ul>"}]`,
		},
	}

	jobs, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{Company: "Fallback"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "45804" {
		t.Errorf("ExternalID = %q, want 45804", j.ExternalID)
	}
	if want := "https://top.co/careers/45804"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Title != "Chief Technology Officer / CTO" {
		t.Errorf("Title = %q", j.Title)
	}
	// The board lists many portfolio companies, so the employer comes from the posting.
	if j.Company != "Mira" {
		t.Errorf("Company = %q, want Mira (per-posting, not the entry fallback)", j.Company)
	}
	// The full body is fetched from the RSC detail endpoint and sanitized (script stripped).
	for _, want := range []string{"Lead the team", "5+ years", "Equity"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	// The detail endpoint must be reached with the RSC header, else it returns HTML not flight.
	if f := fake; f.gotHeaders["RSC"] != "1" {
		t.Errorf("detail RSC header = %q, want 1", f.gotHeaders["RSC"])
	}
}

func TestTopcoSkipsEmptyID(t *testing.T) {
	vacancies := `[
		{"id":"1","position":"Real","company":{"name":"Co"}},
		{"id":"","position":"No ID","company":{"name":"Co"}}
	]`
	fake := &topcoHTTP{
		listBody: topcoPage(topcoFlight(vacancies)),
		details:  map[string]string{"1": `{"body":"<p>b</p>"}`},
	}
	jobs, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("got %v, want only the id-bearing vacancy", jobs)
	}
}

func TestTopcoKeepsJobWhenDetailFails(t *testing.T) {
	// A failed detail fetch must not drop the posting — it is kept with an empty description
	// rather than lost.
	vacancies := `[{"id":"7","position":"Backend Engineer","company":{"name":"Tribute"}}]`
	fake := &topcoHTTP{
		listBody:  topcoPage(topcoFlight(vacancies)),
		detailErr: map[string]bool{"7": true},
	}
	jobs, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "7" {
		t.Fatalf("got %v, want the posting kept despite detail failure", jobs)
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty on detail failure", jobs[0].Description)
	}
}

func TestTopcoIgnoresUndefinedBody(t *testing.T) {
	// A detail whose fields are still the RSC "$undefined" placeholder yields no description
	// rather than the literal placeholder text.
	vacancies := `[{"id":"3","position":"Role","company":{"name":"Co"}}]`
	fake := &topcoHTTP{
		listBody: topcoPage(topcoFlight(vacancies)),
		details:  map[string]string{"3": `{"body":"$undefined","requirements":"$undefined"}`},
	}
	jobs, err := NewTopco(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty (placeholder ignored)", jobs[0].Description)
	}
}
