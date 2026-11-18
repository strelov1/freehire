package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	// workdayPageLimit is Workday's max listing page size.
	workdayPageLimit = 20
	// workdayCapTotal is the `total` some Workday tenants report once a board exceeds this
	// size — past offset=2000 those tenants silently loop back to page 1 instead of erroring
	// or returning an empty page. A board whose total is exactly this must be walked by facet
	// (see listPostings), not paged directly.
	workdayCapTotal = 2000
	// maxFacetDepth bounds how many facet dimensions can be combined while subdividing a
	// capped board; raise it if a board's exhaustion log shows it is not enough.
	maxFacetDepth = 3
	// workdaySliceParallelism bounds how many facet-slice listing requests one capped board has
	// in flight at once, across every level of its split. A capped board is split by its finest
	// dimension, often one value per store (over a thousand requests); serially that eats a
	// sharded run's whole budget. The 403/429 retry absorbs a refusal.
	workdaySliceParallelism = 4
	// workdayRetryMaxAttempts and workdayRetryBase bound how a rate-limited (403/429) listing
	// or detail request is retried before giving up.
	workdayRetryMaxAttempts = 6
	workdayRetryBase        = time.Second
)

// workday adapts Workday's public "CXS" careers API. The board id is the public board
// host and site path, e.g. "ringcentral.wd1.myworkdayjobs.com/RingCentral_Careers";
// the API tenant is the host's first label (here "ringcentral"). The listing endpoint
// is POST-only and carries no description, so it pages the postings and fetches each
// posting's detail (bounded-concurrency) to assemble the description.
// workdayHTTP is the transport workday needs: a POST-only JSON listing plus JSON detail.
type workdayHTTP interface {
	JSONGetter
	JSONPoster
}

type workday struct {
	http workdayHTTP
	// retryBase is the first backoff delay for a rate-limited (403/429) request — a field, not
	// a package var, so tests disable sleeping without touching shared state.
	retryBase time.Duration
}

// NewWorkday builds the Workday adapter over the given HTTP client.
func NewWorkday(c workdayHTTP) Source { return workday{http: c, retryBase: workdayRetryBase} }

func (workday) Provider() string { return "workday" }

// fullBoardListing: listPostings proves completeness on every path — pageAll pages an
// uncapped board to its own reported total, and splitByFacet either resolves a capped board
// via facet subdivision or fails the crawl outright rather than return a partial listing as a
// success (see splitByFacet). See the fullBoardListing interface for the bar.
func (workday) fullBoardListing() {}

// emptyBoardListing: listPostings fails a non-2xx answer (an unknown tenant is 422, an unknown
// site 404) and an undecodable one (a challenge or HTML page), and it fails a first page whose
// jobPostings is absent or null or contradicts a positive total. Only a 200 carrying the
// collection present and empty with total 0 is an empty board. Quiet backoff is left alone: a
// board turning empty changes its fingerprint, which ends the quiet state, and later empty
// crawls pause at most quietMax, each ending with Quiet == 0 so boardListedEmpty admits it.
func (workday) emptyBoardListing() {}

// workdayBoard is a configured board parsed into the parts the CXS endpoints need, plus the
// board's prefix on the public careers site — which is not the CXS path, and differs between
// the two host shapes.
type workdayBoard struct {
	host, tenant, site string
	publicPath         string
}

// parseWorkdayBoard splits a board into the host, tenant, site and public path. Workday
// publishes a career site under either of two host shapes:
//
//	acme.wd1.myworkdayjobs.com/Careers  — a per-tenant host; the tenant is its first label,
//	                                      and the public site is served at /<site>.
//	wd1.myworkdaysite.com/snapchat/snap — a host shared between tenants, so the board spells
//	                                      the tenant out as a middle segment and the public
//	                                      site is served at /recruiting/<tenant>/<site>.
//
// Both address the same CXS endpoints at /wday/cxs/<tenant>/<site>/.
func parseWorkdayBoard(board string) (workdayBoard, error) {
	host, rest, ok := strings.Cut(board, "/")
	if !ok || host == "" || rest == "" {
		return workdayBoard{}, fmt.Errorf("workday: board %q must be \"host/site\"", board)
	}

	if tenant, site, spelledOut := strings.Cut(rest, "/"); spelledOut {
		if tenant == "" || site == "" {
			return workdayBoard{}, fmt.Errorf("workday: board %q must be \"host/tenant/site\"", board)
		}
		return workdayBoard{
			host: host, tenant: tenant, site: site,
			publicPath: "recruiting/" + tenant + "/" + site,
		}, nil
	}

	tenant, _, ok := strings.Cut(host, ".")
	if !ok || tenant == "" {
		return workdayBoard{}, fmt.Errorf("workday: board host %q has no tenant label", host)
	}
	return workdayBoard{host: host, tenant: tenant, site: rest, publicPath: rest}, nil
}

