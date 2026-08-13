package sources

import (
	"context"
	"errors"
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
	// capped board. Verified sufficient against the worst known case (Accenture's largest
	// single job family is still capped after one dimension, resolved by a second) with
	// headroom to spare; raise it if a board's exhaustion log shows it is not enough.
	maxFacetDepth = 3
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
// This is what makes a large board crawlable. Workday serves 20 postings per listing page and one
// posting per detail request, so a 24k-posting board spends 24k requests per crawl on descriptions
// it already stores — enough for the platform to rate-limit the crawl into failure, after which
// the board is never seen again and its whole catalogue leaks as permanently-open jobs.
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

// workdayFacetValue is one value of a listing facet, e.g. a single job family within the
// "jobFamilyGroup" dimension. Count is the platform's own true count for that value, which is
// what makes it safe to plan a subdivision from a response already being parsed — no extra
// probe request needed.
type workdayFacetValue struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// workdayFacet is one filterable dimension a listing response offers, e.g. "jobFamilyGroup" or
// "workerSubType". Applying a value from it (via appliedFacets) scopes the *other* dimensions'
// counts in the response that follows, but not this dimension's own counts (verified live
// against Workday) — so subdividing a still-capped slice further means picking a dimension not
// yet applied in that recursion branch, never re-applying the same one.
type workdayFacet struct {
	Parameter string              `json:"facetParameter"`
	Values    []workdayFacetValue `json:"values"`
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
	if first.Total != workdayCapTotal {
		return s.pageAll(ctx, url, map[string]any{}, first)
	}
	log.Printf("workday: board %s/%s reports the capped total (%d); splitting by facet",
		b.tenant, b.site, workdayCapTotal)
	return s.splitByFacet(ctx, url, b, map[string]any{}, first, map[string]bool{}, 0)
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
	// Some boards (e.g. pg.wd5.myworkdayjobs.com) report the real total only on the
	// first page and total:0 thereafter, so latch the first non-zero total and page
	// against it — reading each page's total would break after page one and silently
	// truncate the board, which the 48h unseen sweep then reads as postings removed.
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

// splitByFacet subdivides a capped slice by the not-yet-used facet dimension that best splits
// it, recursing into each value's own slice (which may itself still be capped, in which case it
// recurses again with a different dimension) up to maxFacetDepth combined dimensions. Results
// are merged and deduped by ExternalPath, since a multi-counting dimension (e.g. workerSubType)
// can place the same posting in more than one slice.
func (s workday) splitByFacet(ctx context.Context, url string, b workdayBoard, appliedFacets map[string]any, capped workdayListPage, used map[string]bool, depth int) ([]workdayPosting, error) {
	dim := pickSplitDimension(capped.Facets, used)
	if dim == nil {
		log.Printf("workday: board %s/%s capped at %d with no further facet dimension to split on; results may be incomplete",
			b.tenant, b.site, workdayCapTotal)
		return s.pageAll(ctx, url, appliedFacets, capped)
	}
	if depth >= maxFacetDepth {
		log.Printf("workday: board %s/%s still capped at %d after splitting %d facet dimensions deep; results may be incomplete",
			b.tenant, b.site, workdayCapTotal, maxFacetDepth)
		return s.pageAll(ctx, url, appliedFacets, capped)
	}

	nextUsed := make(map[string]bool, len(used)+1)
	for k := range used {
		nextUsed[k] = true
	}
	nextUsed[dim.Parameter] = true

	byExternalPath := map[string]workdayPosting{}
	for _, v := range dim.Values {
		sliceFacets := cloneAppliedFacets(appliedFacets)
		sliceFacets[dim.Parameter] = []string{v.ID}

		page, err := s.fetchListPage(ctx, url, sliceFacets, 0)
		if err != nil {
			return nil, err
		}
		var slice []workdayPosting
		if page.Total == workdayCapTotal {
			slice, err = s.splitByFacet(ctx, url, b, sliceFacets, page, nextUsed, depth+1)
		} else {
			slice, err = s.pageAll(ctx, url, sliceFacets, page)
		}
		if err != nil {
			return nil, err
		}
		for _, p := range slice {
			byExternalPath[p.ExternalPath] = p
		}
	}

	postings := make([]workdayPosting, 0, len(byExternalPath))
	for _, p := range byExternalPath {
		postings = append(postings, p)
	}
	return postings, nil
}

// pickSplitDimension returns the facet dimension (not already in used) whose largest single
// value is smallest — i.e. the dimension that leaves the smallest oversized remainder after
// splitting, so the recursion is most likely to clear the cap in one more level rather than
// hitting maxFacetDepth still stuck on an enormous single value. (The opposite heuristic —
// picking the dimension with the single biggest value anywhere — was tried first and, live
// against Accenture, kept choosing a dimension whose top value was still ~2000-sized, making
// no real progress across several depth-3 exhaustions before this was corrected.)
// Returns nil if every present dimension has already been applied in this branch.
func pickSplitDimension(facets []workdayFacet, used map[string]bool) *workdayFacet {
	var best *workdayFacet
	bestMax := -1
	for i := range facets {
		f := &facets[i]
		if used[f.Parameter] || len(f.Values) == 0 {
			continue
		}
		max := 0
		for _, v := range f.Values {
			if v.Count > max {
				max = v.Count
			}
		}
		if best == nil || max < bestMax {
			bestMax = max
			best = f
		}
	}
	return best
}

// cloneAppliedFacets copies an appliedFacets map so a recursion branch can add its own facet
// value without mutating the map a sibling branch is still using.
func cloneAppliedFacets(appliedFacets map[string]any) map[string]any {
	out := make(map[string]any, len(appliedFacets)+1)
	for k, v := range appliedFacets {
		out[k] = v
	}
	return out
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
	remote := isRemote(location) || strings.Contains(strings.ToLower(info.RemoteType), "remote")

	return Job{
		ExternalID:  p.ExternalPath,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(info.JobDescription),
		Remote:      remote,
		Countries:   countryFromCode(info.JobRequisitionLocation.Country.Alpha2Code),
		PostedAt:    parseWorkdayDate(info.StartDate),
		// timeType is Workday's structured full/part-time enum; the pipeline prefers it
		// over the free-text employment-type parse. Workday's timeType only distinguishes
		// full vs part time (contract/intern live in other, per-tenant fields).
		EmploymentType: workdayEmploymentType(info.TimeType),
	}, true
}

// isWorkdayRateLimited reports whether err is a Workday rate-limit response (403 or 429). Some
// Workday tenants return 403 for a burst of requests rather than 429 (the same distinction
// eightfold.go already handles for that provider); the shared client already retries 429/5xx,
// this adds 403.
func isWorkdayRateLimited(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == 403 || se.Code == 429)
}

// postJSONRetrying and getJSONRetrying retry a rate-limited (403/429) request with exponential
// backoff instead of failing the whole board on one throttled response, mirroring eightfold.go's
// getJSONRetrying/isRateLimited. A non-rate-limit error (e.g. 404) returns immediately.
func (s workday) postJSONRetrying(ctx context.Context, url string, body, v any) error {
	return s.retrying(ctx, func() error { return s.http.PostJSON(ctx, url, body, v) })
}

func (s workday) getJSONRetrying(ctx context.Context, url string, v any) error {
	return s.retrying(ctx, func() error { return s.http.GetJSON(ctx, url, v) })
}

// retrying runs call, retrying with exponential backoff while the error is a rate limit
// (isWorkdayRateLimited) and giving up after workdayRetryMaxAttempts.
func (s workday) retrying(ctx context.Context, call func() error) error {
	delay := s.retryBase
	var err error
	for attempt := 0; attempt <= workdayRetryMaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			if delay < 30*time.Second {
				delay *= 2
			}
		}
		if err = call(); err == nil || !isWorkdayRateLimited(err) {
			return err
		}
	}
	return err
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
