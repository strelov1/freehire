package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// luxoft adapts Luxoft's careers site (career.luxoft.com): a server-rendered site whose
// /jobs listing enumerates postings as /jobs/<slug>-<id> links and whose job pages carry a
// schema.org JobPosting ld+json block. The board is the career-site host; the description
// comes from a per-job detail fetch (bounded-concurrency), like the other ld+json detail
// adapters (globalpayments, teamtailor). Its /ajax/filter-jobs endpoint is unusable (it
// caps at 100 and ignores paging), so the paginated HTML listing is the enumeration path.
type luxoft struct {
	http HTMLGetter
}

// NewLuxoft builds the Luxoft adapter over the given HTTP client.
func NewLuxoft(c HTMLGetter) Source { return luxoft{http: c} }

func (luxoft) Provider() string { return "luxoft" }

// lxMaxPages bounds listing pagination so a board that clamps ?page=N to its last page
// (serving the same links forever) cannot loop; the no-new-links check ends it sooner.
const lxMaxPages = 100

// lxPerPage requests the largest page size Luxoft's listing offers, to keep the number of
// listing fetches low (~1400 jobs / 60 ≈ 24 pages).
const lxPerPage = 60

func (l luxoft) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries scheme+host; the listing's relative /jobs hrefs resolve against it
	// (an absolute href resolves to itself), so it is parsed once rather than per page.
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("luxoft: board %q: %w", e.Board, err)
	}

	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= lxMaxPages; page++ {
		// page=1 is the default; Luxoft 301-redirects /jobs?page=1 to a broken relative
		// Location that resolves to a 404, so omit the page param on the first page and add
		// it only from page 2 on.
		listURL := fmt.Sprintf("https://%s/jobs?perPage=%d", e.Board, lxPerPage)
		if page > 1 {
			listURL += fmt.Sprintf("&page=%d", page)
		}
		root, err := l.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("luxoft: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		// Stop on the first page that adds no new links: an empty page, or a board that
		// clamps ?page=N past its last page (de-dup turns the repeat into zero new).
		newLinks := 0
		for _, link := range lxJobLinks(base, root) {
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
		return l.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the URL has no parseable id, the fetch fails, or the page carries no JobPosting, so
// the caller skips just that posting.
func (l luxoft) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := lxJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := l.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p lxPosting
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
		// Luxoft emits no jobLocationType, so the location text is the only remote signal.
		Remote: isRemote(location),
		// Luxoft stamps some postings with a future datePosted; parseLayout drops a future
		// time to nil (NotFuture), so the pipeline falls back to ingest time.
		PostedAt: parseLayout("2006-01-02 15:04:05", p.DatePosted),
	}, true
}

// lxJobLinks returns the absolute hrefs of all anchors linking a /jobs/<slug>-<id> job page,
// resolved against base (the listing URL) so relative hrefs still yield fetchable URLs,
// de-duplicated in first-seen order (a card links the same job more than once). A link is a
// job exactly when it carries a parseable native id (the trailing numeric slug segment).
func lxJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return lxJobID(href) != "" })
}

// lxJobIDPattern captures the posting id from a job URL's /jobs/<slug>-<id> path. The slug
// segment and a trailing -<digits> are both required, so the bare /jobs listing and its
// ?page=N / ?perPage=N controls (which share the /jobs path) are not mistaken for jobs.
var lxJobIDPattern = regexp.MustCompile(`/jobs/[^/?#]*-(\d+)(?:[/?#]|$)`)

// lxJobID extracts the native posting id (the trailing numeric slug segment, e.g. "25262")
// from a job page URL.
func lxJobID(u string) string {
	if m := lxJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// lxPosting is the schema.org JobPosting decoded from a Luxoft job page's ld+json block.
// Unlike Global Payments, its jobLocation is a single Place object whose address is a single
// PostalAddress object (not arrays).
type lxPosting struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	DatePosted  string  `json:"datePosted"`
	JobLocation lxPlace `json:"jobLocation"`
}

type lxPlace struct {
	Name    string    `json:"name"`
	Address lxAddress `json:"address"`
}

type lxAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressRegion   string `json:"addressRegion"`
	AddressCountry  string `json:"addressCountry"`
}

// location builds the display location from the address (city, region, country), falling
// back to the Place's pre-formatted name when the structured parts are empty.
func (p lxPosting) location() string {
	a := p.JobLocation.Address
	if loc := joinNonEmpty(a.AddressLocality, a.AddressRegion, a.AddressCountry); loc != "" {
		return loc
	}
	return p.JobLocation.Name
}
