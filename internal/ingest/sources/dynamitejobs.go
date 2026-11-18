package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// dynamitejobs adapts Dynamite Jobs (dynamitejobs.com), a remote-only job board across every
// function (sales, marketing, support, engineering, ...), so it is NOT technical-only and sets
// no IsTechHint — the title dictionary judges each posting. Boardless and multi-company; each
// posting's employer comes from its own JobPosting hiringOrganization.
//
// The listing page is a client-rendered SPA whose data API sits under /api/, which robots.txt
// disallows, so the crawl uses what the site publishes for crawlers instead: the sitemaps
// enumerate every open posting's page, and each page server-renders a schema.org JobPosting
// (title, body, posted date, employment type, eligible countries). See the Dynamite Jobs traps
// in AGENTS.md.
type dynamitejobs struct {
	sitemap XMLGetter  // sitemap index + per-section urlsets
	pages   HTMLGetter // per-posting detail pages, rate-paced
	// maxNewPerRun bounds how many new postings one run hydrates; non-positive takes
	// dynamitejobsMaxNewPerRun. The rest are deferred to a later run.
	maxNewPerRun int
}

const (
	dynamitejobsSitemapIndexURL = "https://dynamitejobs.com/sitemap/index.xml"
	// dynamitejobsMaxNewPerRun bounds one run's detail requests. Measured 2026-11-17 the
	// sitemaps list ~620 postings, so the first run fits at the pacer's 1s a page (~10
	// minutes) and later runs hydrate only the handful posted since.
	dynamitejobsMaxNewPerRun = 800
	// dynamitejobsMaxLocationCountries is the longest eligible-country list shown as the
	// location text. Longer ones are the site's region presets (Europe is ~50 countries,
	// Latin America ~100), whose structured Countries carry the geography and whose text
	// would only bury the title.
	dynamitejobsMaxLocationCountries = 5
)

// dynamitejobsTestEmployer is the hiringOrganization of the site's own probe posting ("Pentest
// Probe Job", empty body, no countries), which sits in the sitemaps like any other.
const dynamitejobsTestEmployer = "DJ Audit Probe"

// NewDynamiteJobs builds the Dynamite Jobs adapter: sitemap fetches the sitemaps, pages the
// posting pages (rate-paced, see dynamitejobsRequestInterval).
func NewDynamiteJobs(sitemap XMLGetter, pages HTMLGetter) Source {
	return dynamitejobs{sitemap: sitemap, pages: pages}
}

func (dynamitejobs) Provider() string { return "dynamitejobs" }

func (dynamitejobs) boardless() {}

func (dynamitejobs) aggregator() {}

func (d dynamitejobs) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	return d.FetchNew(ctx, ce, func(string) bool { return false })
}

// FetchNew fetches a posting's page only when the catalogue does not already have it; a stored
// posting yields a liveness-refresh job with no request.
func (d dynamitejobs) FetchNew(ctx context.Context, ce CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	urls, err := d.jobURLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("dynamitejobs: sitemap: %w", err)
	}
	budget := d.maxNewPerRun
	if budget <= 0 {
		budget = dynamitejobsMaxNewPerRun
	}
	var jobs []Job
	var fresh []string
	deferred := 0
	for _, u := range urls {
		id := dynamitejobsJobID(u)
		switch {
		case seen(id):
			jobs = append(jobs, Job{ExternalID: id, URL: u, Company: ce.Company, SeenRefresh: true})
		case len(fresh) < budget:
			fresh = append(fresh, u)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("dynamitejobs: hydration budget %d reached; %d new postings deferred to a later run", budget, deferred)
	}
	hydrated := fetchDetails(fresh, defaultDetailWorkers, func(u string) (Job, bool) {
		return d.detail(ctx, ce, u)
	})
	// A crawl that listed candidates and read none of them failed, whatever it reports:
	// a quiet source is not a broken one, but "some candidates, none read" is.
	if len(fresh) > 0 && len(hydrated) == 0 {
		return nil, fmt.Errorf("dynamitejobs: listed %d new postings, read none", len(fresh))
	}
	return append(jobs, hydrated...), nil
}