// workdayPosting is one item from the jobs listing (no description here).
type workdayPosting struct {
	Title         string `json:"title"`
	ExternalPath  string `json:"externalPath"`
	LocationsText string `json:"locationsText"`
}

// Fetch is the list-only-plus-detail crawl: it hydrates every posting. Kept for callers that do
// not drive hydration; ingest goes through FetchNew.
func (s workday) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.crawl(ctx, e, func(string) bool { return false })
}

// FetchNew is the hydrating crawl: it lists the board exactly as Fetch does, but fetches a
// posting's detail — the description the listing omits — only for a posting the catalogue lacks.
// A seen posting yields the listing-only job as a liveness refresh, costing no request at all.
//
// This is what makes a large board crawlable: Workday serves 20 postings per listing page and
// one per detail request, so re-fetching descriptions the catalogue already stores gets the
// crawl rate-limited into failure, after which the board's catalogue leaks as permanently open.
func (s workday) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.crawl(ctx, e, seen)
}

// crawl is the shared walk behind Fetch and FetchNew: page the board, then map each posting to a
// Job — hydrating the ones seen reports as new, and marking the rest for a liveness refresh.
// Detail fetches run under the shared bounded worker pool.
func (s workday) crawl(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseWorkdayBoard(e.Board)
	if err != nil {
		return nil, err
	}

	postings, err := s.listPostings(ctx, b)
	if err != nil {
		return nil, err
	}

	return fetchDetails(postings, defaultDetailWorkers, func(p workdayPosting) (Job, bool) {
		if seen(p.ExternalPath) {
			// Already ingested: refresh liveness only, no detail request. The job carries just
			// the identity the listing supplies — the pipeline resolves the stored row from it
			// and must not re-upsert content, which would wipe the hydrated description.
			return s.listingJob(e, b, p), true
		}
		return s.detail(ctx, e, b, p)
	}), nil
}

// listingJob builds the identity-only Job a liveness refresh needs from the listing alone: the
// same external id and URL the hydrated path would produce, so the refresh resolves to the row
// that posting was stored as.
func (s workday) listingJob(e CompanyEntry, b workdayBoard, p workdayPosting) Job {
	return Job{
		ExternalID:  p.ExternalPath,
		URL:         fmt.Sprintf("https://%s/%s%s", b.host, b.publicPath, p.ExternalPath),
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Location:    p.LocationsText,
		SeenRefresh: true,
	}
}

// workdayListPage is one page of the jobs-listing response.
type workdayListPage struct {
	Total       int              `json:"total"`
	JobPostings []workdayPosting `json:"jobPostings"`
	Facets      []workdayFacet   `json:"facets"`
}

// listPostings pages through the board's postings via the POST-only jobs endpoint. Most boards
// page directly (pageAll); a board whose first page reports the capped total (workdayCapTotal)
// is walked by facet instead (splitByFacet), since that total is a ceiling on what a single
// query can return, not the board's true size.
func (s workday) listPostings(ctx context.Context, b workdayBoard) ([]workdayPosting, error) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", b.host, b.tenant, b.site)
	first, err := s.fetchListPage(ctx, url, map[string]any{}, 0)
	if err != nil {
		return nil, fmt.Errorf("workday: list board %s: %w", b.site, err)
	}
	if first.JobPostings == nil {
		return nil, errListingAbsent("workday", b.site, "jobPostings")
	}
	// A capped total is walked by facet below, whatever its first page carries.
	if len(first.JobPostings) == 0 && first.Total > 0 && first.Total != workdayCapTotal {
		return nil, fmt.Errorf("workday: board %s/%s: the first page lists no postings but reports a total of %d",
			b.tenant, b.site, first.Total)
	}
	if first.Total != workdayCapTotal {
		return s.pageAll(ctx, url, map[string]any{}, first)
	}
	log.Printf("workday: board %s/%s reports the capped total (%d); splitting by facet",
		b.tenant, b.site, workdayCapTotal)
	inflight := make(chan struct{}, workdaySliceParallelism)
	return s.splitByFacet(ctx, url, b, map[string]any{}, first, map[string]bool{}, 0, inflight)
}

