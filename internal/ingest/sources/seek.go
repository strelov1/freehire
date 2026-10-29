package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// seek adapts SEEK, the dominant job board in Australia and New Zealand. It aggregates postings
// from many employers (employer read per posting), enumerated by ICT subclassification id carried
// as the board file entry's board, with the market in the entry's Region — the same
// board-is-a-slice, region-is-a-market split sources/adzuna.yml uses.
//
// SEEK's human-facing pages sit behind a Cloudflare interstitial (a plain request to
// /software-engineer-jobs/... answers 403 "Just a moment..."), but its own frontend search API does
// not: /api/jobsearch/v5/search serves JSON to any client — no cookie, no credential and no
// browser-shaped User-Agent.
//
// seekCrawler is the protocol both providers share. Each provider is its own type over it —
// seek and jobStreet — because the sweep markers differ and a marker is a method set, not a
// field: SEEK AU/NZ reads only a window of each slice (sweepGrace), while JobStreet reads each
// board to the total its search states (fullBoardListing, boardURLPrefix).
type seekCrawler struct {
	provider string
	markets  map[string]seekMarket
	maxPages int
	// verifyTotal makes the walk a proof of the whole board: reaching the page ceiling, a later
	// page failing, or a walk whose distinct postings differ from the search's stated totalCount
	// all fail the crawl instead of returning what was gathered. JobStreet only.
	verifyTotal bool
	http        JSONGetter
	details     *seekDetailNetwork
}

// seek is SEEK AU/NZ.
type seek struct{ seekCrawler }

// jobStreet is JobStreet (SG/MY/ID/PH) and JobsDB (HK/TH).
type jobStreet struct{ seekCrawler }

// NewSeek builds the SEEK adapter. The listing and the detail take SEPARATE transports because only
// one of them needs pacing: the search listing is ~150 requests a run and has never been refused,
// while the GraphQL detail is metered by a per-IP budget (see seekDetailInterval). sources.All hands
// the bare client for the first and the network's one shared detail client for the second.
func NewSeek(list JSONGetter, detail *seekDetailNetwork) Source {
	return seek{seekCrawler{provider: "seek", markets: seekMarkets, maxPages: seekMaxPages, http: list, details: detail}}
}

// NewJobStreet builds the JobStreet/JobsDB adapter over the same SEEK frontend protocol.
// JobStreet (SG/MY/ID/PH) and JobsDB (HK/TH) share SEEK's v5 search and GraphQL schemas but use
// different market hosts/site keys, so they are one implementation with a distinct provider
// identity and board catalogue.
func NewJobStreet(list JSONGetter, detail *seekDetailNetwork) Source {
	return jobStreet{seekCrawler{provider: "jobstreet", markets: jobStreetMarkets, maxPages: jobStreetMaxPages, verifyTotal: true, http: list, details: detail}}
}

// seekNetworkProviders are the adapters that share one seekDetailNetwork. ApplyProxyEgress rebuilds
// them together over a network that has a proxy lane, so a ban either of them meets moves both.
var seekNetworkProviders = map[string]func(JSONGetter, *seekDetailNetwork) Source{
	"seek":      NewSeek,
	"jobstreet": NewJobStreet,
}

func (s seekCrawler) Provider() string { return s.provider }

// aggregator marks seek as a genuine multi-company aggregator: a vacancy an employer also posts on
// its own ATS appears here too, so the cross-source dedup pass prefers the first-party copy. Unlike
// the boardless aggregators seek still requires a board (the subclassification id) to bound the
// crawl, so it is not boardless.
func (seekCrawler) aggregator() {}

// sweepGrace widens the unseen sweep because the crawl reaches only a SLICE of each subclassification:
// SEEK stops serving results past roughly the 550th (see seekMaxPages), so the busiest slices — five
// of Australia's 22 when this was written, the largest at ~746 postings — have a tail no crawl can
// reach. Ordered newest-first the reachable window covers most of a SEEK advertisement's 30-day run,
// but a posting drifting past it would, on the 48-hour default, be closed and reopened as it drifts
// back, writing a phantom removal into job_daily_stats each cycle. The marker is sound here because
// liveness CANNOT be probed instead: SEEK's own job pages sit behind the same Cloudflare interstitial
// as its search pages.
func (seek) sweepGrace() time.Duration { return 14 * 24 * time.Hour }

// fullBoardListing: JobStreet has no SEEK-style result window — live SG 6290 served 17 pages
// to its natural end — and at pageSize 100 the search's totalCount equals the distinct postings
// a full walk reads (2026-10-29: SG 6290 1,629 of 1,629, SG 6302 9/9, MY 6287 641/641, ID 6287
// 686/686, HK 6281 3,694/3,694). The walk fails on a later page's error, on the page ceiling,
// and on a distinct count that differs from the last page's totalCount. See the
// fullBoardListing interface for the bar.
func (jobStreet) fullBoardListing() {}

