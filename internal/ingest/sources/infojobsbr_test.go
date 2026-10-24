package sources

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live InfoJobs Brasil responses fetched on 2026-10-24: Acre's
// listing page (its meta description, its filter inputs and the first four cards verbatim),
// three of the pager's answers for it (cards verbatim — the last local cards and the first
// "Todo Brasil" ones, two "Todo Brasil" cards, and the final eof page), one posting page (its
// JobPosting and its fact panel), a "Todo Brasil" posting's page (which carries no JobPosting)
// and a closed posting's page. One card's phone number is
// replaced with a placeholder, and the cards' verified-badge tooltips are cut.

const ijbrAcre = infojobsBRBase + "/empregos-em-acre.aspx"

func ijbrFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/infojobsbr/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ijbrFake serves pages by URL and pager answers by "<listing>?page=N".
type ijbrFake struct {
	pages map[string]string
	gets  []string
}

func (f *ijbrFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.gets = append(f.gets, u)
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 500, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *ijbrFake) GetJSON(_ context.Context, u string, v any) error {
	listing, err := url.QueryUnescape(strings.TrimPrefix(u, infojobsBRPager))
	if err != nil {
		return err
	}
	body, ok := f.pages[listing]
	if !ok {
		return &StatusError{Method: "GET", Code: 500, URL: u}
	}
	return json.Unmarshal([]byte(body), v)
}

func ijbrAdapter(f *ijbrFake) infojobsBR {
	return infojobsBR{http: f, limiter: noWait{}, maxNewPerBoard: infojobsBRMaxNewPerBoard}
}

func acreFake(t *testing.T) *ijbrFake {
	return &ijbrFake{pages: map[string]string{
		ijbrAcre:             ijbrFixture(t, "acre_page1.html"),
		ijbrAcre + "?page=2": ijbrFixture(t, "acre_frag2.json"),
		ijbrAcre + "?page=3": ijbrFixture(t, "acre_frag3.json"),
		ijbrAcre + "?page=4": ijbrFixture(t, "acre_frag_eof.json"),
	}}
}

func TestInfojobsBRCardsReadTheListingFacts(t *testing.T) {
	root, err := html.Parse(strings.NewReader(ijbrFixture(t, "acre_page1.html")))
	if err != nil {
		t.Fatal(err)
	}
	cards := parseInfojobsBRCards(root)
	if len(cards) != 4 {
		t.Fatalf("cards = %d, want 4", len(cards))
	}
	want := infojobsBRCard{
		ID: "12151206", Path: "/vaga-de-representante-vendas-rio-branco-em-acre__12151206.aspx",
		Title: "REPRESENTANTE DE VENDAS - RIO BRANCO", Company: "YOUTILITY CENTER DO BRASIL",
		Location: "Rio Branco - AC", Salary: "A combinar", WorkMode: "Presencial",
	}
	if cards[0] != want {
		t.Errorf("card = %+v\nwant %+v", cards[0], want)
	}
	if cards[0].state() != "AC" {
		t.Errorf("state = %q", cards[0].state())
	}
}

func TestInfojobsBRDropsTheHiddenEmployer(t *testing.T) {
	var p struct {
		HTML string `json:"listFragmentHTML"`
	}
	if err := json.Unmarshal([]byte(ijbrFixture(t, "acre_frag_eof.json")), &p); err != nil {
		t.Fatal(err)
	}
	root, _ := html.Parse(strings.NewReader(p.HTML))
	if cards := parseInfojobsBRCards(root); len(cards) != 0 {
		t.Errorf("an \"Empresa confidencial\" card must not be read, got %+v", cards)
	}
}

func TestInfojobsBRStateBoardStopsAtTheNationalTail(t *testing.T) {
	f := acreFake(t)
	cards, err := ijbrAdapter(f).list(context.Background(), "ac")
	if err != nil {
		t.Fatal(err)
	}
	// Four cards on page 1 and three on page 2 are Acre's; page 3 carries only "Todo Brasil"
	// cards, so the walk ends there and never asks for page 4.
	if len(cards) != 7 {
		t.Errorf("cards = %d, want 7", len(cards))
	}
	for _, c := range cards {
		if c.state() != "AC" {
			t.Errorf("a non-Acre card was kept: %+v", c)
		}
	}
	for _, u := range f.gets {
		if strings.Contains(u, "page=4") {
			t.Error("the walk went past the first page without an Acre posting")
		}
	}
}