// fetchListPage fetches one page of the given (possibly facet-scoped) query.
func (s workday) fetchListPage(ctx context.Context, url string, appliedFacets map[string]any, offset int) (workdayListPage, error) {
	reqBody := map[string]any{
		"appliedFacets": appliedFacets,
		"limit":         workdayPageLimit,
		"offset":        offset,
		"searchText":    "",
	}
	var page workdayListPage
	if err := s.postJSONRetrying(ctx, url, reqBody, &page); err != nil {
		return workdayListPage{}, err
	}
	return page, nil
}

// pageAll pages a query (unfiltered or facet-scoped) to exhaustion, starting from an
// already-fetched first page so the caller's initial request is never wasted.
func (s workday) pageAll(ctx context.Context, url string, appliedFacets map[string]any, first workdayListPage) ([]workdayPosting, error) {
	var postings []workdayPosting
	// Some boards report the real total only on the first page and total:0 thereafter, so latch
	// the first non-zero total; reading each page's total would silently truncate the board,
	// which the unseen sweep then reads as postings removed.
	total := -1
	offset := 0
	page := first
	for len(page.JobPostings) != 0 {

		postings = append(postings, page.JobPostings...)
		offset += len(page.JobPostings)
		if total < 0 && page.Total > 0 {
			total = page.Total
		}
		if total >= 0 && offset >= total {
			break
		}
		next, err := s.fetchListPage(ctx, url, appliedFacets, offset)
		if err != nil {
			return nil, err
		}
		page = next
	}
	return postings, nil
}

// detail fetches one posting's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that posting.
func (s workday) detail(ctx context.Context, e CompanyEntry, b workdayBoard, p workdayPosting) (Job, bool) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", b.host, b.tenant, b.site, p.ExternalPath)

	var d struct {
		JobPostingInfo struct {
			Title                  string `json:"title"`
			JobDescription         string `json:"jobDescription"`
			Location               string `json:"location"`
			StartDate              string `json:"startDate"`
			ExternalURL            string `json:"externalUrl"`
			RemoteType             string `json:"remoteType"`
			TimeType               string `json:"timeType"`
			JobRequisitionLocation struct {
				Country struct {
					Alpha2Code string `json:"alpha2Code"`
				} `json:"country"`
			} `json:"jobRequisitionLocation"`
		} `json:"jobPostingInfo"`
	}
	if err := s.getJSONRetrying(ctx, url, &d); err != nil {
		return Job{}, false
	}
	info := d.JobPostingInfo

	title := firstNonEmpty(strings.TrimSpace(info.Title), strings.TrimSpace(p.Title))
	location := firstNonEmpty(info.Location, p.LocationsText)
	jobURL := info.ExternalURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://%s/%s%s", b.host, b.publicPath, p.ExternalPath)
	}
	workMode := workdayWorkMode(info.RemoteType)
	remote := workMode == "remote" || isRemote(location)

	return Job{
		ExternalID:  p.ExternalPath,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(info.JobDescription),
		Remote:      remote,
		WorkMode:    workMode,
		Countries:   countryFromCode(info.JobRequisitionLocation.Country.Alpha2Code),
		PostedAt:    parseWorkdayDate(info.StartDate),
		// timeType is Workday's structured full/part-time enum; the pipeline prefers it
		// over the free-text employment-type parse. Workday's timeType only distinguishes
		// full vs part time (contract/intern live in other, per-tenant fields).
		EmploymentType: workdayEmploymentType(info.TimeType),
	}, true
}