// BoardURLPrefix: a JobStreet board id (an ICT subclassification) repeats in every market, and
// external_id carries no market, so the board-scoped close narrows each board to its market
// by the host its postings are served from (toJob builds every URL as host + "/job/" + id).
// See the RegionScopedBoards interface.
func (s jobStreet) BoardURLPrefix(region string) (string, bool) {
	m, ok := s.markets[strings.ToLower(strings.TrimSpace(region))]
	if !ok {
		return "", false
	}
	return m.host + "/job/", true
}

const (
	seekPageSize = 100
	// seekMaxPages backstops the page loop at SEEK's result-window cap: at pageSize 100 the edge
	// serves pages 1..5 and answers page 6 with an empty list, so six requests reach the whole
	// window and confirm its end. The walk's real stop condition is a page adding no new posting;
	// this only bounds an edge that keeps answering with fresh inventory.
	seekMaxPages = 6
	// JobStreet/JobsDB does not share SEEK AU/NZ's ~550-result window: live SG 6290
	// served 17 pages (1,698 rows) and page 18 empty on 2026-09-12. Keep a generous
	// safety ceiling, but unlike SEEK treat reaching it as a crawl failure — JobStreet
	// has demonstrated a natural end, so silently accepting a capped walk would throw
	// away inventory the API was willing to serve.
	jobStreetMaxPages = 100
)

// seekMarket is one SEEK country site. The three request fields travel together because none works
// alone: host serves the market's own domain, siteKey selects its catalogue, and where scopes the
// search — omitting where does NOT mean "everywhere", it collapses the result set to a small
// unrelated subset (36 of 688 on the slice this was measured against).
type seekMarket struct {
	host    string
	siteKey string
	where   string
}

// seekMarkets maps a board entry's Region to its market. A region outside this map is a config
// error, reported per board rather than guessed.
var seekMarkets = map[string]seekMarket{
	"au": {host: "https://www.seek.com.au", siteKey: "AU-Main", where: "All Australia"},
	"nz": {host: "https://www.seek.co.nz", siteKey: "NZ-Main", where: "All New Zealand"},
}

// jobStreetMarkets are the APAC SEEK-network sites live-verified against the same v5 API on
// 2026-09-12. `where` is load-bearing and is NOT spelled "All <country>" on these sites:
// Singapore returned zero results for "All Singapore" while "Singapore" returned the full
// market. Hong Kong and Thailand expose the same platform under the legacy JobsDB brand.
var jobStreetMarkets = map[string]seekMarket{
	"sg": {host: "https://sg.jobstreet.com", siteKey: "SG-Main", where: "Singapore"},
	"my": {host: "https://my.jobstreet.com", siteKey: "MY-Main", where: "Malaysia"},
	"id": {host: "https://id.jobstreet.com", siteKey: "ID-Main", where: "Indonesia"},
	"ph": {host: "https://ph.jobstreet.com", siteKey: "PH-Main", where: "Philippines"},
	"hk": {host: "https://hk.jobsdb.com", siteKey: "HK-Main", where: "Hong Kong"},
	"th": {host: "https://th.jobsdb.com", siteKey: "TH-Main", where: "Thailand"},
}

// seekSearchPage is the slice of a search response the adapter reads. SEEK AU/NZ's totalCount is
// not a check: SEEK reports it as a function of pageSize (the same query answered 36 at pageSize=1, 688
// at pageSize=20 and 666 at pageSize=100), so it can drive neither pagination nor a truncation check.
type seekSearchPage struct {
	Data []seekPosting `json:"data"`
	// TotalCount is read only where it was measured to be exact (verifyTotal): JobStreet at
	// pageSize 100. SEEK AU/NZ's varies with the page size, as above.
	TotalCount int `json:"totalCount"`
}

// seekPosting is one search-result posting. It carries every field the adapter needs except the
// description, which the listing gives only as a one-line teaser.
type seekPosting struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	CompanyName string `json:"companyName"`
	Advertiser  struct {
		Description string `json:"description"`
	} `json:"advertiser"`
	Locations        []seekLocation       `json:"locations"`
	ListingDate      string               `json:"listingDate"`
	SalaryLabel      string               `json:"salaryLabel"`
	WorkTypes        []string             `json:"workTypes"`
	WorkArrangements seekWorkArrangements `json:"workArrangements"`
}

// seekWorkArrangements is SEEK's structured work-arrangement block. A posting may offer several.
type seekWorkArrangements struct {
	Data []seekWorkArrangement `json:"data"`
}

type seekWorkArrangement struct {
	Label struct {
		Text string `json:"text"`
	} `json:"label"`
}

