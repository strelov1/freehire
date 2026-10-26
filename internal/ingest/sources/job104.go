package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// job104 adapts 104 Job Bank (www.104.com.tw), Taiwan's largest job board: several hundred
// thousand postings from every industry. It is one central catalogue, so it is an aggregator
// whose employer is read per posting.
//
// THE BOARD IS ONE OF THE PLATFORM'S OWN AREA CODES — a district (台北市大安區) or, for a
// county or overseas region small enough to read whole, that area. A posting has exactly one
// address, so the areas partition the catalogue and a posting lands under exactly one board.
// The job categories would not: a posting carries up to three, and the pipeline namespaces a
// posting's identity by board, so a category board would store it up to three times.
//
// Measured live on 2026-10-25 (see the "104 traps" section of AGENTS.md):
//
//   - The search reads at most 100 pages of ~30 rows, and its total saturates at 1001. A board
//     whose first page reports 100 pages is split over the job categories (18 families, 44
//     groups, 619 leaves) and then the five disjoint experience brackets, until every slice
//     ends before page 100. A posting under several categories is met in several slices and
//     kept once.
//   - The listing row already carries the full description (the detail endpoint's is the same
//     text), so a crawl is listing pages only.
//   - The filters are soft: promoted rows from anywhere are mixed into a page. A row is kept
//     only when its own address code lies inside the board's area.
//   - Both crawl hosts' IPs are refused by Cloudflare — a plain client and the proxies alike,
//     and a Go Chrome-fingerprint client is shown the challenge — while the Scrapling
//     service's impersonating fetcher is served. It is reached through the service's rotating
//     proxy, because the site rate-limits per address (scraplingtier.go).
type job104 struct {
	http    HeaderJSONGetter
	static  JSONGetter
	limiter waiter
	backoff time.Duration
	codes   *job104Codes
}

const (
	job104SearchURL = "https://www.104.com.tw/jobs/search/api/jobs"
	job104Referer   = "https://www.104.com.tw/jobs/search/"
	job104JobCatURL = "https://static.104.com.tw/category-tool/json/JobCat.json"
	job104AreaURL   = "https://static.104.com.tw/category-tool/json/Area.json"
	// job104MaxPage is the deepest page the search serves; a slice reporting it may hold more.
	job104MaxPage = 100
)

// job104RequestInterval paces every search request of ONE run through one limiter; the
// scheduler runs several shards at once, each with its own. The platform rate-limits per
// address — from the crawl host's own IP, five concurrent shards drew 429s at one request every
// 0.8 s each and still did at 4 s — so the crawl leaves through the Scrapling service's
// rotating proxy (scraplingtier.go), where each request has its own exit. The pace is then
// courtesy toward the site rather than a limit we measured: five shards at 2 s are ~2.5
// requests a second together, and a shard's ~1,000 pages fit its 4500 s budget.
const job104RequestInterval = 2 * time.Second

// job104RateLimitRetries and job104RateLimitBackoff retry a refused (403/429), failed (5xx) or
// unanswered search, the wait
// doubling from 5 s up to retryDelayCap. The Scrapling client retries nothing itself (see
// newScraplingClient), so without this one 429 fails the whole board.
const (
	job104RateLimitRetries = 4
	job104RateLimitBackoff = 5 * time.Second
)

// job104ExpBrackets are the search's experience brackets; each posting falls in exactly one
// (their totals sum exactly to the unfiltered total), so they partition a slice.
var job104ExpBrackets = []string{"1", "3", "5", "10", "99"}

// NewJob104 builds the adapter. search reaches www.104.com.tw; static reaches the code tables
// on static.104.com.tw, which no edge protects.
func NewJob104(search HeaderJSONGetter, static JSONGetter) Source {
	return job104{
		http:    search,
		static:  static,
		limiter: rate.NewLimiter(rate.Every(job104RequestInterval), 1),
		backoff: job104RateLimitBackoff,
		codes:   &job104Codes{},
	}
}

func (job104) Provider() string { return "job104" }

// Many employers post here, each named per posting.
func (job104) aggregator() {}

// job104Code is one node of the platform's category and area trees.
type job104Code struct {
	No       string       `json:"no"`
	Eng      string       `json:"eng"`
	Children []job104Code `json:"n"`
}

// job104Codes memoizes the two code tables for the life of the adapter — one crawl — so every
// board reads them once. Only a successful read is kept, so a transient failure costs one board,
// not the rest of the run.
type job104Codes struct {
	mu      sync.Mutex
	jobCats []job104Code
	areas   map[string]string // area code -> English name
}