// workdayWorkModes maps a Workday remoteType label, lowercased, to a work mode. remoteType is
// not an enum: each tenant names its own workplace types, in its own language. Every entry is
// a label a live tenant publishes. A label that offers a choice ("Hybrid or Onsite"), a schedule
// ("Flex"), a place that is neither an office nor home ("Field Based", "Client site") or nothing
// at all ("Not specified") is absent, so it states no work mode rather than a guessed one.
// A benefit beside an office ("Mobiles Arbeiten möglich", "Télétravail possible") is hybrid,
// as the description dictionaries read it; a refusal of remote work ("Not Remote", "No mobile
// working") is onsite.
var workdayWorkModes = map[string]string{
	"remote": "remote", "fully remote": "remote", "remote-restricted": "remote",
	"home based/remote": "remote", "remote work only": "remote", "virtual": "remote",

	"hybrid": "hybrid", "hybrid working": "hybrid", "hybrid work": "hybrid", "hybrid remote": "hybrid",
	"this position is a hybrid of office/remote working": "hybrid",
	"onsite - hybrid eligible":                           "hybrid",
	"eligible for hybrid work":                           "hybrid",
	"partially remote":                                   "hybrid",
	"office flex":                                        "hybrid",
	"location type: hybrid option available (mix of in-person and remote)": "hybrid",
	"mobiles arbeiten möglich": "hybrid", "partial mobile working": "hybrid",
	"télétravail partiel": "hybrid", "télétravail possible": "hybrid", "híbrido": "hybrid",

	"on-site": "onsite", "onsite": "onsite", "on site": "onsite", "on campus": "onsite",
	"office": "onsite", "in office": "onsite", "in-office": "onsite", "in-office position": "onsite",
	"in person": "onsite", "in-person": "onsite", "fully onsite": "onsite", "fully in-person": "onsite",
	"onsite only": "onsite", "onsite job": "onsite", "on-site/in-office": "onsite",
	"full-time onsite": "onsite", "in office 100%": "onsite", "n/a - in office only": "onsite",
	"in branch": "onsite", "not remote": "onsite",
	"on-site only, not eligible for remote work":      "onsite",
	"location type: in person (remote not available)": "onsite",
	"not eligible for hybrid work":                    "onsite",
	"not eligible for agile working":                  "onsite",
	"no mobile working":                               "onsite",
	"mobiles arbeiten nicht möglich":                  "onsite",
	"poste uniquement en présentiel":                  "onsite",
	"travail sur site":                                "onsite",
}

// workdayWorkMode reads a posting's remoteType label as a work mode, "" when it states none.
// Beyond the exact labels, a qualified remote or hybrid label ("Remote - Nationwide", "Remote
// - Must reside within territory", "Hybrid - Onsite / 2 Days Remote", "Hybrid (Onsite/WFA)")
// keeps the mode it opens with: the qualifier narrows where or how often, never which.
func workdayWorkMode(remoteType string) string {
	label := strings.ToLower(strings.Join(strings.Fields(remoteType), " "))
	if m, ok := workdayWorkModes[label]; ok {
		return m
	}
	switch {
	case strings.HasPrefix(label, "remote -"):
		return "remote"
	case strings.HasPrefix(label, "hybrid -"), strings.HasPrefix(label, "hybrid ("):
		return "hybrid"
	}
	return ""
}

// postJSONRetrying and getJSONRetrying retry a rate-limited (403/429) request with exponential
// backoff instead of failing the whole board on one throttled response. Some Workday tenants
// return 403 for a burst of requests rather than 429, which is why isRateLimited counts it.
// A non-rate-limit error (e.g. 404) returns immediately.
func (s workday) postJSONRetrying(ctx context.Context, url string, body, v any) error {
	return s.retrying(ctx, func() error { return s.http.PostJSON(ctx, url, body, v) })
}

func (s workday) getJSONRetrying(ctx context.Context, url string, v any) error {
	return s.retrying(ctx, func() error { return s.http.GetJSON(ctx, url, v) })
}

func (s workday) retrying(ctx context.Context, call func() error) error {
	return retryWhile(ctx, workdayRetryMaxAttempts, s.retryBase, isRateLimited, call)
}

// workdayEmploymentType maps Workday's timeType ("Full time" / "Part time") onto the
// freehire employment-type vocabulary, returning "" for any other/absent value so the
// description parser decides instead — structured signal only, never a guess.
func workdayEmploymentType(timeType string) string {
	switch strings.TrimSpace(strings.ToLower(timeType)) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	default:
		return ""
	}
}

// parseWorkdayDate reads Workday's startDate, which may be a full RFC3339 timestamp or
// a date-only value, returning nil for anything unparseable (posted_at is nullable).
func parseWorkdayDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseDate(s)
}
