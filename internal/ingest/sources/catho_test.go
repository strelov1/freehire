package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

func cathoFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/catho/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// cathoFake serves the sitemaps and posting pages from fixtures. pages maps a posting URL to its
// fixture; a posting listed in redirects lands on the slug's search page, as a closed one does.
type cathoFake struct {
	t         *testing.T
	xml       map[string]string
	pages     map[string]string
	redirects map[string]string
	mu        sync.Mutex
	gets      []string
}

func (f *cathoFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	name, ok := f.xml[u]
	if !ok {
		return &StatusError{Code: 404}
	}
	return xml.Unmarshal([]byte(cathoFixture(f.t, name)), v)
}

func (f *cathoFake) GetHTMLResolved(_ context.Context, u string) (*html.Node, string, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	if to, ok := f.redirects[u]; ok {
		// A landing page carrying a posting must still read as gone.
		root, err := html.Parse(strings.NewReader(cathoFixture(f.t, "detail.html")))
		return root, to, err
	}
	name, ok := f.pages[u]
	if !ok {
		return nil, "", &StatusError{Code: 500}
	}
	root, err := html.Parse(strings.NewReader(cathoFixture(f.t, name)))
	return root, u, err
}

const (
	cathoFiscalURL       = "https://www.catho.com.br/vagas/analista-fiscal-junior/38959057/"
	cathoConfidentialURL = "https://www.catho.com.br/vagas/pessoa-arquiteta-de-solucoes-servicenow-senior-clt-home-office/38329440/"
	cathoCleaningURL     = "https://www.catho.com.br/vagas/recepcionista-campo-grande/37812537/"
	cathoClosedURL       = "https://www.catho.com.br/vagas/motorista-categoria-d/38722114/"
)

func newCathoFake(t *testing.T) *cathoFake {
	return &cathoFake{
		t: t,
		xml: map[string]string{
			cathoSitemapIndex: "sitemap-index.xml",
			cathoBaseURL + "/sitemap2/sitemap_vagas_1.xml": "sitemap_vagas_1.xml",
			cathoBaseURL + "/sitemap2/sitemap_vagas_2.xml": "sitemap_vagas_2.xml",
		},
		pages: map[string]string{
			cathoFiscalURL:       "detail.html",
			cathoConfidentialURL: "detail_confidential.html",
			cathoCleaningURL:     "detail_upto.html",
		},
		redirects: map[string]string{cathoClosedURL: "https://www.catho.com.br/vagas/motorista-categoria-d/"},
	}
}

func newTestCatho(h cathoHTTP) catho {
	s := NewCatho(h).(catho)
	s.limiter = noWait{}
	return s
}

