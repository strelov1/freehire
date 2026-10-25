package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// gemeentebanen adapts Gemeentebanen (www.gemeentebanen.nl), the job site of the Dutch
// municipalities and their joint bodies (omgevingsdiensten, regional social services), ~600
// postings at a time. It is boardless and an aggregator — each municipality is named per posting.
// See the "Dutch public boards" section of AGENTS.md for the measurements behind each rule.
//
// The posting set is one sitemap, which names only each posting's URL; the body is the posting
// page's schema.org JobPosting, so the adapter hydrates only the postings the catalogue lacks.
type gemeentebanen struct {
	http         gemeentebanenHTTP
	limiter      waiter
	maxNewPerRun int
}

// gemeentebanenHTTP is the transport the adapter needs: the sitemap as XML and each posting page
// with the URL it finally landed on, since a closed posting redirects rather than answering 404.
type gemeentebanenHTTP interface {
	XMLGetter
	HTMLResolvedGetter
}

const gemeentebanenSitemapURL = "https://www.gemeentebanen.nl/sitemap-vacancies.xml"

// gemeentebanenRequestInterval paces every request of a run through one limiter; one a second is
// courtesy, not a measured limit.
const gemeentebanenRequestInterval = time.Second

const gemeentebanenDetailWorkers = 2

// gemeentebanenMaxNewPerRun bounds how many unseen postings one run hydrates: 1,000 pages at one
// a second is ~17 minutes, which takes the whole first catalogue (~600) in one run.
const gemeentebanenMaxNewPerRun = 1000

// NewGemeentebanen builds the adapter over the given client.
func NewGemeentebanen(c gemeentebanenHTTP) Source {
	return gemeentebanen{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(gemeentebanenRequestInterval), 1),
		maxNewPerRun: gemeentebanenMaxNewPerRun,
	}
}

func (gemeentebanen) Provider() string { return "gemeentebanen" }

// One national catalogue, no per-tenant id.
func (gemeentebanen) boardless() {}

// Hundreds of municipalities post here, each named per posting.
func (gemeentebanen) aggregator() {}

// gemeentebanenPostingPath matches a posting URL, /vacatures/<id>/<employer-slug>/<title-slug>.
// The slugs are decoration, so the id is the identity.
var gemeentebanenPostingPath = regexp.MustCompile(`^https://www\.gemeentebanen\.nl/vacatures/(\d+)/`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s gemeentebanen) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored and hydrates
// up to maxNewPerRun of the rest, newest id first.
func (s gemeentebanen) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return hydrateListed(postings, seen, s.maxNewPerRun, gemeentebanenDetailWorkers, func(p listedPosting) (Job, bool) {
		return s.detail(ctx, p)
	}), nil
}

// list reads the posting sitemap and returns its postings, newest id first. The sitemap also
// names the listing page itself, which is no posting.
func (s gemeentebanen) list(ctx context.Context) ([]listedPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.http, gemeentebanenSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("gemeentebanen: sitemap: %w", err)
	}
	return numericListing(doc, gemeentebanenPostingPath), nil
}

// detail fetches one posting page. A closed posting redirects to the listing page; the redirect
// is the verdict, whatever the landing page holds.
func (s gemeentebanen) detail(ctx context.Context, p listedPosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, final, err := s.http.GetHTMLResolved(ctx, p.url)
	if err != nil || firstSubmatch(gemeentebanenPostingPath, final) != p.id {
		return Job{}, false
	}
	return gemeentebanenDetail(root, p)
}

// gemeentebanenLD is the part of the posting page's schema.org JobPosting the adapter reads.
// employmentType is deliberately absent: it is FULL_TIME on every sampled posting, 32-hour ones
// included, so it is the form's default rather than a statement.
type gemeentebanenLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces          `json:"jobLocation"`
	BaseSalary  *schemaMonetaryAmount `json:"baseSalary"`
}

// gemeentebanenDetail maps a posting page to a Job. ok is false for a page carrying no posting.
func gemeentebanenDetail(root *xhtml.Node, p listedPosting) (Job, bool) {
	var ld gemeentebanenLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if company == "" {
		return Job{}, false
	}
	// addressRegion is a province abbreviation ("NB", "ZH") the location dictionary would not
	// resolve; the city already places the posting.
	var city, country string
	if len(ld.JobLocation) > 0 {
		city = strings.TrimSpace(ld.JobLocation[0].Address.AddressLocality)
		country = strings.TrimSpace(ld.JobLocation[0].Address.AddressCountry)
	}
	j := Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       strings.TrimSpace(ld.Title),
		Company:     company,
		Location:    city,
		Description: sanitizeHTML(ld.Description),
		PostedAt:    parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
	}
	if j.Countries = countryFromCode(country); j.Countries == nil {
		j.FallbackCountries = []string{"nl"}
	}
	ld.BaseSalary.apply(&j, "EUR")
	return j, true
}
