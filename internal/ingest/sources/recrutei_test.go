package sources

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func recruteiListingURL(board string) string {
	return "https://api.recrutei.com.br/api/v2/vacancies/per-departments/" + board
}

// recruteiListingJSON builds a per-departments listing response body with every item in one
// department, whose declared total matches the item count — the happy-path shape most tests
// want; TestRecruteiFetchFailsWhenTotalDisagreesWithItemCount builds its own mismatched body.
func recruteiListingJSON(items ...string) string {
	n := strconv.Itoa(len(items))
	return `{"message":"ok","data":{"total":` + n + `,"departments":1,"vacancies":[{"department":"Geral","total":` +
		n + `,"items":[` + strings.Join(items, ",") + `]}]}}`
}

func recruteiItemJSON(id int, title, regime, companyName string, location []string, publicLink string) string {
	quoted := make([]string, len(location))
	for i, l := range location {
		quoted[i] = `"` + l + `"`
	}
	return `{"id":` + strconv.Itoa(id) + `,"title":"` + title + `","regime":"` + regime +
		`","company_name":"` + companyName + `","location":[` + strings.Join(quoted, ",") +
		`],"public_link":"` + publicLink + `","slug":"slug","client":null,"pcd":false}`
}

func recruteiDetailHTML(title, description, datePosted string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"JobPosting","title":"` + title +
		`","description":"` + description + `","datePosted":"` + datePosted +
		`","employmentType":"FULL_TIME","jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"undefined","addressRegion":"","addressCountry":"Brasil"}}}` +
		`</script></head><body></body></html>`
}

func TestRecruteiProvider(t *testing.T) {
	if got := NewRecrutei(nil).Provider(); got != "recrutei" {
		t.Errorf("Provider() = %q, want %q", got, "recrutei")
	}
}

// Recrutei earns fullBoardListing because one POST is a proven-complete listing (verified
// against a declared total), so a listing failure or a completeness-check mismatch aborts
// the whole Fetch rather than returning a partial board.
func TestRecruteiMarkers(t *testing.T) {
	s := NewRecrutei(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("recrutei should implement the fullBoardListing marker")
	}
}

func TestRecruteiRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["recrutei"] {
		t.Error("FullBoardListingProviders(All(nil)) should include recrutei")
	}
}

func TestRecruteiRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["recrutei"]
	if !ok {
		t.Fatal("All() missing provider recrutei")
	}
	if s.Provider() != "recrutei" {
		t.Errorf("All()[recrutei].Provider() = %q", s.Provider())
	}
}

func TestRecruteiFetchListsAndHydrates(t *testing.T) {
	board := "digisystem"
	item1 := recruteiItemJSON(108736, "Analista de Suporte N2 Jr", "CLT", "Digisystem",
		[]string{"Brasília", "DF", "Brasil"}, "https://jobs.recrutei.com.br/digisystem/vacancy/108736-analista")
	item2 := recruteiItemJSON(157190, "Desenvolvedor Fullstack Senior", "Pessoa Jurídica", "Digisystem",
		[]string{"Brasil"}, "https://jobs.recrutei.com.br/digisystem/vacancy/157190-dev")
	listing := recruteiListingJSON(item1, item2)

	fake := (&routedHTTP{}).
		route(recruteiListingURL(board), listing).
		route("108736-analista", recruteiDetailHTML("Analista de Suporte N2 Jr", "<p>Suporte ao cliente.</p>", "14/07/2025 14:37:27")).
		route("157190-dev", recruteiDetailHTML("Desenvolvedor Fullstack Senior", "<p>Build things.</p>", "08/09/2026 16:06:44"))

	jobs, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Fallback Co"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "108736" {
		t.Errorf("ExternalID = %q, want 108736", j1.ExternalID)
	}
	if j1.Title != "Analista de Suporte N2 Jr" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "Digisystem" {
		t.Errorf("Company = %q, want the listing's company_name", j1.Company)
	}
	if j1.Location != "Brasília, DF, Brasil" {
		t.Errorf("Location = %q, want the listing's own location, not the detail page's", j1.Location)
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (CLT)", j1.EmploymentType)
	}
	if j1.Description != "<p>Suporte ao cliente.</p>" {
		t.Errorf("Description = %q", j1.Description)
	}
	if j1.PostedAt == nil || j1.PostedAt.Format("2006-01-02") != "2025-07-14" {
		t.Errorf("PostedAt = %v, want 2025-07-14 (from DD/MM/YYYY)", j1.PostedAt)
	}
	if j1.URL != "https://jobs.recrutei.com.br/digisystem/vacancy/108736-analista" {
		t.Errorf("URL = %q", j1.URL)
	}

	j2 := jobs[1]
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (Pessoa Jurídica)", j2.EmploymentType)
	}
	if j2.Location != "Brasil" {
		t.Errorf("Location = %q, want Brasil (not the literal \"undefined\" the detail page would carry)", j2.Location)
	}
}

