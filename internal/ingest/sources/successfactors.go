package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// successfactors adapts SAP SuccessFactors career sites. The board is the career-site
// host (e.g. "jobs.tetrapak.com"). The site's job sitemap enumerates the postings; each
// job page is server-rendered HTML carrying schema.org JobPosting microdata, so the
// description comes from a per-job detail fetch (bounded-concurrency), like the other
// detail-fetching adapters.
// successfactorsHTTP is the transport successfactors needs: an XML sitemap plus HTML
// detail pages.
type successfactorsHTTP interface {
	XMLGetter
	HTMLGetter
}

type successfactors struct {
	http successfactorsHTTP
}

// NewSuccessFactors builds the SuccessFactors adapter over the given HTTP client.
func NewSuccessFactors(c successfactorsHTTP) Source { return successfactors{http: c} }

func (successfactors) Provider() string { return "successfactors" }

func (s successfactors) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	sitemapURL := fmt.Sprintf("https://%s/job_sitemap.xml", e.Board)
	sitemap, err := getSitemap(ctx, s.http, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("successfactors: sitemap %s: %w", e.Board, err)
	}

	// Each job's title and description come from its own page fetch, fanned out under a
	// bounded worker pool; the sitemap's lastmod is carried as posted_at.
	return fetchDetails(sitemap.URLs, defaultDetailWorkers, func(entry sitemapLoc) (Job, bool) {
		return s.detail(ctx, e, entry)
	}), nil
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page
// fetch fails so the caller can skip just that posting.
func (s successfactors) detail(ctx context.Context, e CompanyEntry, entry sitemapLoc) (Job, bool) {
	id := sfJobID(entry.Loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}

	root, err := s.http.GetHTML(ctx, entry.Loc)
	if err != nil {
		return Job{}, false
	}

	title := itempropText(root, "title")
	if title == "" {
		title = metaProperty(root, "og:title")
	}

	// On a hub the configured company is only the hub's own name; the employer is the tenant
	// the job URL names, resolved through the entry's curated map. An ordinary board keeps its
	// configured company, whatever segments its URLs happen to carry.
	company := e.Company
	if e.Hub {
		if name, ok := e.Tenants[sfTenant(entry.Loc)]; ok {
			company = name
		}
	}

	return Job{
		ExternalID: id,
		URL:        entry.Loc,
		Title:      title,
		Company:    company,
		// Location is intentionally empty: SuccessFactors does not expose it in the
		// microdata, and enrichment derives it from the description.
		Location:    "",
		Description: sanitizeHTML(itempropHTML(root, "description")),
		Remote:      isRemote(title),
		PostedAt:    parseDate(entry.LastMod),
	}, true
}

// sfJobIDPattern captures the leading digits of a job URL's last path segment, ignoring a
// trailing locale suffix (e.g. ".../98012-en_GB" → "98012", ".../12345/" → "12345").
var sfJobIDPattern = regexp.MustCompile(`/(\d+)(?:-[^/]*)?/?$`)

// sfJobID extracts the native numeric posting id from a job page URL.
func sfJobID(loc string) string {
	return firstSubmatch(sfJobIDPattern, loc)
}

// sfTenant returns the hub tenant a job URL belongs to: the first path segment, which is how a
// SuccessFactors hub identifies the employer behind a posting when nothing else on the page
// does. It returns "" when the URL names no tenant — a bare host or an unparseable URL, or a
// path opening on "job", the platform's own word for a posting. That last one is the tenant-less
// shape a hub sitemap also lists, and it is every URL on an ordinary single-tenant site, where
// the segment follows the host instead of a tenant.
func sfTenant(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	segment, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	if segment == "job" {
		return ""
	}
	return segment
}
