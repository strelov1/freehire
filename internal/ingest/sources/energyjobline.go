package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// energyjobline adapts EnergyJobline (www.energyjobline.com), an energy-sector job-portal
// aggregator (oil & gas is one of its categories). It is boardless — one sitemap index
// covers the whole site, no per-tenant board id — and an aggregator: each posting's
// employer comes from its own schema.org JobPosting hiringOrganization, the same
// resolution bayt.go/gulftalent.go use. For agency/recruiter-submitted postings that
// value is the agency's own brand (e.g. "Energy Jobline ZR") rather than a confidential
// end client — verified against the live site's own human-visible company link, not just
// its ld+json, so it is stored as-is rather than filtered or guessed around.
//
// The sitemap and the detail pages need different transports (see crawlerUserAgent and
// energyjoblineRequestInterval), so, like clinch, the two are separate parameters rather
// than one combined client.
type energyjobline struct {
	sitemap XMLGetter  // the top-level sitemap index + per-page urlsets (4 requests/run)
	pages   HTMLGetter // per-posting detail pages: Googlebot UA, rate-paced
}

const energyjoblineSitemapIndexURL = "https://www.energyjobline.com/sitemap.xml"

// NewEnergyJobline builds the EnergyJobline adapter: sitemap fetches the site's sitemap
// index, pages fetches each job-detail page (see crawlerUserAgent, pacedHTMLGetter).
func NewEnergyJobline(sitemap XMLGetter, pages HTMLGetter) Source {
	return energyjobline{sitemap: sitemap, pages: pages}
}

func (energyjobline) Provider() string { return "energyjobline" }

// energyjobline is single-sitemap, so its config entry carries no board.
func (energyjobline) boardless() {}

// aggregator documents that one crawl aggregates postings from many companies (the
// employer comes from each posting, not the configured entry).
func (energyjobline) aggregator() {}

func (e energyjobline) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	urls, err := e.jobURLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("energyjobline: sitemap: %w", err)
	}
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return e.detail(ctx, ce, u)
	}), nil
}

// jobURLs resolves the top-level sitemap index to its paginated sub-sitemaps (each a flat
// urlset mixing job postings with the site's other pages, e.g. news articles) and returns
// every job-detail URL found across all of them.
func (e energyjobline) jobURLs(ctx context.Context) ([]string, error) {
	idx, err := getSitemap(ctx, e.sitemap, energyjoblineSitemapIndexURL)
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, sm := range idx.Sitemaps {
		locs, err := sitemapJobLocs(ctx, e.sitemap, sm.Loc, energyjoblineJobID)
		if err != nil {
			return nil, err
		}
		urls = append(urls, locs...)
	}
	return urls, nil
}

// detail fetches one job-detail page and maps its JobPosting ld+json to a Job. A URL with
// no extractable id, an unreadable fetch, a missing JobPosting block, or an empty
// hiringOrganization all return an unreadableDetail stub rather than a dropped posting or
// a guessed employer — the posting's existence is still proven, just with no usable
// catalogue entry.
func (e energyjobline) detail(ctx context.Context, ce CompanyEntry, link string) (Job, bool) {
	id := energyjoblineJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := e.pages.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, ce.Company), true
		}
		return Job{}, false
	}
	var p energyjoblinePosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, link, ce.Company), true
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	if company == "" {
		return unreadableDetail(id, link, ce.Company), true
	}
	location := joinNonEmpty(
		strings.TrimSpace(p.jobLocationAddress().Locality),
		strings.TrimSpace(p.jobLocationAddress().Country),
	)
	return Job{
		ExternalID: id,
		URL:        link,
		Title:      strings.TrimSpace(p.Title),
		Company:    company,
		Location:   location,
		// EnergyJobline's description carries HTML-entity-encoded markup (e.g. literal
		// "&amp;" inside the JSON string, confirmed on a live posting), so it needs
		// unescaping before sanitizeHTML — unlike bayt.go/gulftalent.go, whose descriptions
		// arrive already decoded.
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// energyjoblineJobURLPattern captures the trailing numeric id from a canonical job-detail
// URL (https://www.energyjobline.com/job/<slug>-<id>), excluding the listing root
// (/jobs), the site root, and unrelated pages (e.g. /news-article/..., /company/...).
var energyjoblineJobURLPattern = regexp.MustCompile(`^https?://(?:www\.)?energyjobline\.com/job/[a-z0-9-]+-(\d+)/?$`)

// energyjoblineJobID extracts the job id from a canonical job-detail URL, returning "" for
// any other page shape on the site.
func energyjoblineJobID(u string) string {
	return firstSubmatch(energyjoblineJobURLPattern, u)
}

// energyjoblinePosting is the schema.org JobPosting decoded from an EnergyJobline
// job-detail page's ld+json.
type energyjoblinePosting struct {
	Title       string               `json:"title"`
	Description string               `json:"description"`
	DatePosted  string               `json:"datePosted"`
	HiringOrg   energyjoblineOrg     `json:"hiringOrganization"`
	JobLocation []energyjoblinePlace `json:"jobLocation"`
}

type energyjoblineOrg struct {
	Name string `json:"name"`
}

type energyjoblinePlace struct {
	Address energyjoblineAddress `json:"address"`
}

type energyjoblineAddress struct {
	Locality string `json:"addressLocality"`
	Country  string `json:"addressCountry"`
}

// jobLocationAddress returns the first jobLocation's address, or a zero value when the
// posting carries none — EnergyJobline's postings each carry exactly one, but the field is
// an array in the markup, so this guards the empty case rather than indexing directly.
func (p energyjoblinePosting) jobLocationAddress() energyjoblineAddress {
	if len(p.JobLocation) == 0 {
		return energyjoblineAddress{}
	}
	return p.JobLocation[0].Address
}