func TestRecruteiRegimeMapping(t *testing.T) {
	cases := []struct {
		regime string
		want   string
	}{
		{"CLT", "full_time"},
		{"Pessoa Jurídica", "contract"},
		{"CLT ou PJ", ""},
		{"Não informado", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := recruteiEmploymentType(c.regime); got != c.want {
			t.Errorf("recruteiEmploymentType(%q) = %q, want %q", c.regime, got, c.want)
		}
	}
}

func TestRecruteiEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	board := "empty-co"
	fake := (&routedHTTP{}).route(recruteiListingURL(board), recruteiListingJSON())
	jobs, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestRecruteiFetchFailsWholeBoardOnListingError(t *testing.T) {
	board := "boom-co"
	fake := (&routedHTTP{}).routeErr(recruteiListingURL(board), errors.New("boom"))
	if _, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

// recruteiMismatchedTotalJSON declares a total that disagrees with its own items array — the
// shape a pagination change on the platform's side would produce.
const recruteiMismatchedTotalJSON = `{"message":"ok","data":{"total":5,"departments":1,"vacancies":[{"department":"Geral","total":5,"items":[{"id":1,"title":"A","regime":"CLT","company_name":"Acme","location":["Brasil"],"public_link":"https://jobs.recrutei.com.br/acme/vacancy/1-a","slug":"a","client":null,"pcd":false}]}]}}`

func TestRecruteiFetchFailsWhenTotalDisagreesWithItemCount(t *testing.T) {
	board := "acme"
	fake := (&routedHTTP{}).route(recruteiListingURL(board), recruteiMismatchedTotalJSON)
	if _, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite the declared total disagreeing with the summed item count")
	}
}

func TestRecruteiUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	board := "acme"
	item := recruteiItemJSON(1, "Engenheiro", "CLT", "Acme", []string{"Brasil"},
		"https://jobs.recrutei.com.br/acme/vacancy/1-engenheiro")
	fake := (&routedHTTP{}).
		route(recruteiListingURL(board), recruteiListingJSON(item)).
		routeErr("1-engenheiro", errors.New("timeout"))

	jobs, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (an unreadable marker, not a drop)", len(jobs))
	}
	if jobs[0].ExternalID != "1" {
		t.Errorf("ExternalID = %q, want 1", jobs[0].ExternalID)
	}
}

// recruteiStringLocationItemJSON builds a listing item whose "location" is a bare JSON
// STRING rather than the array shape every other test uses — the shape found live in
// production on ~20% of a real tenant's postings (broke the very first live crawl with
// "cannot unmarshal string into []string" the day this adapter shipped).
func recruteiStringLocationItemJSON(id int, title, location, publicLink string) string {
	return `{"id":` + strconv.Itoa(id) + `,"title":"` + title + `","regime":"CLT","company_name":"Acme","location":"` +
		location + `","public_link":"` + publicLink + `","slug":"slug","client":null,"pcd":false}`
}

func TestRecruteiFetchToleratesAStringLocation(t *testing.T) {
	board := "acme"
	naoInformado := recruteiStringLocationItemJSON(1, "Vaga A", "Não informado", "https://jobs.recrutei.com.br/acme/vacancy/1-a")
	otherString := recruteiStringLocationItemJSON(2, "Vaga B", "Remoto", "https://jobs.recrutei.com.br/acme/vacancy/2-b")
	fake := (&routedHTTP{}).
		route(recruteiListingURL(board), recruteiListingJSON(naoInformado, otherString)).
		route("1-a", recruteiDetailHTML("Vaga A", "<p>A</p>", "14/07/2025 14:37:27")).
		route("2-b", recruteiDetailHTML("Vaga B", "<p>B</p>", "14/07/2025 14:37:27"))

	jobs, err := NewRecrutei(fake).Fetch(context.Background(), CompanyEntry{Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v, want a string \"location\" to decode rather than fail the whole board", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].Location != "" {
		t.Errorf("Location = %q, want empty for the \"Não informado\" placeholder", jobs[0].Location)
	}
	if jobs[1].Location != "Remoto" {
		t.Errorf("Location = %q, want the literal string carried through for any other value", jobs[1].Location)
	}
}