// seekLocation is one place a posting names. The label is the free-text form SEEK displays; the
// country code is the structured signal, so it feeds Job.Countries directly.
type seekLocation struct {
	Label       string `json:"label"`
	CountryCode string `json:"countryCode"`
}

// Fetch is the list-only crawl (no description).
func (s seekCrawler) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	m, postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(m); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew hydrates a posting's description from SEEK's GraphQL endpoint only for a posting the
// catalogue does not already have (seen); a seen posting yields the list-only job marked
// SeenRefresh, so the pipeline refreshes liveness without spending a detail request or wiping the
// body hydrated when it was new. A single detail failure is isolated (logged, list-only fallback) —
// a posting is never dropped over a missing description.
func (s seekCrawler) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	m, postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, e, m, postings, seen, nil), nil
}

// FetchNewGated lets the pipeline tell this aggregator which listed employers are already
// covered by first-party ATS sources before we spend SEEK's scarce GraphQL detail budget.
// Covered postings are still emitted list-only so the ordinary aggregator gate can count and
// discard them; only the body request is skipped. This matters especially for JobStreet, where
// a first crawl spans tens of thousands of listings but a substantial share of employers are
// already covered directly by freehire.
func (s seekCrawler) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	m, postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(postings))
	for _, p := range postings {
		if company := p.employer(); company != "" {
			names = append(names, company)
		}
	}
	return s.hydrate(ctx, e, m, postings, seen, gate.Covered(names)), nil
}

func (s seekCrawler) hydrate(ctx context.Context, e CompanyEntry, m seekMarket, postings []seekPosting,
	seen func(externalID string) bool, skip map[string]bool) []Job {
	return fetchDetails(postings, defaultDetailWorkers, func(p seekPosting) (Job, bool) {
		base, ok := p.toJob(m)
		if !ok {
			return Job{}, false // unusable posting — dropped, as in Fetch
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			base.Description = "" // liveness refresh only: never rewrite the stored body
			return base, true
		}
		if skip[base.Company] {
			base.Description = "" // aggregator gate will discard it; a detail request is pure loss
			return base, true
		}
		body, outcome := s.details.content(ctx, s.provider, m.host, p.ID)
		switch outcome {
		case detailHydrated:
			base.Description += sanitizeHTML(body) // base.Description is the salary paragraph (or "")
			return base, true
		case detailFailed:
			log.Printf("%s: detail %s/%s failed; deferring the posting to the next crawl", s.provider, e.Region, p.ID)
		}
		// detailStopped is network-wide and was logged once when it happened, not per posting.
		return Job{}, false
	})
}

// crawl pages the search listing until a page yields no posting it has not already collected.
//
// With verifyTotal the walk must end with exactly the distinct postings the search states, and
// one that does not is walked once more before the crawl fails: a posting listed or withdrawn
// while the walk is under way shifts every later page by one, which leaves the count one off
// without anything being wrong with the board. A second mismatch is not that.
func (s seekCrawler) crawl(ctx context.Context, e CompanyEntry) (seekMarket, []seekPosting, error) {
	m, ok := s.markets[strings.ToLower(strings.TrimSpace(e.Region))]
	if !ok {
		return seekMarket{}, nil, fmt.Errorf("%s: company %q has an unknown market (Region) %q", s.provider, e.Company, e.Region)
	}
	out, err := s.walk(ctx, e, m)
	var miscount errSeekMiscount
	if errors.As(err, &miscount) {
		out, err = s.walk(ctx, e, m)
	}
	if err != nil {
		return seekMarket{}, nil, err
	}
	return m, out, nil
}

// errSeekMiscount is a verifyTotal walk that reached the listing's end with a different number of
// distinct postings than the search states.
type errSeekMiscount struct{ msg string }

func (e errSeekMiscount) Error() string { return e.msg }

// walk is one pass over a board's search listing; see crawl.
func (s seekCrawler) walk(ctx context.Context, e CompanyEntry, m seekMarket) ([]seekPosting, error) {
	board := strings.TrimSpace(e.Board)
	var out []seekPosting
	seen := map[string]bool{}
	for page := 1; page <= s.maxPages; page++ {
		var resp seekSearchPage
		if err := s.http.GetJSON(ctx, s.searchURL(m, board, page), &resp); err != nil {
			if page == 1 || s.verifyTotal {
				return nil, fmt.Errorf("%s: search %s subclass %q page %d: %w", s.provider, e.Region, board, page, err)
			}
			break
		}
		added := 0
		for _, p := range resp.Data {
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
			added++
		}
		if added == 0 { // empty page, repeated tail, or SEEK's result window exhausted
			if s.verifyTotal && len(seen) != resp.TotalCount {
				return nil, errSeekMiscount{fmt.Sprintf("%s: search %s subclass %q ended at page %d with %d distinct postings, the search states %d",
					s.provider, e.Region, board, page, len(seen), resp.TotalCount)}
			}
			return out, nil
		}
	}
	if s.verifyTotal {
		return nil, fmt.Errorf("%s: search %s subclass %q exceeded %d pages without reaching the end", s.provider, e.Region, board, s.maxPages)
	}
	return out, nil
}

