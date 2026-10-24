package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// catho adapts Catho (www.catho.com.br), Brazil's largest general job board: ~214k public
// postings from tens of thousands of employers, almost none of them IT. It is boardless and an
// aggregator — the employer is read per posting. See the "Catho traps" section of AGENTS.md for
// the measurements behind each rule.
//
// The posting set comes from the job sitemaps (robots.txt disallows the search, /buscar/vagas/),
// which carry nothing but each posting's URL, so every fact is bought with the posting page:
// the adapter hydrates only postings the catalogue lacks and refreshes the rest by identity.
type catho struct {
	http         cathoHTTP
	limiter      waiter
	maxNewPerRun int
}

// cathoHTTP is the transport the adapter needs: the sitemaps as XML and each posting page with
// the URL it finally landed on, since a closed posting redirects rather than answering 404.
type cathoHTTP interface {
	XMLGetter
	HTMLResolvedGetter
}

const (
	cathoBaseURL      = "https://www.catho.com.br"
	cathoSitemapIndex = cathoBaseURL + "/sitemap-index.xml"
	// cathoSitemapNeedle selects the posting sitemaps among the index's entries; the rest list
	// search, profession and article pages.
	cathoSitemapNeedle = "/sitemap2/sitemap_vagas_"
)

// cathoRequestInterval paces every request of a run, sitemap and posting page alike, through
// one limiter. A posting page answered in ~0.75 s from the crawl host and 52 pages at ~1/s drew
// no refusal; two a second is courtesy toward a large site, not a limit we measured.
const cathoRequestInterval = 500 * time.Millisecond

// cathoDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
const cathoDetailWorkers = 3

// cathoMaxNewPerRun bounds how many unseen postings one run hydrates: 3000 pages at two a second
// is ~25 minutes, inside the 50-minute run timeout with room for a slow origin. The first runs
// face the whole ~214k catalogue, so it is taken in slices, newest first; a posting past the
// bound is not stored this run and is hydrated by a later one.
const cathoMaxNewPerRun = 3000

// cathoConfidential is the label Catho shows when the employer chose to stay hidden (~16% of
// sampled postings). It is a placeholder, not a company, so such a posting is dropped rather
// than filed under one bogus employer collecting every anonymous posting on the site.
const cathoConfidential = "Empresa Confidencial"

// NewCatho builds the adapter over the given client (see compatibleUASource for why it is not
// the default one).
func NewCatho(c cathoHTTP) Source {
	return catho{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(cathoRequestInterval), 1),
		maxNewPerRun: cathoMaxNewPerRun,
	}
}

func (catho) Provider() string { return "catho" }

// One national board, no per-tenant id.
func (catho) boardless() {}

// Tens of thousands of employers post here, each named per posting.
func (catho) aggregator() {}

// cathoPosting is one sitemap entry: the posting's numeric id and its canonical URL.
type cathoPosting struct {
	id  int64
	url string
}

func (p cathoPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// cathoPostingPath matches a posting URL, /vagas/<slug>/<id>/. The slug is cosmetic — the site
// serves the posting under any slug — so the id alone is the identity.
var cathoPostingPath = regexp.MustCompile(`^/vagas/[^/]+/(\d+)/?$`)

// cathoPostingID returns the posting id a URL carries, or 0 when it is not a posting URL.
func cathoPostingID(raw string) int64 {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	m := cathoPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s catho) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemaps name, refreshes the ones already stored and hydrates
// up to maxNewPerRun of the rest, newest id first so a backlog never delays a fresh posting. A
// posting whose page cannot be read is not yielded: nothing is stored for it yet, so a later
// run simply tries again.
func (s catho) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []cathoPosting
	var refreshed []Job
	for _, p := range postings {
		if seen(p.externalID()) {
			refreshed = append(refreshed, Job{ExternalID: p.externalID(), URL: p.url, SeenRefresh: true})
			continue
		}
		if len(unseen) < s.maxNewPerRun {
			unseen = append(unseen, p)
		}
	}
	jobs := fetchDetails(unseen, cathoDetailWorkers, func(p cathoPosting) (Job, bool) {
		return s.detail(ctx, p)
	})
	return append(jobs, refreshed...), nil
}

