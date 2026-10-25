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

// academicTransfer adapts AcademicTransfer (www.academictransfer.com), where the Dutch
// universities, university medical centres and research institutes publish their academic and
// support positions, ~770 at a time. It is boardless and an aggregator — the institution is named
// per posting. See the "Dutch public boards" section of AGENTS.md for the measurements behind
// each rule.
//
// The posting set is one sitemap, which names only each posting's URL; the body is the posting
// page's schema.org JobPosting, so the adapter hydrates only the postings the catalogue lacks.
type academicTransfer struct {
	http         academicTransferHTTP
	limiter      waiter
	maxNewPerRun int
}

type academicTransferHTTP interface {
	XMLGetter
	HTMLGetter
}

const academicTransferSitemapURL = "https://www.academictransfer.com/sitemap-vacancies.xml"

// academicTransferRequestInterval is the site's robots.txt Crawl-delay, honoured for every
// request of a run, sitemap included.
const academicTransferRequestInterval = 10 * time.Second

// academicTransferMaxNewPerRun bounds how many unseen postings one run hydrates: 240 pages at the
// crawl delay is 40 minutes, inside the 50-minute run timeout. The first runs face the whole
// catalogue (~770), so it takes four; after that a run meets the few dozen a day publishes.
const academicTransferMaxNewPerRun = 240

// NewAcademicTransfer builds the adapter over the given client.
func NewAcademicTransfer(c academicTransferHTTP) Source {
	return academicTransfer{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(academicTransferRequestInterval), 1),
		maxNewPerRun: academicTransferMaxNewPerRun,
	}
}

func (academicTransfer) Provider() string { return "academictransfer" }

// One national catalogue, no per-tenant id.
func (academicTransfer) boardless() {}

// Universities, medical centres and institutes post here, each named per posting.
func (academicTransfer) aggregator() {}

// academicTransferPostingPath matches a posting URL, /<lang>/jobs/<id>/<slug>/. A posting is
// listed once, under the language it was written in; the slug is decoration.
var academicTransferPostingPath = regexp.MustCompile(`^https://www\.academictransfer\.com/(?:en|nl)/jobs/(\d+)/`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s academicTransfer) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored and hydrates
// up to maxNewPerRun of the rest, newest id first.
func (s academicTransfer) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.http, academicTransferSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("academictransfer: sitemap: %w", err)
	}
	postings := numericListing(doc, academicTransferPostingPath)
	// One worker: the crawl delay, not latency, sets the pace.
	return hydrateListed(postings, seen, s.maxNewPerRun, 1, func(p listedPosting) (Job, bool) {
		return s.detail(ctx, p)
	}), nil
}

// detail fetches one posting page. A closed posting answers 404 and is dropped by the failed
// fetch.
func (s academicTransfer) detail(ctx context.Context, p listedPosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	return academicTransferDetail(root, p)
}

// academicTransferLD is the part of the posting's schema.org JobPosting the adapter reads; the
// page nests it as the mainEntity of a WebPage.
type academicTransferLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     any    `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces          `json:"jobLocation"`
	BaseSalary  *schemaMonetaryAmount `json:"baseSalary"`
}

// academicTransferDetail maps a posting page to a Job. ok is false for a page carrying no
// posting.
func academicTransferDetail(root *xhtml.Node, p listedPosting) (Job, bool) {
	var ld academicTransferLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if company == "" {
		return Job{}, false
	}
	var city, country string
	if len(ld.JobLocation) > 0 {
		city = strings.TrimSpace(ld.JobLocation[0].Address.AddressLocality)
		country = strings.TrimSpace(ld.JobLocation[0].Address.AddressCountry)
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          strings.TrimSpace(ld.Title),
		Company:        company,
		Location:       city,
		Description:    sanitizeHTML(ld.Description),
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: schemaScheduleType(ld.EmploymentType),
	}
	if j.Countries = countryFromCode(country); j.Countries == nil {
		j.FallbackCountries = []string{"nl"}
	}
	ld.BaseSalary.apply(&j, "EUR")
	return j, true
}