// seekPrivateAdvertiser is the label SEEK shows for a posting whose employer stays anonymous. It is
// a placeholder, not a company, so a posting offering nothing else is dropped rather than filed
// under it — one bogus company would otherwise collect anonymous postings from both markets.
const seekPrivateAdvertiser = "Private Advertiser"

// employer resolves the posting's company. companyName is the employer SEEK holds a profile for and
// is empty on roughly one posting in thirty, where advertiser.description carries the name the
// employer typed instead. Empty means the posting is unusable.
func (p seekPosting) employer() string {
	name := strings.TrimSpace(firstNonEmpty(p.CompanyName, p.Advertiser.Description))
	if strings.EqualFold(name, seekPrivateAdvertiser) {
		return ""
	}
	return name
}

// workMode maps SEEK's structured work arrangements into our work-mode vocabulary, preferring the
// most remote arrangement a posting offers. An unstated or unrecognized arrangement yields "", so
// the pipeline's location heuristic decides instead of this guessing.
func (p seekPosting) workMode() string {
	set := map[string]bool{}
	for _, a := range p.WorkArrangements.Data {
		set[strings.ToLower(strings.TrimSpace(a.Label.Text))] = true
	}
	switch {
	case set["remote"]:
		return "remote"
	case set["hybrid"]:
		return "hybrid"
	case set["on-site"]:
		return "onsite"
	default:
		return ""
	}
}

// seekEmploymentType maps SEEK's workTypes labels into vocab.EmploymentTypeValues, taking the first
// that maps. An unmapped set (e.g. "Casual/Vacation", which has no counterpart in our vocabulary)
// yields "" so the pipeline's dictionaries decide.
func seekEmploymentType(workTypes []string) string {
	for _, wt := range workTypes {
		switch strings.ToLower(strings.TrimSpace(wt)) {
		case "full time":
			return "full_time"
		case "part time":
			return "part_time"
		case "contract/temp":
			return "contract"
		}
	}
	return ""
}

// salaryParagraph renders the listing's salary label as a leading paragraph, folded into the
// description because the label is free text rather than a structured amount — measured live it
// ranges over "$75,000 – $85,000 per year", "160000", "$1,000/day + super" and "Rates Negotiable",
// none of which Job's structured salary fields can take. Roughly a tenth of the postings that fill
// it use it for marketing copy instead ("Strong remuneration", "Optional 9 day fortnight | Career
// Growth"); it is quoted verbatim anyway, because SEEK labels the field as the salary and second-
// guessing which values are "really" salaries would be exactly the heuristic the structured-facet
// contract forbids. Empty when SEEK states nothing.
func (p seekPosting) salaryParagraph() string {
	label := strings.TrimSpace(p.SalaryLabel)
	if label == "" {
		return ""
	}
	return sanitizeHTML("<p>Salary: " + label + "</p>")
}

// searchURL builds one page of a subclassification search, newest-first.
func (seekCrawler) searchURL(m seekMarket, subclassification string, page int) string {
	q := url.Values{}
	q.Set("siteKey", m.siteKey)
	q.Set("where", m.where)
	q.Set("subclassification", subclassification)
	q.Set("page", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(seekPageSize))
	q.Set("sortmode", "ListedDate")
	return m.host + "/api/jobsearch/v5/search?" + q.Encode()
}

// toJob maps a listing posting to a Job, returning ok=false for an unusable posting (no id to key
// on, or no employer which would break the company slug).
func (p seekPosting) toJob(m seekMarket) (Job, bool) {
	company := p.employer()
	if p.ID == "" || company == "" {
		return Job{}, false
	}
	var location, countryCode string
	if len(p.Locations) > 0 {
		location, countryCode = strings.TrimSpace(p.Locations[0].Label), p.Locations[0].CountryCode
	}
	mode := p.workMode()
	return Job{
		ExternalID:     p.ID,
		URL:            m.host + "/job/" + p.ID,
		Title:          strings.TrimSpace(p.Title),
		Company:        company,
		Location:       location,
		Countries:      countryFromCode(countryCode),
		Description:    p.salaryParagraph(),
		Remote:         mode == "remote",
		WorkMode:       mode,
		EmploymentType: seekEmploymentType(p.WorkTypes),
		PostedAt:       parseRFC3339(p.ListingDate),
	}, true
}