func (c *job104Codes) get(ctx context.Context, static JSONGetter) ([]job104Code, map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.jobCats != nil {
		return c.jobCats, c.areas, nil
	}
	var cats, areas []job104Code
	if err := static.GetJSON(ctx, job104JobCatURL, &cats); err != nil {
		return nil, nil, fmt.Errorf("job104: category table: %w", err)
	}
	if err := static.GetJSON(ctx, job104AreaURL, &areas); err != nil {
		return nil, nil, fmt.Errorf("job104: area table: %w", err)
	}
	if len(cats) == 0 || len(areas) == 0 {
		return nil, nil, errors.New("job104: empty code table")
	}
	names := make(map[string]string)
	var walk func(nodes []job104Code)
	walk = func(nodes []job104Code) {
		for _, n := range nodes {
			names[n.No] = n.Eng
			walk(n.Children)
		}
	}
	walk(areas)
	c.jobCats, c.areas = cats, names
	return cats, names, nil
}

type job104Response struct {
	Data     []job104Row `json:"data"`
	Metadata struct {
		Pagination struct {
			LastPage int `json:"lastPage"`
		} `json:"pagination"`
	} `json:"metadata"`
}

// job104Row is one search row; it carries everything the posting states.
type job104Row struct {
	JobName     string  `json:"jobName"`
	CustName    string  `json:"custName"`
	Description string  `json:"description"`
	JobAddrNo   int64   `json:"jobAddrNo"`
	AddrDesc    string  `json:"jobAddrNoDesc"`
	AppearDate  string  `json:"appearDate"`
	SalaryLow   int     `json:"salaryLow"`
	SalaryHigh  int     `json:"salaryHigh"`
	SalaryType  int     `json:"s10"`
	JobRo       int     `json:"jobRo"`
	Period      int     `json:"period"`
	Remote      int     `json:"remoteWorkType"`
	JobCat      []int64 `json:"jobCat"`
	Link        struct {
		Job string `json:"job"`
	} `json:"link"`
}

// Fetch reads every posting addressed inside the board's area. Any request failing fails the
// board: a partial list would read to the sweep as postings gone.
func (s job104) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	area := strings.TrimSpace(e.Board)
	prefix := job104AreaPrefix(area)
	if prefix == "" {
		return nil, fmt.Errorf("job104: board %q is not an area code", e.Board)
	}
	cats, areas, err := s.codes.get(ctx, s.static)
	if err != nil {
		return nil, err
	}
	var (
		seen      = make(map[string]bool)
		jobs      []Job
		truncated int
	)
	add := func(rows []job104Row) {
		for _, r := range rows {
			if !strings.HasPrefix(strconv.FormatInt(r.JobAddrNo, 10), prefix) {
				continue // a promoted row from another area
			}
			j, ok := job104Job(r, areas)
			if !ok || seen[j.ExternalID] {
				continue
			}
			seen[j.ExternalID] = true
			jobs = append(jobs, j)
		}
	}

	// walk reads a slice, or splits it when it reaches the page cap: first down the category
	// tree (nodes are the categories at this depth), then over the experience brackets.
	var walk func(params url.Values, nodes []job104Code, expSplit bool) error
	walk = func(params url.Values, nodes []job104Code, expSplit bool) error {
		first, err := s.search(ctx, params, 1)
		if err != nil {
			return err
		}
		last := first.Metadata.Pagination.LastPage
		if last < job104MaxPage {
			add(first.Data)
			return s.readPages(ctx, params, last, add)
		}
		switch {
		case len(nodes) > 0:
			for _, n := range nodes {
				if err := walk(with104(params, "jobcat", n.No), n.Children, false); err != nil {
					return err
				}
			}
		case !expSplit:
			for _, b := range job104ExpBrackets {
				if err := walk(with104(params, "jobexp", b), nil, true); err != nil {
					return err
				}
			}
		default:
			add(first.Data)
			truncated++
			return s.readPages(ctx, params, job104MaxPage, add)
		}
		return nil
	}
	if err := walk(url.Values{"area": {area}}, cats, false); err != nil {
		return nil, err
	}
	if truncated > 0 {
		log.Printf("job104: board %s: %d slices still reach page %d on every split; read their first %d pages", area, truncated, job104MaxPage, job104MaxPage)
	}
	return jobs, nil
}

// readPages reads pages 2..last of a slice (page 1 is already in hand).
func (s job104) readPages(ctx context.Context, params url.Values, last int, add func([]job104Row)) error {
	for page := 2; page <= last && page <= job104MaxPage; page++ {
		resp, err := s.search(ctx, params, page)
		if err != nil {
			return err
		}
		add(resp.Data)
	}
	return nil
}