// list reads every posting sitemap the index names and returns their postings, newest id first.
// Any sitemap failing fails the crawl: a short list would hydrate a skewed slice and leave the
// postings it missed un-refreshed.
func (s catho) list(ctx context.Context) ([]cathoPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	index, err := getSitemap(ctx, s.http, cathoSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("catho: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []cathoPosting
	files := 0
	for _, sm := range index.Sitemaps {
		if !strings.Contains(sm.Loc, cathoSitemapNeedle) {
			continue
		}
		files++
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		doc, err := getSitemap(ctx, s.http, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("catho: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			id := cathoPostingID(u.Loc)
			if id == 0 || ids[id] {
				continue
			}
			ids[id] = true
			out = append(out, cathoPosting{id: id, url: u.Loc})
		}
	}
	if files == 0 {
		return nil, fmt.Errorf("catho: sitemap index names no %s file", cathoSitemapNeedle)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// detail fetches one posting page. A page that redirected away from the posting is the site's
// answer that the posting closed (it lands on the search page for the slug) and is dropped by
// that answer, not by whatever the landing page happens to hold: today it carries no ld+json,
// but a JobPosting block there would describe some other posting.
func (s catho) detail(ctx context.Context, p cathoPosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, final, err := s.http.GetHTMLResolved(ctx, p.url)
	if err != nil || cathoPostingID(final) != p.id {
		return Job{}, false
	}
	return cathoDetail(root, p)
}

// cathoLD is the part of the posting page's schema.org JobPosting the adapter reads. Its
// baseSalary is deliberately absent: it is a coarse band that contradicts the figure the page
// itself shows (see cathoSalary).
type cathoLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation []struct {
		Address struct {
			AddressRegion  string `json:"addressRegion"`
			AddressCountry string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// cathoDetail maps a posting page to a Job. ok is false for a page carrying no posting and for
// one whose employer is hidden.
func cathoDetail(root *xhtml.Node, p cathoPosting) (Job, bool) {
	var ld cathoLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if cathoHiddenEmployer(company) {
		return Job{}, false
	}
	var region, country string
	if len(ld.JobLocation) > 0 {
		region = strings.TrimSpace(ld.JobLocation[0].Address.AddressRegion)
		country = strings.TrimSpace(ld.JobLocation[0].Address.AddressCountry)
	}
	j := Job{
		ExternalID:     p.externalID(),
		URL:            p.url,
		Title:          strings.TrimSpace(ld.Title),
		Company:        company,
		Location:       joinNonEmpty(cathoCity(root), region, country),
		Description:    cathoDescription(root, ld.Description),
		PostedAt:       cathoDate(ld.DatePosted),
		EmploymentType: cathoEmploymentType(ld.EmploymentType),
	}
	if j.Countries = countryFromCode(country); j.Countries == nil {
		j.FallbackCountries = []string{"br"}
	}
	if lo, hi, ok := cathoSalary(cathoSalaryText(root)); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = lo, hi, "BRL", "month"
	}
	return j, true
}

// cathoHiddenEmployer reports whether the employer name is Catho's placeholder for a hidden one:
// the "Empresa Confidencial" label, or a name the employer masked as a run of asterisks.
func cathoHiddenEmployer(name string) bool {
	return strings.EqualFold(name, cathoConfidential) || strings.Trim(name, "* ") == ""
}

// cathoCity reads the first city from the posting header ("1 vaga - Eldorado do Sul"). The ld+json
// cannot supply it: its addressLocality holds the STATE's name ("Rio Grande do Sul").
func cathoCity(root *xhtml.Node) string {
	icon := firstByClass(root, "i_job_location")
	if icon == nil || icon.Parent == nil {
		return ""
	}
	line := strings.Join(strings.Fields(textContent(icon.Parent)), " ")
	if _, city, ok := strings.Cut(line, " - "); ok {
		return strings.TrimSpace(cathoMoreCities.ReplaceAllString(city, ""))
	}
	return ""
}

// cathoMoreCities is the header's note that a posting is open in further cities
// ("Cachoeirinha + 1 cidade"), which the page names nowhere else.
var cathoMoreCities = regexp.MustCompile(`\s*\+\s*\d+\s+cidades?\s*$`)

// cathoDescription builds the body: the posting's text, one paragraph per line the employer
// wrote, then the facts the page lists under it (contract type, working hours) and the benefits.
// The text comes from the ld+json, which keeps the employer's line breaks as \n; the page's own
// copy of it is one whitespace-pre-line paragraph that HTML would collapse.
func cathoDescription(root *xhtml.Node, text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString("<p>" + html.EscapeString(line) + "</p>")
		}
	}
	if about := cathoSection(root, "Sobre a vaga"); about != nil {
		for c := about.FirstChild; c != nil; c = c.NextSibling {
			// The heading row and the text paragraph are already covered above.
			if c.Type != xhtml.ElementNode || c.Data == "div" || HasClass(c, "whitespace-pre-line") {
				continue
			}
			b.WriteString(renderNode(c))
		}
	}
	if pay := cathoSection(root, "Salário"); pay != nil {
		if ul := firstByTag(pay, "ul"); ul != nil {
			b.WriteString("<p><strong>Benefícios</strong></p>" + renderNode(ul))
		}
	}
	return sanitizeHTML(b.String())
}

// cathoSection returns the box whose <h2> reads heading. "Sobre a vaga" sits in a header row
// inside its box, "Salário" directly in it, so the box is the nearest ancestor that is not that
// row.
func cathoSection(root *xhtml.Node, heading string) *xhtml.Node {
	var box *xhtml.Node
	walk(root, func(n *xhtml.Node) bool {
		if box != nil {
			return false
		}
		if n.Type == xhtml.ElementNode && n.Data == "h2" && strings.TrimSpace(textContent(n)) == heading {
			box = n.Parent
			if box != nil && HasClass(box, "flex") {
				box = box.Parent
			}
			return false
		}
		return true
	})
	return box
}

func renderNode(n *xhtml.Node) string {
	var b strings.Builder
	if err := xhtml.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}

// cathoDate reads datePosted, which states a DAY as its last second ("2026-10-23T23:59:59Z");
// the day is kept, at its start, so a posting published today is never dated in the future.
func cathoDate(s string) *time.Time {
	if len(s) < 10 {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return nil
	}
	return &t
}

// cathoEmploymentType maps Catho's contract regime. Only values with a lossless counterpart are
// mapped: "CLT (Efetivo)" is a permanent hire under the labour code and says nothing about
// hours, and a posting offering two regimes ("CLT (Efetivo) Prestador de serviços (PJ)")
// names none.
func cathoEmploymentType(s string) string {
	switch strings.TrimSpace(s) {
	case "Estágio":
		return "internship"
	case "Prestador de serviços (PJ)", "Temporário", "Autônomo":
		return "contract"
	}
	return ""
}

// cathoSalaryText reads the salary the posting header shows ("A partir de R$ 2.000,00").
func cathoSalaryText(root *xhtml.Node) string {
	icon := firstByClass(root, "i_salary")
	if icon == nil || icon.Parent == nil {
		return ""
	}
	if strong := firstByTag(icon.Parent, "strong"); strong != nil {
		return strings.Join(strings.Fields(textContent(strong)), " ")
	}
	return ""
}

var cathoMoney = regexp.MustCompile(`R\$\s*([\d.]+)(?:,\d{2})?`)

// cathoSalary parses the header's monthly salary: an exact figure ("R$ 1.947"), a floor
// ("A partir de R$ 2.000,00"), a ceiling ("Até R$ 2.000,00") or a range ("De R$ 4.001,00 a
// R$ 5.000,00"). "A Combinar" (negotiable) and anything else yields nothing.
func cathoSalary(s string) (lo, hi *int, ok bool) {
	var vals []int
	for _, m := range cathoMoney.FindAllStringSubmatch(s, -1) {
		v, err := strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
		if err != nil || v <= 0 {
			return nil, nil, false
		}
		vals = append(vals, v)
	}
	switch {
	case len(vals) == 2 && strings.HasPrefix(s, "De "):
		return &vals[0], &vals[1], true
	case len(vals) != 1:
		return nil, nil, false
	case strings.HasPrefix(s, "A partir de "):
		return &vals[0], nil, true
	case strings.HasPrefix(s, "Até "):
		return nil, &vals[0], true
	case strings.HasPrefix(s, "R$"):
		return &vals[0], &vals[0], true
	}
	return nil, nil, false
}
