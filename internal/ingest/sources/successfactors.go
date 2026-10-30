package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/net/html"
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

// fullBoardListing: Fetch is a single sitemap fetch with no pagination loop — it either
// succeeds with the whole listing or fails outright. See the fullBoardListing interface for
// the bar.
func (successfactors) fullBoardListing() {}

func (s successfactors) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold.
// The page is this adapter's whole cost — the sitemap names a posting and carries nothing
// else — and re-reading every stored page on every run could not finish: measured
// 2026-10-28, each of the three shards' 2400-second crawls reached ~350 of its ~850 boards,
// every run for at least ten days ended on its deadline, and the eight boards in flight when
// it did (jobs.lidl alone lists 21k postings) saved nothing. A seen posting is re-listed as a
// SeenRefresh carrying the sitemap's identity — the sitemap itself proves the posting is
// still published — so its liveness refreshes and its hydrated body and facets are kept.
// The sitemap stays whole, so the board-scoped sweep still closes what it stopped listing.
func (s successfactors) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	sitemapURL := fmt.Sprintf("https://%s/job_sitemap.xml", e.Board)
	sitemap, err := getSitemap(ctx, s.http, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("successfactors: sitemap %s: %w", e.Board, err)
	}

	// Each new job's title and description come from its own page fetch, fanned out under a
	// bounded worker pool; the sitemap's lastmod is carried as posted_at.
	return fetchDetails(sitemap.URLs, defaultDetailWorkers, func(entry sitemapLoc) (Job, bool) {
		if id := sfJobID(entry.Loc); id != "" && seen(id) {
			return Job{
				ExternalID:  id,
				URL:         entry.Loc,
				Company:     sfCompany(e, entry.Loc),
				SeenRefresh: true,
			}, true
		}
		return s.detail(ctx, e, entry)
	}), nil
}

// detail fetches one job page and maps it to a Job. A page the platform reports gone is
// dropped (ok=false); a page this crawl merely could not read comes back as an
// unreadableDetail marker, since the page is this adapter's only source for the posting and a
// dropped one is indistinguishable from a posting taken down.
func (s successfactors) detail(ctx context.Context, e CompanyEntry, entry sitemapLoc) (Job, bool) {
	id := sfJobID(entry.Loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}

	// Resolved from the URL before the fetch, so an unreadable posting can still name the
	// employer whose close scope it withholds.
	company := sfCompany(e, entry.Loc)

	root, err := s.http.GetHTML(ctx, entry.Loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, entry.Loc, company), true
		}
		return Job{}, false
	}

	title := itempropText(root, "title")
	if title == "" {
		title = metaProperty(root, "og:title")
	}

	place, countries := sfLocation(root)
	return Job{
		ExternalID:  id,
		URL:         entry.Loc,
		Title:       title,
		Company:     company,
		Location:    place,
		Countries:   countries,
		Description: sanitizeHTML(itempropHTML(root, "description")),
		Remote:      isRemote(title),
		PostedAt:    parseDate(entry.LastMod),
	}, true
}

// sfLocation reads where a posting is from its page. The JobPosting's jobLocation microdata
// carries either a structured address (addressLocality/addressCountry) or one streetAddress
// string; the standard template also renders the place as a jobGeoLocation line, which is
// the most complete form (it spells out the region), so it is preferred when present.
// addressRegion is never used: the platform truncates it to four characters ("Québ",
// "II d"). The structured addressCountry is returned as the posting's country, since in a
// free-text line "CA" reads as California rather than Canada. A tenant whose custom layout
// drops both the microdata and the rendered line states no place, and gets none.
func sfLocation(root *html.Node) (string, []string) {
	country := itempropAttr(root, "addressCountry")

	rendered := ""
	if n := firstByClass(root, "jobGeoLocation"); n != nil {
		rendered = textContent(n)
	}
	place := firstNonEmpty(
		rendered,
		itempropAttr(root, "streetAddress"),
		joinNonEmpty(itempropAttr(root, "addressLocality"), country),
	)

	var countries []string
	if c := location.NormalizeCountry(country); c != "" {
		countries = []string{c}
	}
	return place, countries
}

// sfJobIDPattern captures the leading digits of a job URL's last path segment, ignoring a
// trailing locale suffix (e.g. ".../98012-en_GB" → "98012", ".../12345/" → "12345").
var sfJobIDPattern = regexp.MustCompile(`/(\d+)(?:-[^/]*)?/?$`)

// sfJobID extracts the native numeric posting id from a job page URL.
func sfJobID(loc string) string {
	return firstSubmatch(sfJobIDPattern, loc)
}

// sfCompany is the employer behind one posting. On a hub the configured company is only the
// hub's own name; the employer is the tenant the job URL names, resolved through the entry's
// curated map. An ordinary board keeps its configured company, whatever segments its URLs
// happen to carry.
func sfCompany(e CompanyEntry, loc string) string {
	if e.Hub {
		if name, ok := e.Tenants[sfTenant(loc)]; ok {
			return name
		}
	}
	return e.Company
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
