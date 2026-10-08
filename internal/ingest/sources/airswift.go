package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// airswift adapts Airswift (airswift.com), an international oil & gas / engineering
// recruitment agency. Boardless: one flat sitemap enumerates the whole catalogue. NOT an
// aggregator — hiringOrganization.name is confirmed live to be the literal string
// "Airswift" on every posting (standard recruitment-agency anonymization), so every
// posting is attributed to the agency itself.
//
// Roughly 55-60% of the sitemap's entries are expired postings that still answer
// HTTP 200 with no JobPosting block — the page instead renders a
// "c-jobs-article-expired" element ("...we are no longer accepting applicants.",
// confirmed live). detail treats that as a confirmed closure and drops the posting
// outright, the same category as a 404/410, rather than an unreadableDetail stub:
// marking that large a fraction of every crawl Unreadable would permanently withhold
// the board's stale-job close (see fetchDetails/Unreadable's documented contract).
//
// The sitemap and the detail pages need different transports (see
// airswiftRequestInterval — the detail path 429s under an unpaced burst), so, like
// clinch and energyjobline, the two are separate parameters rather than one combined
// client.
type airswift struct {
	sitemap XMLGetter  // the sitemap.xml (one request per run)
	pages   HTMLGetter // per-posting detail pages, rate-paced
}

const (
	airswiftSitemapURL = "https://www.airswift.com/sitemap.xml"
	// airswiftCompany is the employer for every posting — hiringOrganization.name is
	// confirmed live to be this literal string on every posting, not a real end client,
	// so this is set as a constant rather than read from the configured entry or the
	// posting's own (uninformative) hiringOrganization field.
	airswiftCompany = "Airswift"
)

// NewAirswift builds the Airswift adapter: sitemap fetches the site's sitemap.xml,
// pages fetches each job-detail page (see airswiftRequestInterval, pacedHTMLGetter).
func NewAirswift(sitemap XMLGetter, pages HTMLGetter) Source {
	return airswift{sitemap: sitemap, pages: pages}
}

func (airswift) Provider() string { return "airswift" }

// airswift is single-sitemap, so its config entry carries no board.
func (airswift) boardless() {}

func (a airswift) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	urls, err := sitemapJobLocs(ctx, a.sitemap, airswiftSitemapURL, airswiftJobID)
	if err != nil {
		return nil, fmt.Errorf("airswift: sitemap: %w", err)
	}
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return a.detail(ctx, ce, u)
	}), nil
}

// detail fetches one job-detail page and maps its JobPosting ld+json to a Job.
//
// Decision order: no extractable id → drop; a confirmed-gone 404/410 → drop; any other
// fetch error → unreadableDetail; a confirmed-expired page (the
// c-jobs-article-expired marker) → drop; a page with neither the expired marker nor a
// JobPosting block → unreadableDetail (genuinely ambiguous, unlike the expired case);
// JobPosting present → map normally.
func (a airswift) detail(ctx context.Context, ce CompanyEntry, link string) (Job, bool) {
	id := airswiftJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := a.pages.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, ce.Company), true
		}
		return Job{}, false
	}
	if airswiftExpired(root) {
		return Job{}, false
	}
	var p airswiftPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, link, ce.Company), true
	}
	location := joinNonEmpty(
		strings.TrimSpace(p.jobLocationAddress().Locality),
		strings.TrimSpace(p.jobLocationAddress().Country),
	)
	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       strings.TrimSpace(p.Title),
		Company:     airswiftCompany,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// airswiftExpired reports whether the page renders Airswift's own confirmed-closure
// marker: an element whose class attribute contains "c-jobs-article-expired".
func airswiftExpired(root *html.Node) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && strings.Contains(Attr(n, "class"), "c-jobs-article-expired") {
			found = true
			return false
		}
		return true
	})
	return found
}

// airswiftJobURLPattern captures the numeric id from a canonical job-detail URL
// (https://www.airswift.com/jobs/<slug>-<id>), excluding the listing root and
// unrelated pages.
var airswiftJobURLPattern = regexp.MustCompile(`^https?://(?:www\.)?airswift\.com/jobs/[a-z0-9-]+-(\d+)/?$`)

// airswiftJobID extracts the job id from a canonical job-detail URL, returning "" for
// any other page shape on the site.
func airswiftJobID(u string) string {
	return firstSubmatch(airswiftJobURLPattern, u)
}

// airswiftPosting is the schema.org JobPosting decoded from an Airswift job-detail
// page's ld+json.
type airswiftPosting struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	DatePosted  string          `json:"datePosted"`
	JobLocation []airswiftPlace `json:"jobLocation"`
}

type airswiftPlace struct {
	Address airswiftAddress `json:"address"`
}

type airswiftAddress struct {
	Locality string `json:"addressLocality"`
	Country  string `json:"addressCountry"`
}

// jobLocationAddress returns the first jobLocation's address, or a zero value when the
// posting carries none.
func (p airswiftPosting) jobLocationAddress() airswiftAddress {
	if len(p.JobLocation) == 0 {
		return airswiftAddress{}
	}
	return p.JobLocation[0].Address
}