func cathoByID(jobs []Job) map[string]Job {
	m := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestCathoMapsAPostingPage(t *testing.T) {
	jobs, err := newTestCatho(newCathoFake(t)).Fetch(context.Background(), CompanyEntry{Provider: "catho"})
	if err != nil {
		t.Fatal(err)
	}
	j, ok := cathoByID(jobs)["38959057"]
	if !ok {
		t.Fatalf("posting 38959057 not yielded: %+v", jobs)
	}
	if j.Title != "Analista Fiscal Júnior" || j.Company != "LISBOA & MORAES CONTABILIDADE" || j.URL != cathoFiscalURL {
		t.Errorf("identity = %q / %q / %q", j.Title, j.Company, j.URL)
	}
	// The city comes from the page header: the ld+json's addressLocality is the state's name.
	if j.Location != "Eldorado do Sul, RS, Brasil" {
		t.Errorf("Location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "br" || j.FallbackCountries != nil {
		t.Errorf("Countries = %v, FallbackCountries = %v", j.Countries, j.FallbackCountries)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02T15:04") != "2026-10-23T00:00" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	// "CLT (Efetivo)" names no schedule, so no employment type.
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	// The header's "A partir de R$ 4.000,00", not the ld+json's 4001-5000 band.
	if j.SalaryMin == nil || *j.SalaryMin != 4000 || j.SalaryMax != nil || j.SalaryCurrency != "BRL" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	for _, want := range []string{
		"<p>Conhecimento em legislação tributária básica (ICMS, IPI, PIS, COFINS)</p>",
		"Tipo de contrato", "Segunda a sexta-feira", "Benefícios", "Vale-transporte",
	} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description lacks %q:\n%s", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "Compartilhar") || strings.Contains(j.Description, "Sobre a vaga") {
		t.Errorf("Description carries the section header row:\n%s", j.Description)
	}
}

func TestCathoDropsHiddenEmployersAndClosedPostings(t *testing.T) {
	jobs, err := newTestCatho(newCathoFake(t)).Fetch(context.Background(), CompanyEntry{Provider: "catho"})
	if err != nil {
		t.Fatal(err)
	}
	got := cathoByID(jobs)
	if _, ok := got["38329440"]; ok {
		t.Error("an Empresa Confidencial posting was yielded")
	}
	if _, ok := got["38722114"]; ok {
		t.Error("a posting that redirected to the search page was yielded")
	}
	// The posting listed in both sitemaps is yielded once.
	if len(jobs) != 2 {
		t.Errorf("yielded %d postings, want 2: %+v", len(jobs), jobs)
	}
	if j := got["37812537"]; j.SalaryMin != nil || j.SalaryMax == nil || *j.SalaryMax != 2000 {
		t.Errorf("Até R$ 2.000,00 = %v-%v", j.SalaryMin, j.SalaryMax)
	}
}

func TestCathoRefreshesSeenAndHydratesNewestFirstWithinTheBudget(t *testing.T) {
	f := newCathoFake(t)
	s := newTestCatho(f)
	s.maxNewPerRun = 1
	seen := func(id string) bool { return id == "38329440" }
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "catho"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	got := cathoByID(jobs)
	if r := got["38329440"]; !r.SeenRefresh || r.URL != cathoConfidentialURL {
		t.Errorf("seen posting = %+v, want a SeenRefresh", r)
	}
	// One new posting per run: the highest id, 38959057, not the sitemap's first.
	if j, ok := got["38959057"]; !ok || j.SeenRefresh {
		t.Errorf("newest unseen posting not hydrated: %+v", jobs)
	}
	if len(jobs) != 2 {
		t.Errorf("yielded %d, want the refresh and one hydration: %+v", len(jobs), jobs)
	}
	for _, u := range f.gets {
		if u == cathoConfidentialURL || u == cathoCleaningURL || u == cathoClosedURL {
			t.Errorf("fetched %s, which is seen or past the budget", u)
		}
	}
}

func TestCathoFailsWhenASitemapFails(t *testing.T) {
	f := newCathoFake(t)
	delete(f.xml, cathoBaseURL+"/sitemap2/sitemap_vagas_2.xml")
	if _, err := newTestCatho(f).Fetch(context.Background(), CompanyEntry{Provider: "catho"}); err == nil {
		t.Fatal("a failed posting sitemap did not fail the crawl")
	}
}

func TestCathoFailsWhenTheIndexNamesNoPostingSitemap(t *testing.T) {
	f := newCathoFake(t)
	f.xml[cathoSitemapIndex] = "sitemap_vagas_1.xml" // a urlset, so no <sitemap> entries
	if _, err := newTestCatho(f).Fetch(context.Background(), CompanyEntry{Provider: "catho"}); err == nil {
		t.Fatal("an index naming no posting sitemap read as an empty catalogue")
	}
}

func TestCathoSalary(t *testing.T) {
	cases := []struct {
		in     string
		lo, hi int // 0 = nil
		ok     bool
	}{
		{"R$ 1.947", 1947, 1947, true},
		{"R$ 15.000", 15000, 15000, true},
		{"A partir de R$ 2.000,00", 2000, 0, true},
		{"Até R$ 2.000,00", 0, 2000, true},
		{"De R$ 4.001,00 a R$ 5.000,00", 4001, 5000, true},
		{"A Combinar", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		lo, hi, ok := cathoSalary(c.in)
		if ok != c.ok || intOr0(lo) != c.lo || intOr0(hi) != c.hi {
			t.Errorf("cathoSalary(%q) = %v %v %v, want %d %d %v", c.in, intOr0(lo), intOr0(hi), ok, c.lo, c.hi, c.ok)
		}
	}
}

func intOr0(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func TestCathoHiddenEmployer(t *testing.T) {
	for name, want := range map[string]bool{
		"Empresa Confidencial": true, "empresa confidencial": true, "********": true, "": true,
		"CARREFOUR": false, "LISBOA & MORAES CONTABILIDADE": false,
	} {
		if got := cathoHiddenEmployer(name); got != want {
			t.Errorf("cathoHiddenEmployer(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCathoEmploymentType(t *testing.T) {
	for in, want := range map[string]string{
		"CLT (Efetivo)": "", "Estágio": "internship", "Temporário": "contract",
		"Prestador de serviços (PJ)": "contract", "Autônomo": "contract",
		"CLT (Efetivo) Prestador de serviços (PJ)": "",
	} {
		if got := cathoEmploymentType(in); got != want {
			t.Errorf("cathoEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCathoCityDropsTheMoreCitiesNote(t *testing.T) {
	for in, want := range map[string]string{
		"1 vaga - Cachoeirinha + 1 cidade": "Cachoeirinha",
		"3 vagas - São Paulo + 12 cidades": "São Paulo",
		"1 vaga - Eldorado do Sul":         "Eldorado do Sul",
	} {
		root, err := html.Parse(strings.NewReader(`<p><span class="icon i_job_location"></span>` + in + `</p>`))
		if err != nil {
			t.Fatal(err)
		}
		if got := cathoCity(root); got != want {
			t.Errorf("cathoCity(%q) = %q, want %q", in, got, want)
		}
	}
}
