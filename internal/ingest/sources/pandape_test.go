package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live Pandapé responses fetched on 2026-10-23: braspress's and
// abacoconsulting's POST /ListVacancies pages (cards verbatim; braspress's three pages cut to
// five cards and their subtotal rewritten to match, abacoconsulting's single page whole) and
// four vacancy pages (the JobPosting block, the title and the description-to-requirements
// section verbatim). The braspress page's recruiter phone number and e-mail address are
// replaced with placeholders.

const pdBraspress = "https://braspress.pandape.infojobs.com.br"

func pdFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/pandape/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// pdFake serves listing pages by "<url>#<PageNumber>" and vacancy pages by URL.
type pdFake struct {
	pages map[string]string
	gets  []string
}

func (f *pdFake) PostFormJSON(_ context.Context, u string, v url.Values, out any) error {
	body, ok := f.pages[u+"#"+v.Get("PageNumber")]
	if !ok {
		return &StatusError{Method: "POST", Code: 500, URL: u}
	}
	return json.Unmarshal([]byte(body), out)
}

func (f *pdFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.gets = append(f.gets, u)
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func pdAdapter(f *pdFake) pandape {
	return pandape{http: f, limiter: noWait{}, maxNewPerBoard: pandapeMaxNewPerBoard}
}

func braspressFake(t *testing.T) *pdFake {
	return &pdFake{pages: map[string]string{
		pdBraspress + "/ListVacancies#1": pdFixture(t, "braspress_page1.json"),
		pdBraspress + "/ListVacancies#2": pdFixture(t, "braspress_page2.json"),
		pdBraspress + "/ListVacancies#3": pdFixture(t, "braspress_page3.json"),
		pdBraspress + "/Detail/3861560":  pdFixture(t, "braspress_detail.html"),
	}}
}

func TestPandapeCardsReadTitleAndFacts(t *testing.T) {
	var p pandapePage
	if err := json.Unmarshal([]byte(pdFixture(t, "abaco_page1.json")), &p); err != nil {
		t.Fatal(err)
	}
	cards, err := parsePandapeCards(p.View)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 9 {
		t.Fatalf("cards = %d, want 9", len(cards))
	}
	want := pandapeCard{ID: "3518777", Title: "Consultor(a) SAP MM Sênior", Location: "Todo Brasil", WorkMode: "Home Office", Contract: "Prestador de Serviços (PJ)"}
	if cards[2] != want {
		t.Errorf("card = %+v, want %+v", cards[2], want)
	}
	if cards[3].WorkMode != "Híbrido" {
		t.Errorf("hybrid card = %+v", cards[3])
	}
}

func TestPandapeWalksEveryPageAndHydratesOnlyUnseen(t *testing.T) {
	f := braspressFake(t)
	seen := map[string]bool{"3861588": true, "3663900": true, "2695835": true, "2740389": true}
	jobs, err := pdAdapter(f).FetchNew(context.Background(), CompanyEntry{Board: "braspress", Company: "Braspress"}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("jobs = %d, want 5", len(jobs))
	}
	if len(f.gets) != 1 || f.gets[0] != pdBraspress+"/Detail/3861560" {
		t.Errorf("detail requests = %v, want only the unseen vacancy", f.gets)
	}
	var j Job
	refreshed := 0
	for _, x := range jobs {
		if x.SeenRefresh {
			refreshed++
		} else {
			j = x
		}
	}
	if refreshed != 4 {
		t.Errorf("refreshed = %d, want 4", refreshed)
	}
	if j.Title != "Ajudante de Manutenção Mecânica" || j.Company != "Braspress" || j.Location != "Guarulhos, SP" {
		t.Errorf("job = %+v", j)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "br" || j.FallbackCountries != nil {
		t.Errorf("countries = %v / %v", j.Countries, j.FallbackCountries)
	}
	if j.WorkMode != "onsite" || j.EmploymentType != "full_time" {
		t.Errorf("work mode %q, employment %q", j.WorkMode, j.EmploymentType)
	}
	// The page body keeps the emoji the JobPosting's description writes as "?".
	if !strings.Contains(j.Description, "🚍 Vale Transporte") || !strings.Contains(j.Description, "Requisitos") {
		t.Errorf("description lacks the page body or the requirements: %q", j.Description)
	}
	want := time.Date(2026, 10, 23, 18, 59, 35, 110860200, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want %v", j.PostedAt, want)
	}
}

func TestPandapeFailsAShortWalk(t *testing.T) {
	f := braspressFake(t)
	// The first page claims a total the walk never reached.
	f.pages[pdBraspress+"/ListVacancies#1"] = strings.Replace(f.pages[pdBraspress+"/ListVacancies#1"], `"subtotal": 5`, `"subtotal": 6`, 1)
	if _, err := pdAdapter(f).FetchNew(context.Background(), CompanyEntry{Board: "braspress"}, func(string) bool { return true }); err == nil {
		t.Fatal("a walk shorter than the stated total must fail")
	}
}

func TestPandapeFailsALaterPage(t *testing.T) {
	f := braspressFake(t)
	delete(f.pages, pdBraspress+"/ListVacancies#2")
	if _, err := pdAdapter(f).FetchNew(context.Background(), CompanyEntry{Board: "braspress"}, func(string) bool { return true }); err == nil {
		t.Fatal("a failed later page must fail the crawl")
	}
}

func TestPandapeUnknownTenantIsBoardGone(t *testing.T) {
	f := &pdFake{pages: map[string]string{}}
	g := pandape{http: notFoundPoster{f}, limiter: noWait{}, maxNewPerBoard: 1}
	_, err := g.FetchNew(context.Background(), CompanyEntry{Board: "nosuchtenant"}, func(string) bool { return false })
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

type notFoundPoster struct{ *pdFake }

func (notFoundPoster) PostFormJSON(_ context.Context, u string, _ url.Values, _ any) error {
	return &StatusError{Method: "POST", Code: 404, URL: u}
}

func TestPandapeBoundsHydrationPerBoard(t *testing.T) {
	f := braspressFake(t)
	g := pdAdapter(f)
	g.maxNewPerBoard = 1
	// Two unseen vacancies in listing order (3861560, then 3663900); the bound takes the first.
	unseen := map[string]bool{"3861560": true, "3663900": true}
	jobs, err := g.FetchNew(context.Background(), CompanyEntry{Board: "braspress"}, func(id string) bool { return !unseen[id] })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.gets) != 1 || f.gets[0] != pdBraspress+"/Detail/3861560" {
		t.Errorf("detail requests = %v, want only the first unseen vacancy", f.gets)
	}
	// The vacancy past the bound is not yielded at all, so nothing is stored for it yet.
	if len(jobs) != 4 {
		t.Errorf("jobs = %d, want 1 hydrated + 3 refreshed", len(jobs))
	}
}

func TestPandapeFailsWhenNoNewVacancyIsRead(t *testing.T) {
	f := braspressFake(t)
	delete(f.pages, pdBraspress+"/Detail/3861560")
	if _, err := pdAdapter(f).FetchNew(context.Background(), CompanyEntry{Board: "braspress"}, func(id string) bool { return id != "3861560" }); err == nil {
		t.Fatal("listing new vacancies and reading none must fail")
	}
}

func TestPandapeRemoteAndContractFromCard(t *testing.T) {
	root, err := html.Parse(strings.NewReader(pdFixture(t, "abaco_detail_homeoffice.html")))
	if err != nil {
		t.Fatal(err)
	}
	c := pandapeCard{ID: "3518777", Title: "Consultor(a) SAP MM Sênior", Location: "Todo Brasil", WorkMode: "Home Office", Contract: "Prestador de Serviços (PJ)"}
	j, ok := pandapeJob(root, c, "https://abacoconsulting.pandape.infojobs.com.br/Detail/3518777", CompanyEntry{Company: "Ábaco Consulting"})
	if !ok {
		t.Fatal("not read")
	}
	if j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "contract" || j.Location != "Todo Brasil" {
		t.Errorf("job = %+v", j)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "br" {
		t.Errorf("countries = %v", j.Countries)
	}
}

func TestPandapeSalaryFromJobPosting(t *testing.T) {
	root, err := html.Parse(strings.NewReader(pdFixture(t, "randstad_detail_salary.html")))
	if err != nil {
		t.Fatal(err)
	}
	c := pandapeCard{ID: "3721298", Title: "Project Buyer (Comprador de Projeto)", Contract: "Efetivo – CLT", WorkMode: "Presencial"}
	j, ok := pandapeJob(root, c, "https://randstad-brasil.pandape.infojobs.com.br/Detail/3721298", CompanyEntry{Company: "Randstad"})
	if !ok {
		t.Fatal("not read")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 5000 || j.SalaryMax == nil || *j.SalaryMax != 5500 || j.SalaryCurrency != "BRL" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.Location != "Campina Grande do Sul, PR" || j.EmploymentType != "full_time" {
		t.Errorf("job = %+v", j)
	}
}
