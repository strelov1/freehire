package sources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// vagasHTTP is a route-aware test HTMLGetter. It serves a body per exact URL; an unmapped URL
// yields an empty listing (0 links), which is how a tail page terminates pagination. failURLs
// makes a specific URL error.
type vagasHTTP struct {
	pages    map[string]string
	failURLs map[string]bool
	got      []string
}

func (f *vagasHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.got = append(f.got, url)
	if f.failURLs[url] {
		return nil, errors.New("vagasHTTP: boom")
	}
	body, ok := f.pages[url]
	if !ok {
		body = `<html><body></body></html>` // empty listing → stops pagination
	}
	return html.Parse(strings.NewReader(body))
}

// vagasListingHTML links each given path from a listing page.
func vagasListingHTML(paths ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul>`)
	for _, p := range paths {
		b.WriteString(`<li><a href="` + p + `">vaga</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// vagasDetailHTML builds a job page carrying a JobPosting ld+json block (plus a decoy WebSite
// block, as the real page has two ld+json scripts).
func vagasDetailHTML(title, company, locality, region, country, datePosted, desc string) string {
	return `<html><head>` +
		`<script type="application/ld+json">{"@type":"WebSite","name":"Vagas"}</script>` +
		`<script type="application/ld+json">{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `","description":"` + desc + `","datePosted":"` + datePosted + `",` +
		`"hiringOrganization":{"@type":"Organization","name":"` + company + `"},` +
		`"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressRegion":"` + region + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

func TestVagasFetch(t *testing.T) {
	const host = "https://www.vagas.com.br/"
	pages := map[string]string{
		"https://www.vagas.com.br/vagas-de-tecnologia?pagina=1": vagasListingHTML(
			"/vagas/v101/analista-de-dados", "/vagas/v102/dev-backend"),
		"https://www.vagas.com.br/vagas-de-programador?pagina=1": vagasListingHTML(
			"/vagas/v102/dev-backend", "/vagas/v103/dev-frontend"), // 102 is a cross-area duplicate
		host + "vagas/v101/analista-de-dados": vagasDetailHTML(
			"Analista de Dados", "Acme", "São Paulo", "SP", "Brasil", "2026-06-17", "Full description here."),
		host + "vagas/v102/dev-backend": vagasDetailHTML(
			"Dev Backend", "Beta", "Rio de Janeiro", "RJ", "Brasil", "2026-06-18", "Backend role."),
		host + "vagas/v103/dev-frontend": vagasDetailHTML(
			"Dev Frontend", "Gamma", "Remoto", "", "Brasil", "2026-06-19", "Frontend role."),
	}
	http := &vagasHTTP{pages: pages}

	jobs, err := vagas{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want 3 de-duped jobs, got %d: %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j101, ok := byID["101"]
	if !ok {
		t.Fatalf("missing job 101; got %+v", jobs)
	}
	if j101.Title != "Analista de Dados" || j101.Company != "Acme" {
		t.Fatalf("101 identity: %+v", j101)
	}
	if j101.URL != host+"vagas/v101/analista-de-dados" {
		t.Fatalf("101 url: %q", j101.URL)
	}
	if j101.Location != "São Paulo, SP, Brasil" {
		t.Fatalf("101 location: %q", j101.Location)
	}
	if j101.PostedAt == nil {
		t.Fatalf("101 posted_at not parsed")
	}
	if !strings.Contains(j101.Description, "Full description here") {
		t.Fatalf("101 description: %q", j101.Description)
	}
}

// A detail page with no JobPosting is skipped, not turned into a blank job.
func TestVagasDetailWithoutJobPostingSkipped(t *testing.T) {
	const host = "https://www.vagas.com.br/"
	pages := map[string]string{
		"https://www.vagas.com.br/vagas-de-tecnologia?pagina=1": vagasListingHTML("/vagas/v200/ghost"),
		host + "vagas/v200/ghost":                               `<html><body>no ld+json</body></html>`,
	}
	jobs, err := vagas{http: &vagasHTTP{pages: pages}}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("want 0 jobs, got %+v", jobs)
	}
}

// The first area's listing failing is a board-level error.
func TestVagasFirstAreaFailure(t *testing.T) {
	http := &vagasHTTP{failURLs: map[string]bool{
		"https://www.vagas.com.br/vagas-de-tecnologia?pagina=1": true,
	}}
	if _, err := (vagas{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when the first area listing fails")
	}
}

func TestVagasIsProxied(t *testing.T) {
	// vagas.com.br 403s the prod datacenter IP on the very first listing request, so vagas
	// must egress through the proxy (a residential IP is served the full HTML).
	if _, ok := proxiedProviders["vagas"]; !ok {
		t.Error("vagas must be in proxiedProviders (its edge 403s the prod datacenter IP)")
	}
}

func TestPacedVagasGetterPaces(t *testing.T) {
	// vagas 429s the single proxy IP under an unpaced burst, so its crawl must go through a
	// rate limiter (mirrors careerspage). Guard that the wrapper actually paces, not passes raw.
	inner := &recordingHTMLGetter{node: &html.Node{}}
	g, ok := pacedVagasGetter(inner).(rateLimitedHTMLGetter)
	if !ok {
		t.Fatal("pacedVagasGetter must return a rate-limited getter")
	}
	if g.inner != HTMLGetter(inner) {
		t.Fatal("pacedVagasGetter must wrap the given getter")
	}
	if g.limiter == nil {
		t.Fatal("pacedVagasGetter must install a limiter")
	}
}

func TestVagasJobID(t *testing.T) {
	cases := map[string]string{
		"/vagas/v2820917/assistente-ti":           "2820917",
		"https://www.vagas.com.br/vagas/v101/dev": "101",
		"/empresa/acme":                           "",
		"/vagas/cadastro":                         "",
	}
	for in, want := range cases {
		if got := vagasJobID(in); got != want {
			t.Errorf("vagasJobID(%q) = %q, want %q", in, got, want)
		}
	}
}