func TestInfojobsBRNationalBoardKeepsOnlyTodoBrasil(t *testing.T) {
	cards, err := ijbrAdapter(acreFake(t)).list(context.Background(), infojobsBRNationalBoard)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 4 {
		t.Errorf("cards = %d, want the 4 \"Todo Brasil\" ones", len(cards))
	}
	for _, c := range cards {
		if c.Location != "Todo Brasil" {
			t.Errorf("kept %+v", c)
		}
	}
}

func TestInfojobsBRFailsALaterPage(t *testing.T) {
	f := acreFake(t)
	delete(f.pages, ijbrAcre+"?page=2")
	if _, err := ijbrAdapter(f).list(context.Background(), "ac"); err == nil {
		t.Fatal("a failed pager page must fail the board")
	}
}

func TestInfojobsBRCutsATooLargeListing(t *testing.T) {
	f := acreFake(t)
	// Acre's listing claims more than one walk can reach, so it is cut by its categories, each
	// of which the fake serves small enough to walk.
	f.pages[ijbrAcre] = strings.Replace(f.pages[ijbrAcre], `content="1.969 Vagas`, `content="19.690 Vagas`, 1)
	page := ijbrFixture(t, "acre_page1.html")
	for _, u := range infojobsBRFilterURLs(mustParse(t, page), "categoria", ijbrAcre) {
		f.pages[u] = page
	}
	sl, err := ijbrAdapter(f).slices(context.Background(), ijbrAcre, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sl) != 34 || sl[0].url != infojobsBRBase+"/empregos-de-administracao-em-acre.aspx" {
		t.Errorf("slices = %d (first %+v), want one per category", len(sl), sl[0].url)
	}
}

func TestInfojobsBRFailsAListingNoCutReaches(t *testing.T) {
	f := &ijbrFake{pages: map[string]string{
		ijbrAcre: `<html><head><meta name="description" content="19.690 Vagas de Emprego em Acre."></head><body></body></html>`,
	}}
	if _, err := ijbrAdapter(f).slices(context.Background(), ijbrAcre, 0); err == nil {
		t.Fatal("a listing too large to walk and with no filter to cut by must fail")
	}
}

