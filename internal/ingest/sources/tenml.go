package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// tenml adapts 10ML's own careers page (10ml.com/careers), a single-company source with no
// per-tenant board id (boardless). 10ML is a European perfume retailer headquartered in
// Dortmund; it runs its own Shopify storefront and publishes its openings there rather than on
// an ATS, so the careers page is the only place these postings exist.
//
// The page is server-rendered: the listing links every opening as /careers/<slug>, and each
// detail page carries one schema.org JobPosting ld+json block with everything a posting states
// in a structured field — salary (MonetaryAmount), the Dortmund office address, and, for a role
// open to remote work, jobLocationType TELECOMMUTE plus the EU countries it may be done from
// (applicantLocationRequirements). The listing mixes technical and non-technical roles; every
// one is ingested and the pipeline's own category gate decides what reaches search.
type tenml struct {
	http HTMLGetter
}

const (
	tenmlBaseURL    = "https://10ml.com"
	tenmlListingURL = tenmlBaseURL + "/careers"
	tenmlCompany    = "10ML"
)

// tenmlJobPath matches a single-segment /careers/<slug> detail path: the bare listing (which the
// page's language switcher also links) and anything deeper are not postings.
var tenmlJobPath = regexp.MustCompile(`^/careers/([^/?#]+)/?$`)

// NewTenML builds the 10ML adapter over the given HTTP client.
func NewTenML(c HTMLGetter) Source { return tenml{http: c} }

func (tenml) Provider() string { return "10ml" }

// tenml is single-company, so its config entry carries no board.
func (tenml) boardless() {}

func (t tenml) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := t.http.GetHTML(ctx, tenmlListingURL)
	if err != nil {
		return nil, fmt.Errorf("10ml: listing: %w", err)
	}
	base, err := url.Parse(tenmlListingURL)
	if err != nil {
		return nil, fmt.Errorf("10ml: base url: %w", err)
	}
	links := jobLinks(base, root, func(href string) bool { return tenmlSlug(href) != "" })
	// A listing that links nothing is a hard error rather than an empty result, the jettycloud
	// rule: the page carries about a dozen roles, so "no openings" and "the markup moved" look
	// identical in the output, and reading it as the former would let one re-templated page
	// retire the whole employer on the next unseen sweep.
	if len(links) == 0 {
		return nil, fmt.Errorf("10ml: listing links no openings — the markup changed, or the crawl is being refused")
	}
	return fetchDetails(links, defaultDetailWorkers, func(loc string) (Job, bool) {
		return t.detail(ctx, e, loc)
	}), nil
}

// detail fetches one opening and maps its JobPosting ld+json to a Job. The slug is the native
// id (the JobPosting's own identifier.value carries the same string), so a detail that cannot be
// read still names the posting: a 404/410 drops it, while any other failure — or a page that
// stops carrying a JobPosting — yields an unreadableDetail marker instead of reading as a closure.
func (t tenml) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := tenmlSlug(loc)
	if id == "" {
		return Job{}, false
	}
	company := firstNonEmpty(e.Company, tenmlCompany)
	root, err := t.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, company), true
		}
		return Job{}, false
	}
	var p tenmlPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, loc, company), true
	}

	// TELECOMMUTE is the page's structured remote signal; such a role also keeps its Dortmund
	// address (remote within the EU, or the office), so the countries it is open in are the
	// applicant-location list rather than the office's one country.
	remote := strings.EqualFold(strings.TrimSpace(p.JobLocationType), "TELECOMMUTE")
	countries := countryFromCode(p.JobLocation.Address.AddressCountry)
	if remote {
		if c := countriesFromCodes(p.ApplicantLocationRequirements.Names()); len(c) > 0 {
			countries = c
		}
	}
	job := Job{
		ExternalID:     id,
		URL:            firstNonEmpty(strings.TrimSpace(p.URL), loc),
		Title:          strings.TrimSpace(p.Title),
		Company:        firstNonEmpty(strings.TrimSpace(p.HiringOrganization.Name), company),
		Location:       p.JobLocation.Address.Location(),
		Description:    sanitizeHTML(p.Description),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       NotFuture(parseRFC3339OrDate(p.DatePosted)),
		Countries:      countries,
	}
	p.applySalary(&job)
	return job, true
}

// tenmlPosting is the slice of the detail page's JobPosting this adapter reads. validThrough is
// published too but has no counterpart on Job; the unseen sweep closes a withdrawn opening.
type tenmlPosting struct {
	URL                string `json:"url"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation                   schemaPlace      `json:"jobLocation"`
	ApplicantLocationRequirements schemaNamedAreas `json:"applicantLocationRequirements"`
	BaseSalary                    struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// applySalary copies the posting's baseSalary onto the job when it states a currency, a mapped
// period and at least one positive bound; anything less leaves the salary to enrichment.
func (p tenmlPosting) applySalary(j *Job) {
	period := schemaSalaryPeriod(p.BaseSalary.Value.UnitText)
	currency := strings.ToUpper(strings.TrimSpace(p.BaseSalary.Currency))
	lo, hi := roundSalaryPart(p.BaseSalary.Value.MinValue), roundSalaryPart(p.BaseSalary.Value.MaxValue)
	if period == "" || len(currency) != 3 || (lo == nil && hi == nil) {
		return
	}
	j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = lo, hi, currency, period
}

// tenmlSlug returns the opening's slug from a /careers/<slug> link (relative or absolute), or ""
// when the link is not a single-opening page.
func tenmlSlug(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	m := tenmlJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return m[1]
}