func (s job104) search(ctx context.Context, params url.Values, page int) (job104Response, error) {
	q := with104(params, "page", strconv.Itoa(page))
	// Newest first, so a page boundary moves only when a posting is added ahead of it.
	q.Set("order", "2")
	var resp job104Response
	err := retryWhile(ctx, job104RateLimitRetries, s.backoff, isRefusedOrTransient, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		resp = job104Response{}
		return s.http.GetJSONWithHeaders(ctx, job104SearchURL+"?"+q.Encode(), map[string]string{"Referer": job104Referer}, &resp)
	})
	if err != nil {
		return resp, fmt.Errorf("job104: search %s page %d: %w", params.Encode(), page, err)
	}
	return resp, nil
}

func with104(params url.Values, key, value string) url.Values {
	out := make(url.Values, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out.Set(key, value)
	return out
}

// job104AreaPrefix is the leading digits every address code inside an area shares: an area code
// is three-digit groups, and a group of zeros means "the whole of the level above"
// (6001001000 is all of Taipei, 6001001005 one of its districts). It returns "" for a value
// that is not a ten-digit code.
func job104AreaPrefix(area string) string {
	if len(area) != 10 {
		return ""
	}
	if _, err := strconv.ParseUint(area, 10, 64); err != nil {
		return ""
	}
	prefix := area
	for len(prefix) > 4 && strings.HasSuffix(prefix, "000") {
		prefix = prefix[:len(prefix)-3]
	}
	return prefix
}

// job104Job maps a search row onto a Job. The job code in the posting URL is the identity — the
// row's jobNo is a different internal id the detail endpoint does not answer to.
func job104Job(r job104Row, areas map[string]string) (Job, bool) {
	u, err := url.Parse(r.Link.Job)
	if err != nil {
		return Job{}, false
	}
	code := strings.Trim(u.Path, "/")
	code = code[strings.LastIndex(code, "/")+1:]
	if code == "" || strings.TrimSpace(r.JobName) == "" {
		return Job{}, false
	}
	addr := strconv.FormatInt(r.JobAddrNo, 10)
	j := Job{
		ExternalID:     code,
		URL:            "https://www.104.com.tw/job/" + code,
		Title:          strings.TrimSpace(r.JobName),
		Company:        strings.TrimSpace(r.CustName),
		Location:       firstNonEmpty(job104Location(addr, areas), r.AddrDesc),
		Description:    plainTextToHTML(r.Description),
		WorkMode:       job104WorkMode(r.Remote),
		Remote:         r.Remote == 1,
		EmploymentType: map[int]string{1: "full_time", 2: "part_time", 4: "contract"}[r.JobRo],
		IsTechHint:     job104IsTech(r.JobCat),
	}
	if strings.HasPrefix(addr, "6001") {
		j.Countries = []string{"tw"}
	}
	// period is the minimum years of experience plus one; 0 states no requirement.
	if r.Period > 0 {
		years := r.Period - 1
		j.ExperienceYearsMin = &years
	}
	if t, err := time.Parse("20060102", r.AppearDate); err == nil {
		j.PostedAt = &t
	}
	job104Salary(&j, r)
	return j, true
}

// job104Location names the address in English: the district's own name, with the country for
// a Taiwanese one (the platform names overseas areas by country or region already).
func job104Location(addr string, areas map[string]string) string {
	name := areas[addr]
	if name == "" {
		return ""
	}
	if strings.HasPrefix(addr, "6001") {
		return name + ", Taiwan"
	}
	return name
}

// job104IsTech reports whether the platform filed the posting under its IT software and systems
// family (資訊軟體系統類, 2007xxxxxx), whose 37 leaves are all technical crafts.
func job104IsTech(cats []int64) bool {
	for _, c := range cats {
		if c/1000000 == 2007 {
			return true
		}
	}
	return false
}

func job104WorkMode(remote int) string {
	switch remote {
	case 1:
		return "remote"
	case 2:
		return "hybrid"
	default:
		return ""
	}
}

// job104SalaryUnbounded is the platform's "and up" upper bound (月薪60000元以上).
const job104SalaryUnbounded = 9999999

// job104Salary states the posting's pay in TWD, the only currency the platform quotes.
// s10 = 10 is "negotiable", which states no figure.
func job104Salary(j *Job, r job104Row) {
	period := map[int]string{30: "hour", 40: "day", 50: "month", 60: "year", 70: "month"}[r.SalaryType]
	if period == "" {
		return
	}
	j.SalaryMin = roundSalaryPart(float64(r.SalaryLow))
	if r.SalaryHigh != job104SalaryUnbounded {
		j.SalaryMax = roundSalaryPart(float64(r.SalaryHigh))
	}
	if j.SalaryMin == nil && j.SalaryMax == nil {
		return
	}
	j.SalaryCurrency = "TWD"
	j.SalaryPeriod = period
}
