package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// vagasHTTP is a route-aware test HTMLGetter. It serves a body per exact URL; an unmapped URL
// yields an empty listing (0 links), which is how a tail page terminates pagination. failURLs
// makes a specific URL error.
type vagasHTTP struct {
	pages    map[string]string
	failURLs map[string]bool
	// The adapter fans its detail fetches out across a worker pool, so this recorder is
	// written from several goroutines at once and needs the lock. The assertions read it
	// after Fetch has joined the pool, which is already ordered.
	mu  sync.Mutex
	got []string
}

func (f *vagasHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if f.failURLs[url] {
		return nil, errors.New("vagasHTTP: boom")
	}
	body, ok := f.pages[url]
	if !ok {
		body = `<html><body></body></html>` // empty listing → stops pagination
	}
	return html.Parse(strings.NewReader(body))
}

// vagasListingHTML renders a one-page listing whose heading states exactly its own cards.
func vagasListingHTML(paths ...string) string { return vagasListingPage(len(paths), paths...) }

// vagasListingPage renders a listing page in the site's markup (scrubbed): the heading states the
// catalogue total, each card's link carries the title and its span the employer.
func vagasListingPage(total int, paths ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<html><body><h1>%d vagas de emprego</h1><div id="todasVagas"><ul>`, total)
	for _, p := range paths {
		b.WriteString(`<li class="vaga odd "><header><h2 class="cargo">` +
			`<a class="link-detalhes-vaga" title="Vaga ` + p + `" href="` + p + `"> Vaga </a></h2>` +
			`<span class="emprVaga"> Employer ` + p + ` </span></header></li>`)
	}
	b.WriteString(`</ul></div></body></html>`)
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
		vagasListingURL(1): vagasListingPage(41,
			"/vagas/v101/analista-de-dados", "/vagas/v102/dev-backend"),
		// The site reshuffles its listing between requests, so page 2 repeats a posting page 1
		// showed; it is read once.
		vagasListingURL(2): vagasListingPage(41,
			"/vagas/v102/dev-backend", "/vagas/v103/dev-frontend"),
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

// vagas's JSON-LD description is a flattened, separator-stripped blob that glues words
// together (headings/list items run into the prose); the page's job-description__text block
// carries the real formatted HTML. The adapter must prefer the block so lists/paragraphs survive.
func TestVagasDetailPrefersStructuredHTMLBlock(t *testing.T) {
	const host = "https://www.vagas.com.br/"
	page := `<html><head>` +
		`<script type="application/ld+json">{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"Dev","datePosted":"2026-06-18",` +
		`"description":"Missao do cargo.REQUISITOS:Item um;Item dois",` +
		`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
		`"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"SP","addressCountry":"Brasil"}}}` +
		`</script></head><body>` +
		`<div class="job-tab-content job-description__text texto">` +
		`<p>Missao do cargo.</p><p><strong>REQUISITOS:</strong></p><ul><li>Item um</li><li>Item dois</li></ul>` +
		`</div></body></html>`
	pages := map[string]string{
		vagasListingURL(1):      vagasListingHTML("/vagas/v900/dev"),
		host + "vagas/v900/dev": page,
	}
	jobs, err := vagas{http: &vagasHTTP{pages: pages}}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	d := jobs[0].Description
	if !strings.Contains(d, "<li>Item um</li>") || !strings.Contains(d, "<li>Item dois</li>") {
		t.Fatalf("description should preserve the HTML list from the block, got: %q", d)
	}
	if strings.Contains(d, "REQUISITOS:Item") {
		t.Fatalf("description still glued from JSON-LD, got: %q", d)
	}
}

// A detail page with no JobPosting is skipped, not turned into a blank job.
func TestVagasDetailWithoutJobPostingSkipped(t *testing.T) {
	const host = "https://www.vagas.com.br/"
	pages := map[string]string{
		vagasListingURL(1):        vagasListingHTML("/vagas/v200/ghost"),
		host + "vagas/v200/ghost": `<html><body>no ld+json</body></html>`,
	}
	jobs, err := vagas{http: &vagasHTTP{pages: pages}}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("want 0 jobs, got %+v", jobs)
	}
}

