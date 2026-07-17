package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// careerspage adapts careers-page.com career sites. The board is the tenant subdomain
// (e.g. "davidjoseph-co"), forming the host "<board>.careers-page.com". The tenant's
// listing is server-rendered HTML paginated via ?page=N, each job card linking to a
// canonical /jobs/<uuid> detail page whose schema.org JobPosting ld+json carries the
// posting. Description and structured fields come from a per-job detail fetch
// (bounded-concurrency), like the other JSON-LD detail adapters (icims/freshteam).

type careerspage struct {
	http HTMLGetter
}

// NewCareerPage builds the careers-page.com adapter over the given HTML client.
func NewCareerPage(c HTMLGetter) Source { return careerspage{http: c} }

func (careerspage) Provider() string { return "careerspage" }

// careerspageMaxPages caps the ?page=N walk so a listing that never yields an empty/repeat
// page cannot loop forever (the boards seen are ~10 pages; this is ample headroom).
const careerspageMaxPages = 100

// careerspageDetailWorkers throttles the per-board detail fan-out well below the shared
// defaultDetailWorkers (8): careers-page.com rate-limits by request volume, and a wide burst
// trips a 429 window after ~8 requests — starving large boards even through the proxy egress
// (see proxiedProviders). A narrow pool keeps the crawl under the window; the standard client's
// 429-retry backoff absorbs the occasional overshoot, so a full board converges in one run.
const careerspageDetailWorkers = 2

func (s careerspage) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s.careers-page.com/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("careerspage: board %q: %w", e.Board, err)
	}

	// Page through the listing until a page yields no new links (the tail/empty page) or the
	// safety cap is hit. A first-page failure is a board-level error; a later-page failure just
	// stops the walk with what we have.
	locs, err := crawlPagedLinks(ctx, s.http, careerspageMaxPages,
		func(page int) string { return fmt.Sprintf("%s?page=%d", base, page) },
		func(root *html.Node) []string { return careerspageJobLinks(base, root) })
	if err != nil {
		return nil, fmt.Errorf("careerspage: listing %s: %w", e.Board, err)
	}

	// Each posting's fields come from its own detail fetch, fanned out under a narrow pool
	// (careerspageDetailWorkers) to stay under careers-page.com's rate-limit window.
	return fetchDetails(locs, careerspageDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job's detail page and maps its JobPosting ld+json to a Job, returning
// ok=false when the fetch fails or the page carries no JobPosting, so the caller skips just
// that posting.
func (s careerspage) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p careerspagePosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	// No native id → the posting would collide on the (source, external_id) dedup key; drop
	// it. The listing predicate already collects only URLs with an id, so this only fires if
	// the detail URL diverges from that shape.
	id := careerspageJobID(loc)
	if id == "" {
		return Job{}, false
	}

	location := joinNonEmpty(
		p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressRegion,
		p.JobLocation.Address.AddressCountry,
	)

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// careerspagePosting is the schema.org JobPosting decoded from a careers-page.com detail
// page's application/ld+json block. jobLocation is a single Place (unlike iCIMS's array).
type careerspagePosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// careerspageJobIDPattern captures the job UUID from a canonical /jobs/<uuid> URL. The
// match must end at the UUID (end-of-string or a ?/# query/fragment), so the per-card
// /jobs/<uuid>/refer and /apply sub-action links yield no match.
var careerspageJobIDPattern = regexp.MustCompile(`/jobs/([0-9a-fA-F-]{36})(?:$|[?#])`)

// careerspageJobID extracts the native job UUID from a detail URL, or "" when the URL is
// not a canonical job posting.
func careerspageJobID(loc string) string {
	return firstSubmatch(careerspageJobIDPattern, loc)
}

// careerspageJobLinks returns the absolute, deduplicated canonical detail URL of every job
// card on a listing page, resolved against base. The sub-action (/refer, /apply) and
// pagination anchors are excluded by the job-id predicate.
func careerspageJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return careerspageJobID(href) != "" })
}
