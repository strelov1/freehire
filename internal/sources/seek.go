package sources

import (
	"context"
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
type seek struct {
	http seekHTTP
}

// seekHTTP is the transport role the adapter needs: a JSON GET for the search listing and a JSON
// POST for the GraphQL detail.
type seekHTTP interface {
	JSONGetter
	JSONPoster
}

// NewSeek builds the SEEK adapter over the shared HTTP client.
func NewSeek(c seekHTTP) Source { return seek{http: c} }

func (seek) Provider() string { return "seek" }

// aggregator marks seek as a genuine multi-company aggregator: a vacancy an employer also posts on
// its own ATS appears here too, so the cross-source dedup pass prefers the first-party copy. Unlike
// the boardless aggregators seek still requires a board (the subclassification id) to bound the
// crawl, so it is not boardless.
func (seek) aggregator() {}

// sweepGrace widens the unseen sweep because the crawl reaches only a SLICE of each subclassification:
// SEEK stops serving results past roughly the 550th (see seekMaxPages), so the busiest slices — five
// of Australia's 22 when this was written, the largest at ~746 postings — have a tail no crawl can
// reach. Ordered newest-first the reachable window covers most of a SEEK advertisement's 30-day run,
// but a posting drifting past it would, on the 48-hour default, be closed and reopened as it drifts
// back, writing a phantom removal into job_daily_stats each cycle. The marker is sound here because
// liveness CANNOT be probed instead: SEEK's own job pages sit behind the same Cloudflare interstitial
// as its search pages.
func (seek) sweepGrace() time.Duration { return 14 * 24 * time.Hour }

const (
	seekPageSize = 100
	// seekMaxPages backstops the page loop at SEEK's result-window cap: at pageSize 100 the edge
	// serves pages 1..5 and answers page 6 with an empty list, so six requests reach the whole
	// window and confirm its end. The walk's real stop condition is a page adding no new posting;
	// this only bounds an edge that keeps answering with fresh inventory.
	seekMaxPages = 6
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

// seekSearchPage is the slice of a search response the adapter reads. totalCount is deliberately
// absent: SEEK reports it as a function of pageSize (the same query answered 36 at pageSize=1, 688
// at pageSize=20 and 666 at pageSize=100), so it can drive neither pagination nor a truncation check.
type seekSearchPage struct {
	Data []seekPosting `json:"data"`
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
func (s seek) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
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
func (s seek) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	m, postings, err := s.crawl(ctx, e)
	if err != nil {
		return nil, err
	}
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
		if body, ok := s.detail(ctx, m, p.ID); ok {
			base.Description += body // base.Description is the salary paragraph (or "")
		} else {
			log.Printf("seek: detail %s/%s failed; ingesting list-only", e.Region, p.ID)
		}
		return base, true
	}), nil
}

// crawl pages the search listing until a page yields no posting it has not already collected.
func (s seek) crawl(ctx context.Context, e CompanyEntry) (seekMarket, []seekPosting, error) {
	m, ok := seekMarkets[strings.ToLower(strings.TrimSpace(e.Region))]
	if !ok {
		return seekMarket{}, nil, fmt.Errorf("seek: company %q has an unknown market (Region) %q", e.Company, e.Region)
	}
	board := strings.TrimSpace(e.Board)
	var out []seekPosting
	seen := map[string]bool{}
	for page := 1; page <= seekMaxPages; page++ {
		var resp seekSearchPage
		if err := s.http.GetJSON(ctx, s.searchURL(m, board, page), &resp); err != nil {
			if page == 1 {
				return seekMarket{}, nil, fmt.Errorf("seek: search %s subclass %q page %d: %w", e.Region, board, page, err)
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
		if added == 0 { // empty page, or SEEK's result window exhausted
			break
		}
	}
	return m, out, nil
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

// seekDetailQuery is the description-fetching GraphQL operation. Its variables are declared exactly
// as used: SEEK's endpoint rejects an unused variable with GRAPHQL_VALIDATION_FAILED rather than
// ignoring it — which is the useful half of the bargain, since a drifted field fails loudly instead
// of silently emptying every body.
const seekDetailQuery = `query jobDetails($jobId: ID!) {
  jobDetails(id: $jobId) {
    job {
      content(platform: WEB)
    }
  }
}`

// seekDetailResponse is the slice of the GraphQL reply the adapter reads.
type seekDetailResponse struct {
	Data struct {
		JobDetails struct {
			Job struct {
				Content string `json:"content"`
			} `json:"job"`
		} `json:"jobDetails"`
	} `json:"data"`
}

// detail fetches a posting's description through SEEK's GraphQL endpoint and returns it sanitized.
// ok is false on a failed request or a reply carrying no content, so the caller falls back to the
// list-only job rather than dropping the posting.
func (s seek) detail(ctx context.Context, m seekMarket, id string) (string, bool) {
	body := map[string]any{
		"operationName": "jobDetails",
		"query":         seekDetailQuery,
		"variables":     map[string]any{"jobId": id},
	}
	var resp seekDetailResponse
	if err := s.http.PostJSON(ctx, m.host+"/graphql", body, &resp); err != nil {
		return "", false
	}
	content := resp.Data.JobDetails.Job.Content
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	return sanitizeHTML(content), true
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
func (seek) searchURL(m seekMarket, subclassification string, page int) string {
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
