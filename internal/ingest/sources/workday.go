package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
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
	// workdaySliceParallelism bounds how many facet-slice listing requests one capped board has
	// in flight at once, across every level of its split. A capped board is split by its
	// finest dimension, which is often one value per store: michaels.wd5 has 1,340 locations and
	// bridgestone.wd5 1,676, one request each. Made serially that is 15–30 minutes for one board,
	// enough to eat a sharded run's whole budget. Four at a time keeps the load on one tenant
	// modest, and the 403/429 retry absorbs a refusal.
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
	// Parameter and Values are set instead of ID and Count when the value is itself a facet:
	// Workday nests its location dimensions (locationCountry, locationRegionStateProvince,
	// locations, ...) inside a "locationMainGroup" wrapper whose values are facets, not values.
	Parameter string              `json:"facetParameter"`
	Values    []workdayFacetValue `json:"values"`
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
//
// Slices are listed concurrently; inflight is one semaphore shared by the whole split, so the
// board never has more than workdaySliceParallelism requests open however deep it recurses. It is
// held only around a slice's own requests, never across a recursion, so a level waiting on its
// children cannot starve them.
func (s workday) splitByFacet(ctx context.Context, url string, b workdayBoard, appliedFacets map[string]any, capped workdayListPage, used map[string]bool, depth int, inflight chan struct{}) ([]workdayPosting, error) {
	dim := pickSplitDimension(capped.Facets, used)
	if dim == nil {
		// Paging the capped response as-is would return fewer postings than the board
		// actually has and report success — indistinguishable from the board having shrunk,
		// and exactly the shape that let solidjobs' undetected truncation force freehire#2337's
		// revert. A board this size cannot be proven complete without a usable dimension, so
		// the crawl must fail loudly instead.
		return nil, fmt.Errorf("workday: board %s/%s capped at %d with no further facet dimension to split on",
			b.tenant, b.site, workdayCapTotal)
	}
	if depth >= maxFacetDepth {
		// Same reasoning as the no-dimension case above: a still-capped slice this deep cannot
		// be proven complete, so it must fail rather than silently return the depth limit's
		// leaf slice as the whole board.
		return nil, fmt.Errorf("workday: board %s/%s still capped at %d after splitting %d facet dimensions deep",
			b.tenant, b.site, workdayCapTotal, maxFacetDepth)
	}

	nextUsed := make(map[string]bool, len(used)+1)
	for k := range used {
		nextUsed[k] = true
	}
	nextUsed[dim.Parameter] = true

	var mu sync.Mutex
	byExternalPath := map[string]workdayPosting{}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workdaySliceParallelism)
	for _, v := range dim.Values {
		sliceFacets := cloneAppliedFacets(appliedFacets)
		sliceFacets[dim.Parameter] = []string{v.ID}
		g.Go(func() error {
			inflight <- struct{}{}
			page, err := s.fetchListPage(gctx, url, sliceFacets, 0)
			<-inflight
			if err != nil {
				return err
			}
			var slice []workdayPosting
			if page.Total == workdayCapTotal {
				slice, err = s.splitByFacet(gctx, url, b, sliceFacets, page, nextUsed, depth+1, inflight)
			} else {
				inflight <- struct{}{}
				slice, err = s.pageAll(gctx, url, sliceFacets, page)
				<-inflight
			}
			if err != nil {
				return err
			}
			mu.Lock()
			for _, p := range slice {
				byExternalPath[p.ExternalPath] = p
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	postings := make([]workdayPosting, 0, len(byExternalPath))
	for _, p := range byExternalPath {
		postings = append(postings, p)
	}
	return postings, nil
}

// splittableFacets flattens Workday's facet tree into the dimensions a query can actually be
// scoped by. A wrapper whose values are themselves facets ("locationMainGroup" holding
// locationCountry, locationRegionStateProvince and locations) is replaced by those nested facets,
// and a value without an id is dropped, since it cannot be applied. Measured 2026-10-27 on
// michaels.wd5: the nested state facet (largest value 378) is what clears the 2000 cap, while the
// wrapper — read as a dimension of zero-count values — looked like the best split of all and
// was applied with an empty id.
func splittableFacets(facets []workdayFacet) []workdayFacet {
	var out []workdayFacet
	var walk func(f workdayFacet)
	walk = func(f workdayFacet) {
		applicable := workdayFacet{Parameter: f.Parameter}
		for _, v := range f.Values {
			if v.Parameter != "" {
				walk(workdayFacet{Parameter: v.Parameter, Values: v.Values})
				continue
			}
			if v.ID != "" {
				applicable.Values = append(applicable.Values, workdayFacetValue{ID: v.ID, Count: v.Count})
			}
		}
		if len(applicable.Values) > 0 {
			out = append(out, applicable)
		}
	}
	for _, f := range facets {
		walk(f)
	}
	return out
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
	candidates := splittableFacets(facets)
	var best *workdayFacet
	bestMax := -1
	for i := range candidates {
		f := &candidates[i]
		if used[f.Parameter] || len(f.Values) == 0 {
			continue
		}
		max, sum := 0, 0
		for _, v := range f.Values {
			sum += v.Count
			if v.Count > max {
				max = v.Count
			}
		}
		// A capped slice holds at least workdayCapTotal postings, so a dimension whose values
		// add up to fewer cannot partition it: Workday's "distance" facet sums to 5 on a
		// 4,390-posting board, and splitting on it listed 548 and called that the whole board.
		if sum < workdayCapTotal {
			continue
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
// not an enum: each tenant names its own workplace types, in its own language, and about one
// posting in eight carries one. Every entry is a label a live tenant publishes (sampled across
// ~1,000 boards, 2026-10-30). A label that offers a choice ("Hybrid or Onsite"), a schedule
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