func TestInfojobsBRHydratesOnlyWhatTheGateLeaves(t *testing.T) {
	f := acreFake(t)
	link := infojobsBRBase + "/vaga-de-representante-vendas-rio-branco-em-acre__12151206.aspx"
	f.pages[link] = ijbrFixture(t, "detail.html")
	seen := map[string]bool{"12149475": true}
	gate := DetailGate{
		Covered:  func([]string) map[string]bool { return map[string]bool{"REPOR BRASIL": true} },
		Rejected: func(j Job) bool { return j.ExternalID == "12027100" },
	}
	g := ijbrAdapter(f)
	g.maxNewPerBoard = 1
	jobs, err := g.FetchNewGated(context.Background(), CompanyEntry{Board: "ac"}, func(id string) bool { return seen[id] }, gate)
	if err != nil {
		t.Fatal(err)
	}
	var details []string
	for _, u := range f.gets {
		if strings.Contains(u, "/vaga-de-") {
			details = append(details, u)
		}
	}
	if len(details) != 1 || details[0] != link {
		t.Errorf("detail requests = %v, want only %s", details, link)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["12149475"].SeenRefresh {
		t.Error("a stored posting must be refreshed")
	}
	if j, ok := byID["12146881"]; !ok || j.Description != "" {
		t.Errorf("a covered posting must be yielded body-less, got %+v (ok=%v)", j, ok)
	}
	if j, ok := byID["12027100"]; !ok || j.Description != "" {
		t.Errorf("a rejected posting must be yielded body-less, got %+v (ok=%v)", j, ok)
	}
	j := byID["12151206"]
	if j.Company != "Youtility Center do Brasil" || j.Location != "Rio Branco, AC, Brasil" || j.WorkMode != "onsite" || j.EmploymentType != "full_time" {
		t.Errorf("job = %+v", j)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "br" {
		t.Errorf("countries = %v", j.Countries)
	}
	if !strings.Contains(j.Description, "Tipo de contrato e Jornada:") || !strings.Contains(j.Description, "Vale refeição") ||
		!strings.Contains(j.Description, "<h3>Exigências</h3>") || strings.Contains(j.Description, "Premium") {
		t.Errorf("description = %q", j.Description)
	}
	want := time.Date(2026, 10, 23, 19, 56, 0, 0, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want %v", j.PostedAt, want)
	}
	// Past the per-board bound: listed, unseen, not covered, not rejected — and not yielded.
	for _, id := range []string{"11939284", "11925370", "10492430"} {
		if _, ok := byID[id]; ok {
			t.Errorf("posting %s past the bound was yielded", id)
		}
	}
}

func TestInfojobsBRNationalPostingHasNoJobPosting(t *testing.T) {
	c := infojobsBRCard{ID: "12151980", Title: "Analista Acadêmico II", Company: "UNIASSELVI", Location: "Todo Brasil", WorkMode: "Home office"}
	j, ok := infojobsBRJob(mustParse(t, ijbrFixture(t, "detail_national.html")), c, "u")
	if !ok {
		t.Fatal("a live posting without a JobPosting must still be read")
	}
	if j.Company != "UNIASSELVI" || j.Location != "Todo Brasil" || j.Countries != nil || len(j.FallbackCountries) != 1 || j.PostedAt != nil {
		t.Errorf("job = %+v", j)
	}
	if j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "full_time" || !strings.Contains(j.Description, "Analista Acadêmico II") {
		t.Errorf("job = %+v", j)
	}
}

func TestInfojobsBRSkipsAListingWithNoResult(t *testing.T) {
	f := &ijbrFake{pages: map[string]string{
		ijbrAcre: `<html><head><meta name="description" content="Vagas de Emprego de Artes em Acre. Temos Novas Vagas."></head><body></body></html>`,
	}}
	sl, err := ijbrAdapter(f).slices(context.Background(), ijbrAcre, 0)
	if err != nil || len(sl) != 0 {
		t.Fatalf("slices = %v, %v; want none and no error", sl, err)
	}
}

func TestInfojobsBRClosedPostingIsDeferred(t *testing.T) {
	if _, ok := infojobsBRJob(mustParse(t, ijbrFixture(t, "closed.html")), infojobsBRCard{ID: "10500000", Title: "x"}, "u"); ok {
		t.Fatal("a closed posting's page carries no JobPosting and must not be read")
	}
}

func TestInfojobsBRFailsWhenNoNewPostingIsRead(t *testing.T) {
	if _, err := ijbrAdapter(acreFake(t)).FetchNew(context.Background(), CompanyEntry{Board: "ac"}, func(string) bool { return false }); err == nil {
		t.Fatal("listing new postings and reading none must fail")
	}
}

func TestInfojobsBRRefusesAnUnknownBoard(t *testing.T) {
	if _, err := ijbrAdapter(acreFake(t)).Fetch(context.Background(), CompanyEntry{Board: "acre"}); err == nil {
		t.Fatal("a board that is not a state code must be refused")
	}
}

func TestInfojobsBRSalaryAndEmploymentType(t *testing.T) {
	for _, c := range []struct {
		in     string
		lo, hi int
		ok     bool
	}{
		{"R$ 1.625,00 a R$ 2.000,00", 1625, 2000, true},
		{"R$ 2.120,00", 2120, 2120, true},
		{"A combinar", 0, 0, false},
		{"R$ 12,50", 0, 0, false},
	} {
		lo, hi, ok := infojobsBRSalary(c.in)
		if ok != c.ok || lo != c.lo || hi != c.hi {
			t.Errorf("salary(%q) = %d-%d %v", c.in, lo, hi, ok)
		}
	}
	for in, want := range map[string]string{
		"Efetivo – CLT - Período Integral":              "full_time",
		"Efetivo – CLT - Parcial manhãs":                "part_time",
		"Estágio - Parcial tardes":                      "internship",
		"Prestador de Serviços (PJ) - Período Integral": "contract",
		"Jovem Aprendiz - Parcial manhãs":               "",
		"Efetivo – CLT - Noturno":                       "",
	} {
		if got := infojobsBREmploymentType(in); got != want {
			t.Errorf("employmentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInfojobsBRDoesNotStoreAPostingClosedSinceTheListing(t *testing.T) {
	f := acreFake(t)
	closed := ijbrFixture(t, "closed.html")
	for _, p := range []string{
		"/vaga-de-representante-vendas-rio-branco-em-acre__12151206.aspx",
		"/vaga-de-artifice-rio-branco-acre-em-acre__12149475.aspx",
	} {
		f.pages[infojobsBRBase+p] = closed
	}
	g := ijbrAdapter(f)
	g.maxNewPerBoard = 2
	jobs, err := g.FetchNew(context.Background(), CompanyEntry{Board: "ac"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("every new posting answering \"gone\" is an answer, not a failed crawl: %v", err)
	}
	for _, j := range jobs {
		t.Errorf("a closed posting was yielded: %+v", j)
	}
}
