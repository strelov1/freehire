package sources

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// werkenVoorNederland adapts Werken voor Nederland (www.werkenvoornederland.nl), the job site of
// the Dutch central government: every ministry, agency and the armed forces publish their
// vacancies and internships there, ~1,400 at a time. It is boardless and an aggregator — the
// employing organisation is read per posting. See the "Dutch public boards" section of
// AGENTS.md for the measurements behind each rule.
//
// The posting set is one sitemap, which names only each posting's URL, so every fact is bought
// with the posting page: the adapter hydrates only the postings the catalogue lacks.
type werkenVoorNederland struct {
	http         werkenVoorNederlandHTTP
	limiter      waiter
	maxNewPerRun int
}

type werkenVoorNederlandHTTP interface {
	XMLGetter
	HTMLGetter
}

const wvnSitemapURL = "https://www.werkenvoornederland.nl/sitemap-vacatures.xml"

// wvnRequestInterval paces every request of a run through one limiter. robots.txt asks for at
// most ten a second; one a second is courtesy toward a government site, not a measured limit.
const wvnRequestInterval = time.Second

// wvnDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
const wvnDetailWorkers = 2

// wvnMaxNewPerRun bounds how many unseen postings one run hydrates: 1,200 pages at one a second
// is 20 minutes. The first run faces the whole catalogue (~1,400), so it takes two; after that a
// run meets the ~40 postings a day publishes.
const wvnMaxNewPerRun = 1200

// NewWerkenVoorNederland builds the adapter over the given client.
func NewWerkenVoorNederland(c werkenVoorNederlandHTTP) Source {
	return werkenVoorNederland{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(wvnRequestInterval), 1),
		maxNewPerRun: wvnMaxNewPerRun,
	}
}

func (werkenVoorNederland) Provider() string { return "werkenvoornederland" }

// One national catalogue, no per-tenant id.
func (werkenVoorNederland) boardless() {}

// Hundreds of government organisations post here, each named per posting.
func (werkenVoorNederland) aggregator() {}

// wvnPostingCode matches the reference that ends every posting path, /vacatures/<slug>-<code>,
// where the code is the organisation's abbreviation, the year and a number (CJIB-2026-9570,
// DEF2644-2005-6416). The slug is the title and changes with it, so the code is the identity.
var wvnPostingCode = regexp.MustCompile(`^https://www\.werkenvoornederland\.nl/vacatures/[^/?#]+-([A-Za-z]+[0-9]*-[0-9]{4}-[0-9]+)/?$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s werkenVoorNederland) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored and hydrates
// up to maxNewPerRun of the rest, most recently modified first.
func (s werkenVoorNederland) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return hydrateListed(postings, seen, s.maxNewPerRun, wvnDetailWorkers, func(p listedPosting) (Job, bool) {
		return s.detail(ctx, p)
	}), nil
}

// list reads the posting sitemap. Its lastmod is the only date it carries, so it orders the
// hydration; the codes are not chronological across organisations.
func (s werkenVoorNederland) list(ctx context.Context) ([]listedPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.http, wvnSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("werkenvoornederland: sitemap: %w", err)
	}
	locs := doc.URLs
	sort.SliceStable(locs, func(i, j int) bool { return locs[i].LastMod > locs[j].LastMod })
	ids := make(map[string]bool)
	var out []listedPosting
	for _, u := range locs {
		id := firstSubmatch(wvnPostingCode, strings.TrimSpace(u.Loc))
		if id == "" || ids[id] {
			continue
		}
		ids[id] = true
		out = append(out, listedPosting{id: id, url: strings.TrimSpace(u.Loc)})
	}
	return out, nil
}

// detail fetches one posting page. A closed posting answers 404 and is dropped by the failed
// fetch.
func (s werkenVoorNederland) detail(ctx context.Context, p listedPosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	return wvnDetail(root, p)
}

// wvnLD is the part of the posting page's schema.org JobPosting the adapter reads. Its
// description is a one-line summary ("<title> in <city> voor 32-36 uur bij <organisation>"); the
// body is read from the page (wvnDescription).
type wvnLD struct {
	Title              string `json:"title"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     any    `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces          `json:"jobLocation"`
	BaseSalary  *schemaMonetaryAmount `json:"baseSalary"`
}

// wvnDetail maps a posting page to a Job. ok is false for a page carrying no posting.
func wvnDetail(root *xhtml.Node, p listedPosting) (Job, bool) {
	var ld wvnLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if company == "" {
		return Job{}, false
	}
	var city, country string
	if len(ld.JobLocation) > 0 {
		// A locality may carry the office's street in brackets, "Den Haag (Rijnstraat 8)".
		city, _, _ = strings.Cut(ld.JobLocation[0].Address.AddressLocality, "(")
		city = strings.TrimSpace(city)
		country = strings.TrimSpace(ld.JobLocation[0].Address.AddressCountry)
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          strings.TrimSpace(ld.Title),
		Company:        company,
		Location:       city,
		Description:    wvnDescription(root),
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: schemaScheduleType(ld.EmploymentType),
		IsTechHint:     wvnTechField(wvnVakgebied(root)),
	}
	if j.Countries = countryFromCode(country); j.Countries == nil {
		j.FallbackCountries = []string{"nl"}
	}
	ld.BaseSalary.apply(&j, "EUR")
	return j, true
}

// wvnTechFields are the vakgebied (field) values that name IT work. A posting may list several
// fields ("ICT,Financieel / economisch"); naming one of these is the site's own classification.
var wvnTechFields = map[string]bool{"ICT": true, "Development en testing": true}

func wvnTechField(fields string) bool {
	for _, f := range strings.Split(fields, ",") {
		if wvnTechFields[strings.TrimSpace(f)] {
			return true
		}
	}
	return false
}

// wvnVakgebiedField matches the field in the analytics object the page pushes
// (window.dataLayer.push({'Vakgebied': 'ICT', ...})) — the only place the posting's field is
// stated as data rather than as a rendered label.
var wvnVakgebiedField = regexp.MustCompile(`'Vakgebied':\s*'([^']*)'`)

func wvnVakgebied(root *xhtml.Node) string {
	var value string
	walk(root, func(n *xhtml.Node) bool {
		if value != "" {
			return false
		}
		if n.Type == xhtml.ElementNode && n.Data == "script" {
			if text := textContent(n); strings.Contains(text, "dataLayer.push") {
				value = firstSubmatch(wvnVakgebiedField, text)
			}
			return false
		}
		return true
	})
	return value
}

// wvnContactSection is the section naming the recruiter and hiring manager, with their phone
// numbers. It is a person's contact card, not part of the posting, and is never copied.
const wvnContactSection = "stel_gerust_je_vraag"

// wvnDescription builds the body from the page's sections ("Dit ga je doen", "Dit vragen wij",
// "Hier kom je te werken", ...): each one's heading and article text. A section without article
// text is a block of rendered facts (the salary scale card) that the structured fields already
// carry.
func wvnDescription(root *xhtml.Node) string {
	var b strings.Builder
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "section" {
			return true
		}
		if Attr(n, "id") == wvnContactSection {
			return false
		}
		article := firstByClass(n, "s-article-content")
		if article == nil {
			return false
		}
		if h := firstByTag(n, "h2"); h != nil {
			if heading := strings.TrimSpace(textContent(h)); heading != "" {
				b.WriteString("<h2>" + html.EscapeString(heading) + "</h2>")
			}
		}
		for c := article.FirstChild; c != nil; c = c.NextSibling {
			b.WriteString(renderNode(c))
		}
		return false
	})
	return sanitizeHTML(b.String())
}