// The first listing page failing is a board-level error.
func TestVagasFirstPageFailure(t *testing.T) {
	http := &vagasHTTP{failURLs: map[string]bool{vagasListingURL(1): true}}
	if _, err := (vagas{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when the first listing page fails")
	}
}

// A later listing page failing costs that page alone: the walk goes on to the pages after it.
func TestVagasLaterPageFailureCostsOnlyThatPage(t *testing.T) {
	const host = "https://www.vagas.com.br/"
	http := &vagasHTTP{
		pages: map[string]string{
			vagasListingURL(1):    vagasListingPage(120, "/vagas/v101/a"),
			vagasListingURL(3):    vagasListingPage(120, "/vagas/v103/c"),
			host + "vagas/v101/a": vagasDetailHTML("A", "Acme", "Recife", "PE", "Brasil", "2026-10-20", "Body."),
			host + "vagas/v103/c": vagasDetailHTML("C", "Acme", "Recife", "PE", "Brasil", "2026-10-20", "Body."),
		},
		failURLs: map[string]bool{vagasListingURL(2): true},
	}
	jobs, err := vagas{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want the postings of pages 1 and 3, got %+v", jobs)
	}
}

// A page that adds nothing new is not the end of a reshuffling listing: the walk runs to the
// page count the heading states.
func TestVagasWalksPastAPageWithNothingNew(t *testing.T) {
	http := &vagasHTTP{pages: map[string]string{
		vagasListingURL(1): vagasListingPage(120, "/vagas/v101/a"),
		vagasListingURL(2): vagasListingPage(120, "/vagas/v101/a"),
		vagasListingURL(3): vagasListingPage(120, "/vagas/v103/c"),
	}}
	jobs, err := vagas{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want both distinct postings, got %+v", jobs)
	}
	for _, u := range http.got {
		if u == vagasListingURL(4) {
			t.Error("walked past the page count the heading states")
		}
	}
}

// A first page that states no total is a layout change, not an empty catalogue.
func TestVagasListingWithoutTotalFails(t *testing.T) {
	http := &vagasHTTP{pages: map[string]string{
		vagasListingURL(1): `<html><body><li class="vaga"><a class="link-detalhes-vaga" href="/vagas/v1/a">A</a></li></body></html>`,
	}}
	if _, err := (vagas{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when page 1 states no total")
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
	g, ok := pacedHTMLGetter(inner, vagasRequestInterval, vagasRequestBurst).(rateLimitedHTMLGetter)
	if !ok {
		t.Fatal("pacedHTMLGetter must return a rate-limited getter")
	}
	if g.inner != HTMLGetter(inner) {
		t.Fatal("pacedHTMLGetter must wrap the given getter")
	}
	if g.limiter == nil {
		t.Fatal("pacedHTMLGetter must install a limiter")
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

// vagas re-fetched the detail page of EVERY posting on every run: 1 939 pages at the pacer's
// one request a second, which through the proxy overran its 50-minute budget and so never
// finished, never recorded success, and let its whole catalogue go stale (last success
// 2026-08-18, measured 2026-09-07). A posting the catalogue already has needs no second read.
// vagasFake serves listing and detail pages and counts what was asked for, so a test can assert
// that a page was NOT fetched.
type vagasFake struct {
	mu    sync.Mutex
	pages map[string]string
	asked map[string]int
}

func (f *vagasFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[url]++
	body, ok := f.pages[url]
	f.mu.Unlock()
	if !ok {
		return nil, errors.New("vagasFake: no page for " + url)
	}
	return html.Parse(strings.NewReader(body))
}

func TestVagasSkipsDetailForAlreadyIngestedPostings(t *testing.T) {
	listing := vagasListingHTML("/vagas/v1234567/dev-backend", "/vagas/v7654321/dev-frontend")
	page := `<html><body><script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",
"title":"Dev","hiringOrganization":{"@type":"Organization","name":"Acme"},"description":"<p>Body.</p>"}</script></body></html>`

	http := &vagasFake{pages: map[string]string{}}
	http.pages[vagasListingURL(1)] = listing
	http.pages["https://www.vagas.com.br/vagas/v1234567/dev-backend"] = page
	http.pages["https://www.vagas.com.br/vagas/v7654321/dev-frontend"] = page

	// v1234567 is already ingested; only v7654321 may cost a detail request.
	jobs, err := NewVagas(http).(vagas).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "1234567" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want both postings back (one hydrated, one refreshed), got %d", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "1234567" && !j.SeenRefresh {
			t.Error("the already-ingested posting was hydrated again instead of refreshed")
		}
		if j.ExternalID == "7654321" && j.SeenRefresh {
			t.Error("the new posting was refreshed instead of hydrated")
		}
	}
	if got := http.asked["https://www.vagas.com.br/vagas/v1234567/dev-backend"]; got != 0 {
		t.Errorf("the seen posting's detail page was fetched %d times, want 0", got)
	}
}

// The pipeline only offers a seen-set to a source that declares itself hydrating, so the
// interface assertion is what actually turns the change on.
func TestVagasIsAHydratingSource(t *testing.T) {
	var s any = NewVagas(&vagasFake{pages: map[string]string{}})
	if _, ok := s.(interface {
		FetchNew(context.Context, CompanyEntry, func(string) bool) ([]Job, error)
	}); !ok {
		t.Error("vagas does not satisfy the hydrating shape; the pipeline will keep re-reading every posting")
	}
}

// Measured 2026-10-22: only 2 of the 40 postings on the first listing page were stored, so a
// run faced ~1 200 new postings at about two seconds a detail page through the proxy, overran
// its 50-minute budget, and — the board writes only after the fetch — stored none of them. The
// next run faced the same backlog. A per-run budget is what lets the backlog shrink.
func TestVagasHydratesAtMostTheBudgetOfNewPostings(t *testing.T) {
	listing := vagasListingHTML("/vagas/v1000001/a", "/vagas/v1000002/b", "/vagas/v1000003/c", "/vagas/v1000004/d")
	page := `<html><body><script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",
"title":"Dev","hiringOrganization":{"@type":"Organization","name":"Acme"},"description":"<p>Body.</p>"}</script></body></html>`

	http := &vagasFake{pages: map[string]string{}}
	http.pages[vagasListingURL(1)] = listing
	for _, p := range []string{"v1000001/a", "v1000002/b", "v1000003/c", "v1000004/d"} {
		http.pages["https://www.vagas.com.br/vagas/"+p] = page
	}

	// One posting is already stored; of the three new ones the budget admits two.
	jobs, err := vagas{http: http, maxNewPerRun: 2}.FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "1000001" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var refreshed, hydrated int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		} else {
			hydrated++
		}
	}
	if refreshed != 1 || hydrated != 2 {
		t.Fatalf("got %d refreshed and %d hydrated, want 1 and 2", refreshed, hydrated)
	}
	details := 0
	for url, n := range http.asked {
		if strings.Contains(url, "/vagas/v") {
			details += n
		}
	}
	if details != 2 {
		t.Errorf("fetched %d detail pages, want exactly the budget of 2", details)
	}
}

// A posting the pipeline will discard is never stored, so never seen: without the gate it would
// take a hydration slot on every run. The listing card names its employer and title, so it is
// yielded body-less and costs no detail request.
func TestVagasGateSkipsDetailForDiscardedPostings(t *testing.T) {
	const covered, rejected, kept = "/vagas/v1/covered", "/vagas/v2/rejected", "/vagas/v3/kept"
	page := `<html><body><script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",
"title":"Dev","hiringOrganization":{"@type":"Organization","name":"Acme"},"description":"<p>Body.</p>"}</script></body></html>`
	http := &vagasFake{pages: map[string]string{vagasListingURL(1): vagasListingHTML(covered, rejected, kept)}}
	for _, p := range []string{covered, rejected, kept} {
		http.pages["https://www.vagas.com.br"+p] = page
	}
	gate := DetailGate{
		Covered: func(companies []string) map[string]bool {
			return map[string]bool{"Employer " + covered: true}
		},
		Rejected: func(j Job) bool { return j.Title == "Vaga "+rejected },
	}
	jobs, err := NewVagas(http).(vagas).FetchNewGated(context.Background(), CompanyEntry{},
		func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want all three postings yielded, got %d", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID != "3" && (j.Description != "" || j.Company == "") {
			t.Errorf("gated posting %s: want body-less with its listing company, got %+v", j.ExternalID, j)
		}
	}
	for _, p := range []string{covered, rejected} {
		if n := http.asked["https://www.vagas.com.br"+p]; n != 0 {
			t.Errorf("%s detail fetched %d times, want 0", p, n)
		}
	}
	if http.asked["https://www.vagas.com.br"+kept] != 1 {
		t.Errorf("the kept posting was not hydrated")
	}
}