// jobURLs walks the sitemap index and returns every distinct posting URL across its
// sub-sitemaps. The company and category sitemaps repeat the same postings (and carry skill and
// category pages, which dynamitejobsJobID filters out), so the union is deduplicated.
func (d dynamitejobs) jobURLs(ctx context.Context) ([]string, error) {
	idx, err := getSitemap(ctx, d.sitemap, dynamitejobsSitemapIndexURL)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var urls []string
	for _, sm := range idx.Sitemaps {
		locs, err := sitemapJobLocs(ctx, d.sitemap, sm.Loc, dynamitejobsJobID)
		if err != nil {
			return nil, err
		}
		for _, u := range locs {
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
	}
	return urls, nil
}

// dynamitejobsJobURLPattern matches a posting page, https://dynamitejobs.com/company/<company>/
// remote-job/<slug>; the sitemaps also list company, skill and category pages.
var dynamitejobsJobURLPattern = regexp.MustCompile(`^https?://dynamitejobs\.com/company/([a-z0-9_-]+)/remote-job/([a-z0-9_-]+)/?$`)

// dynamitejobsJobID is "<company>/<slug>": the slug alone is unique only within a company.
// "" for any other page shape.
func dynamitejobsJobID(u string) string {
	m := dynamitejobsJobURLPattern.FindStringSubmatch(u)
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2]
}

// dynamitejobsPosting is the JobPosting ld+json a posting page carries.
type dynamitejobsPosting struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	HiringOrg      struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	ApplicantLocationRequirements schemaNamedAreas `json:"applicantLocationRequirements"`
}

// detail maps one posting page to a Job. A 404/410 is the site's own "gone"; any other failure,
// or a page with no usable JobPosting, is an unreadableDetail marker — the page is the posting's
// only source.
func (d dynamitejobs) detail(ctx context.Context, ce CompanyEntry, link string) (Job, bool) {
	id := dynamitejobsJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := d.pages.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, ce.Company), true
		}
		return Job{}, false
	}
	var p dynamitejobsPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, link, ce.Company), true
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	title := dynamitejobsTitle(p.Title)
	if company == "" || title == "" {
		return unreadableDetail(id, link, ce.Company), true
	}
	if company == dynamitejobsTestEmployer {
		// The site's own audit probe: it exists to be crawled and is not a vacancy. Dropped,
		// not marked unreadable - the page was read and is not a posting.
		return Job{}, false
	}
	names, countries := dynamitejobsEligibility(p.ApplicantLocationRequirements.Names())
	return Job{
		ExternalID:     id,
		URL:            link,
		Title:          title,
		Company:        company,
		Location:       dynamitejobsLocation(names),
		Description:    sanitizeHTML(p.Description),
		Remote:         true,
		WorkMode:       "remote",
		EmploymentType: dynamitejobsEmploymentType(p.EmploymentType),
		Countries:      countries,
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
	}, true
}

// dynamitejobsTitle drops the ", Remote Job" suffix the site appends to every posting title.
func dynamitejobsTitle(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, ", Remote Job")
	return strings.TrimSpace(t)
}

// dynamitejobsEligibility returns the cleaned country names and their canonical codes. The
// site writes country NAMES; one NormalizeCountry cannot place stays in the names (and so the
// location text) and is left to the location dictionary rather than guessed.
func dynamitejobsEligibility(raw []string) (names, codes []string) {
	for _, n := range raw {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		names = append(names, n)
		if c := location.NormalizeCountry(n); c != "" && !slices.Contains(codes, c) {
			codes = append(codes, c)
		}
	}
	return names, codes
}

// dynamitejobsLocation is the eligible countries when the list is short enough to read, else
// plain "Remote" — the site's region presets expand to dozens of countries.
func dynamitejobsLocation(names []string) string {
	if len(names) == 0 || len(names) > dynamitejobsMaxLocationCountries {
		return "Remote"
	}
	return "Remote - " + strings.Join(names, ", ")
}

// dynamitejobsEmploymentType maps the schema.org enum onto freehire's vocabulary; "" leaves it
// to the description parser. A comma-joined list states several arrangements, so it maps to
// nothing.
func dynamitejobsEmploymentType(t string) string {
	switch strings.TrimSpace(t) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "CONTRACTOR", "CONTRACT", "TEMPORARY":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}
