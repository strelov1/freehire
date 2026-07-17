package sources

import (
	"context"
	"fmt"
	"regexp"
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

// sfSitemapEntry is one <url> of the job sitemap: the job page URL and its last-modified
// date (used as posted_at).
type sfSitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

func (s successfactors) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []sfSitemapEntry `xml:"url"`
	}
	url := fmt.Sprintf("https://%s/job_sitemap.xml", e.Board)
	if err := s.http.GetXML(ctx, url, &sitemap); err != nil {
		return nil, fmt.Errorf("successfactors: sitemap %s: %w", e.Board, err)
	}

	// Each job's title and description come from its own page fetch, fanned out under a
	// bounded worker pool.
	return fetchDetails(sitemap.URLs, defaultDetailWorkers, func(entry sfSitemapEntry) (Job, bool) {
		return s.detail(ctx, e, entry)
	}), nil
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page
// fetch fails so the caller can skip just that posting.
func (s successfactors) detail(ctx context.Context, e CompanyEntry, entry sfSitemapEntry) (Job, bool) {
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

	return Job{
		ExternalID: id,
		URL:        entry.Loc,
		Title:      title,
		Company:    e.Company,
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
