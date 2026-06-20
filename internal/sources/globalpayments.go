package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// globalpayments adapts the Global Payments careers site (jobs.globalpayments.com): a
// server-rendered site whose /en/jobs listing enumerates postings as /en/jobs/<id>/<slug>
// links and whose job pages carry a schema.org JobPosting ld+json block. The board is the
// career-site host; the description comes from a per-job detail fetch (bounded-concurrency),
// like the other ld+json detail adapters (teamtailor, breezy). It exposes no jobLocationType,
// so the remote flag falls back to the location text.
type globalpayments struct {
	http HTMLGetter
}

// NewGlobalPayments builds the Global Payments adapter over the given HTTP client.
func NewGlobalPayments(c HTMLGetter) Source { return globalpayments{http: c} }

func (globalpayments) Provider() string { return "globalpayments" }

// gpMaxPages bounds listing pagination so a board that clamps ?page=N to its last page
// (serving the same links forever) cannot loop; the no-new-links check ends it sooner.
const gpMaxPages = 100

func (g globalpayments) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries scheme+host; the listing's relative /en/jobs hrefs resolve against it
	// (an absolute href resolves to itself), so it is parsed once rather than per page.
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("globalpayments: board %q: %w", e.Board, err)
	}

	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= gpMaxPages; page++ {
		listURL := fmt.Sprintf("https://%s/en/jobs/?page=%d", e.Board, page)
		root, err := g.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("globalpayments: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		// Stop on the first page that adds no new links: an empty page, or a board that
		// clamps ?page=N past its last page (de-dup turns the repeat into zero new).
		newLinks := 0
		for _, link := range gpJobLinks(base, root) {
			if !seen[link] {
				seen[link] = true
				urls = append(urls, link)
				newLinks++
			}
		}
		if newLinks == 0 {
			break
		}
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return g.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (g globalpayments) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := gpJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := g.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p gpPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.location()
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// No jobLocationType is emitted, so the location text is the only remote signal
		// (never the title, which false-positives on "Remote …" role names).
		Remote:   isRemote(location),
		PostedAt: parseRFC3339(p.DatePosted),
	}, true
}

// gpJobLinks returns the absolute hrefs of all anchors linking a /en/jobs/<id>/<slug> job
// page, resolved against base (the listing URL) so relative hrefs still yield fetchable
// URLs, de-duplicated in first-seen order (a card links the same job from its title and
// other controls). A link is a job exactly when it carries a parseable native id.
func gpJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return gpJobID(href) != "" })
}

// gpJobIDPattern captures the posting id from a job URL's /en/jobs/<id>/<slug> path. The
// trailing /<slug> segment is required so the bare /en/jobs/ listing (and its ?page=N
// variants) is not mistaken for a job.
var gpJobIDPattern = regexp.MustCompile(`/en/jobs/([^/?#]+)/[^/?#]+`)

// gpJobID extracts the native posting id (e.g. "r0072212") from a job page URL.
func gpJobID(u string) string {
	if m := gpJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// gpPosting is the schema.org JobPosting decoded from a Global Payments job page's
// application/ld+json block. Its jobLocation entries nest address as an ARRAY (unlike
// Teamtailor's single object), and a Place/address carries a ready "City, Region, Country"
// name used as a fallback when the structured parts are absent.
type gpPosting struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DatePosted  string    `json:"datePosted"`
	JobLocation []gpPlace `json:"jobLocation"`
}

type gpPlace struct {
	Name    string      `json:"name"`
	Address []gpAddress `json:"address"`
}

type gpAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressRegion   string `json:"addressRegion"`
	AddressCountry  string `json:"addressCountry"`
}

// location builds the display location from the first jobLocation's first address
// (city, region, country), falling back to the Place's pre-formatted name when the
// structured parts are empty.
func (p gpPosting) location() string {
	if len(p.JobLocation) == 0 {
		return ""
	}
	pl := p.JobLocation[0]
	if len(pl.Address) > 0 {
		a := pl.Address[0]
		if loc := joinNonEmpty(a.AddressLocality, a.AddressRegion, a.AddressCountry); loc != "" {
			return loc
		}
	}
	return pl.Name
}
