package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// jobleadsHTTP is the transport surface jobleads needs: a Chrome-fingerprint JSON POST for
// the search endpoint and a fetch()-shaped JSON GET for the detail endpoint. fingerprintHTTP
// satisfies it, and pacedJobleadsPoster wraps it without widening the interface.
type jobleadsHTTP interface {
	PostJSON(ctx context.Context, url string, body, v any) error
	FetchJSON(ctx context.Context, url string, v any) error
}

// jobleads adapts jobleads.com, a multi-country job aggregator. Its search API is
// keyword-first and POST-only: an entry's BOARD is the search keyword, its REGION the
// country filter (alpha-2 code), the same board-as-slice shape as whatjobs/hh. Cloudflare
// country filter (alpha-2 code), the same board-as-slice shape as whatjobs/hh. Cloudflare
// rejects the default Go TLS+HTTP2 fingerprint, so the transport is fingerprintHTTP; keyless, no
// cookies. `totalResultCount` is page-scoped, the vdb ranking rotates between runs and on-topic
// density decays fast with page depth, hence the walk's `added == 0`/`jobleadsMaxPages` bounds
// and the 14-day sweepGrace.
//
// The detail API is paywalled for a logged-out caller: `description` is always "" and
// `company` the "Solo per membri registrati" placeholder, so neither is read. The body is
// assembled from the public jobSummary/responsibilities/qualifications/benefits sections;
// the company always comes from the search feed. Salary is NOT mapped: the feed states
// amounts but no period, and the repo's rule (remotedotcom) drops a period-less salary
// rather than filing it under a period the platform does not state.
type jobleads struct {
	http jobleadsHTTP
	// maxNewPerBoard bounds how many new postings one board hydrates per run; non-positive
	// takes jobleadsMaxNewPerBoard.
	maxNewPerBoard int
	// details caches a fetched description by feed id for the adapter's life (one registry
	// build, i.e. one ingest run). The keyword windows of one country overlap heavily and the
	// pipeline stores a row per board (ids are board-namespaced), so every board that lists a
	// new posting still yields it, but only the first one pays the detail request. nil (a
	// literal jobleads{}) disables the cache.
	details *sync.Map
}

const (
	jobleadsBaseURL   = "https://www.jobleads.com"
	jobleadsSearchURL = jobleadsBaseURL + "/job-search/search"
	jobleadsDetailURL = jobleadsBaseURL + "/api/v3/job/detailsForAppNew/en/%s"
	// jobleadsPageSize is the largest page the search serves (limit 1000 answered 1000 in every
	// one of 41 markets, ~1 s, measured 2026-11-15). The search is the stricter-limited endpoint,
	// so a big page buys the same depth for a tenth of the requests.
	jobleadsPageSize   = 1000
	jobleadsSweepGrace = 14 * 24 * time.Hour
	// jobleadsMaxPages bounds the page walk: loop safety, and a relevance cutoff since on-topic
	// density decays fast with depth. A posting cut here reappears in another keyword's window and
	// the 14-day sweepGrace absorbs the partial walk. A real, shallower window ends the walk on an
	// empty page first.
	jobleadsMaxPages = 5 // 5,000 postings deep, as 50 pages of 100 were
	// jobleadsMaxNewPerBoard bounds one board's detail requests per run. All boards share one
	// ~2 req/s limiter (pacedJobleadsPoster) and a board writes nothing until its crawl returns, so
	// an unbounded backlog outlasts the unit's time budget and stores nothing; a run cut by its
	// deadline closes nothing either (jobleads has no board scope). 779 boards (19 keywords x 41
	// markets) x 25 details plus 5 search pages each is ~23k requests, ~3.2 h at 2 req/s. The
	// rest are deferred (not yielded, still new) to a later run.
	jobleadsMaxNewPerBoard = 25
)

// NewJobleads builds the JobLeads adapter over the given transport.
func NewJobleads(c jobleadsHTTP) Source { return jobleads{http: c, details: &sync.Map{}} }

func (jobleads) Provider() string { return "jobleads" }

// jobleads aggregates postings from many companies, so it stays in the source facet.
func (jobleads) Aggregator() {}

// jobleads is NOT boardless: an entry's board is the search keyword selecting its slice of
// the catalogue, like whatjobs/hh/seek.
//
// The post-run unseen sweep waits jobleadsSweepGrace (14d) instead of the 48h default: the
// vdb ranking rotates between runs and the window end is volatile, so a posting can drift
// out of and back into a keyword's window; the API cannot be liveness-probed either (the
// posting pages sit behind the same Cloudflare edge).
func (jobleads) SweepGrace() time.Duration { return jobleadsSweepGrace }

// jobleadsSearchReq is the site's own frontend request. engineOptions IS load-bearing:
// every 200 carried engineType "vdbSearch", so the adapter always states it.
type jobleadsSearchReq struct {
	Keywords      []string         `json:"keywords"`
	Filters       []jobleadsFilter `json:"filters,omitempty"`
	Limit         int              `json:"limit"`
	Page          int              `json:"page"`
	EngineOptions jobleadsEngine   `json:"engineOptions"`
}

type jobleadsEngine struct {
	EngineType string `json:"engineType"`
}

type jobleadsFilter struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

// jobleadsCountryFilter is the location filter that scopes a search to one market; the
// bare alpha-2 code suffices (a geo circle is optional).
func jobleadsCountryFilter(alpha2 string) jobleadsFilter {
	return jobleadsFilter{
		Key:      "location",
		Operator: "eq",
		Value:    []any{map[string]string{"alpha2Country": alpha2}},
	}
}

// jobleadsSearchResp is the search response; only the postings are read (totalResultCount
// is page-scoped and cannot paginate — see the type doc).
type jobleadsSearchResp struct {
	JobResults []jobleadsPosting `json:"jobResults"`
}

// jobleadsPosting is one posting as the search feed yields it. jobDescription is always
// null in the feed; companyName is the real employer (the detail API paywalls it).
type jobleadsPosting struct {
	ID              string   `json:"id"`
	CompanyName     string   `json:"companyName"`
	JobTitle        string   `json:"jobTitle"`
	JobLink         string   `json:"jobLink"`
	CityName        []string `json:"cityName"`
	RegionName      []string `json:"regionName"`
	Alpha2Country   string   `json:"alpha2Country"`
	ValidFrom       int64    `json:"validFrom"`
	JobLocationType []string `json:"jobLocationType"`
}

// walk pages a board's keyword search, calling handle for every usable posting. The walk
// ends on a page with none (a post-window page answers empty), on a later page's
// failure (canonical rule: first-page failure is board-level, a later one ends the walk
// with what it has), or at jobleadsMaxPages.
func (s jobleads) walk(ctx context.Context, e CompanyEntry, handle func(jobleadsPosting) bool) error {
	for page := 1; page <= jobleadsMaxPages; page++ {
		results, err := s.search(ctx, e, page)
		if err != nil {
			if page == 1 {
				return fmt.Errorf("jobleads: list: %w", err)
			}
			return nil
		}
		added := 0
		for _, p := range results {
			if handle(p) {
				added++
			}
		}
		if added == 0 {
			return nil
		}
	}
	return nil
}

func (s jobleads) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	err := s.walk(ctx, e, func(p jobleadsPosting) bool {
		j, ok := p.toJob()
		if !ok {
			return false
		}
		jobs = append(jobs, j)
		return true
	})
	return jobs, err
}

// FetchNew is the hydrating crawl: the same walk, but a posting's detail is fetched only
// when the catalogue does not already have it. An ingested posting is re-listed as a
// SeenRefresh touch (no body — a content-less re-upsert would wipe the description and the
// facets derived from it); a failed detail keeps the posting list-only (the repo's standard
// rule, re-offered via the HydrationRetryWindow); a posting whose link carries no
// extractable hash is kept list-only without even the request (see jobleadsDetailID). At most
// maxNewPerBoard new postings are taken per run; the rest wait for a later one.
func (s jobleads) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	budget := s.maxNewPerBoard
	if budget <= 0 {
		budget = jobleadsMaxNewPerBoard
	}
	var jobs []Job
	hydrated, deferred := 0, 0
	err := s.walk(ctx, e, func(p jobleadsPosting) bool {
		base, ok := p.toJob()
		if !ok {
			return false
		}
		if seen(p.ID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			return true
		}
		// Another board already fetched it: free, so outside the budget.
		if s.details != nil {
			if d, ok := s.details.Load(p.ID); ok {
				base.Description = d.(string)
				jobs = append(jobs, base)
				return true
			}
		}
		// A deferred posting still counts as found, so the walk goes on to refresh the seen
		// postings further down the window.
		if hydrated == budget {
			deferred++
			return true
		}
		hydrated++
		if detailID := jobleadsDetailID(p.JobLink); detailID != "" {
			d, err := s.fetchDetail(ctx, detailID)
			if err != nil {
				log.Printf("jobleads: detail %s failed; ingesting list-only: %v", p.ID, err)
			} else {
				base.Description = d.description()
				if s.details != nil {
					s.details.Store(p.ID, base.Description)
				}
			}
		}
		jobs = append(jobs, base)
		return true
	})
	if deferred > 0 {
		log.Printf("jobleads: board %q: hydration budget %d reached; %d new postings deferred to a later run",
			e.Board, budget, deferred)
	}
	return jobs, err
}

// search issues one page of the board's keyword search.
func (s jobleads) search(ctx context.Context, e CompanyEntry, page int) ([]jobleadsPosting, error) {
	req := jobleadsSearchReq{
		Keywords: []string{e.Board},
		Limit:    jobleadsPageSize,
		Page:     page,
		EngineOptions: jobleadsEngine{
			EngineType: "vdbSearch",
		},
	}
	if e.Region != "" {
		req.Filters = append(req.Filters, jobleadsCountryFilter(e.Region))
	}
	var resp jobleadsSearchResp
	if err := s.http.PostJSON(ctx, jobleadsSearchURL, req, &resp); err != nil {
		return nil, err
	}
	return resp.JobResults, nil
}

// fetchDetail reads one posting's public detail payload, keyed on the canonical hash the
// posting's own slug carries. The feed's "external-<hex>" id is NOT that key — it is a
// variant-scoped internal handle (the slug hash serves 200 while the feed id serves 404 on
// the same posting).
func (s jobleads) fetchDetail(ctx context.Context, detailID string) (jobleadsDetail, error) {
	var resp struct {
		Payload struct {
			Content jobleadsDetail `json:"content"`
		} `json:"payload"`
	}
	if err := s.http.FetchJSON(ctx, fmt.Sprintf(jobleadsDetailURL, detailID), &resp); err != nil {
		return jobleadsDetail{}, err
	}
	return resp.Payload.Content, nil
}

// jobleadsDetailID extracts the slug's canonical hash: the maximal trailing hex run of the
// path's last segment ("/it/job/<title>--<hex>"). The feed id is the hash minus one leading
// char, so a fixed-length cut truncates and the feed id must never be used (see fetchDetail). A
// run too short to be a hash answers "" and the caller skips the detail call rather than burning
// a 404.
func jobleadsDetailID(jobLink string) string {
	s := strings.TrimSuffix(jobLink, "/")
	i := len(s)
	for i > 0 && strings.ContainsRune(jobleadsHexDigits, rune(s[i-1])) {
		i--
	}
	run := s[i:]
	if len(run) < 32 {
		return ""
	}
	return run
}

// jobleadsHexDigits is the alphabet jobleadsDetailID trims a slug's trailing run against.
const jobleadsHexDigits = "0123456789abcdef"

// toJob maps a feed posting to a Job, returning ok=false for an unusable posting (no id,
// no company — which would break the slug — or no link). The url is the posting's own page
// on jobleads.com; the feed carries no direct employer link.
func (p jobleadsPosting) toJob() (Job, bool) {
	if p.ID == "" || p.CompanyName == "" || p.JobLink == "" {
		return Job{}, false
	}
	workMode := jobleadsWorkMode(p.JobLocationType)
	var postedAt *time.Time
	if p.ValidFrom > 0 {
		t := time.Unix(p.ValidFrom, 0)
		postedAt = &t
	}
	return Job{
		ExternalID: p.ID,
		URL:        jobleadsBaseURL + p.JobLink,
		Title:      p.JobTitle,
		Company:    p.CompanyName,
		Location:   jobleadsLocation(p.CityName, p.RegionName),
		Remote:     workMode == "remote",
		WorkMode:   workMode,
		PostedAt:   postedAt,
		Countries:  countryFromCode(p.Alpha2Country),
	}, true
}

// jobleadsWorkMode maps the first structured jobLocationType the posting states onto
// freehire's work-mode vocabulary. The API sends an array; first-stated wins, and an
// unknown value yields "" so the pipeline's location heuristic decides.
func jobleadsWorkMode(types []string) string {
	for _, t := range types {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "remote":
			return "remote"
		case "hybrid":
			return "hybrid"
		case "in_person":
			return "onsite"
		}
	}
	return ""
}

// jobleadsLocation joins a posting's city and region names. A single city + region
// ("Ferrara, Emilia-Romagna") is the common case; a posting spread over several cities
// keeps just the cities, since no one region covers them.
func jobleadsLocation(cities, regions []string) string {
	if len(cities) == 1 && len(regions) == 1 {
		return cities[0] + ", " + regions[0]
	}
	return strings.Join(cities, ", ")
}

// jobleadsDetail is the public part of the detail response, the sections a logged-out
// caller is served. description and company are paywalled there and never read — see the
// type doc.
type jobleadsDetail struct {
	JobSummary       string   `json:"jobSummary"`
	Responsibilities []string `json:"responsibilities"`
	Qualifications   []string `json:"qualifications"`
	Benefits         []string `json:"benefits"`
}

// description assembles the stored body from the public sections: the HTML jobSummary
// plus the plain-text lists, in the order the site itself renders them, sanitized as one.
func (d jobleadsDetail) description() string {
	var parts []string
	if d.JobSummary != "" {
		parts = append(parts, d.JobSummary)
	}
	if len(d.Responsibilities) > 0 {
		parts = append(parts, strings.Join(d.Responsibilities, "\n"))
	}
	if len(d.Qualifications) > 0 {
		parts = append(parts, strings.Join(d.Qualifications, "\n"))
	}
	if len(d.Benefits) > 0 {
		parts = append(parts, strings.Join(d.Benefits, "\n"))
	}
	if len(parts) == 0 {
		return ""
	}
	return sanitizeHTML(strings.Join(parts, "\n\n"))
}

// pacedJobleadsPoster wraps the fingerprint transport with ONE fresh limiter shared across
// one registry build, so every board's search pages and detail fan-out share one token bucket
// (both hit the same Cloudflare-scored edge). The rate is deliberately gentle: a burst
// re-enters the bot-scoring that 403s/429s the edge, while under-shooting only lengthens a run.
// Measured 2026-11-15 from the crawl host, 60 s each: 2 req/s served every request, 5 req/s had
// 75% of searches refused (429). A rotating proxy does not lift it: the edge limits the client,
// not the address (5 req/s through a fresh exit per request still lost 45% of searches, and
// details at 10-20 req/s plateaued at ~1.4 served a second), and its exits draw extra 403s.
func pacedJobleadsPoster(c jobleadsHTTP) jobleadsHTTP {
	return pacedJobleads{
		c:   c,
		lim: rate.NewLimiter(rate.Every(jobleadsRequestInterval), jobleadsRequestBurst),
	}
}

const (
	jobleadsRequestInterval = 500 * time.Millisecond // ~2 req/s
	jobleadsRequestBurst    = 1
)

// pacedJobleads gates every request on the shared limiter before delegating, so a cancelled
// context surfaces as the Wait error and the inner fetch is skipped.
type pacedJobleads struct {
	c   jobleadsHTTP
	lim waiter
}

func (p pacedJobleads) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.lim.Wait(ctx); err != nil {
		return err
	}
	return p.c.PostJSON(ctx, url, body, v)
}

func (p pacedJobleads) FetchJSON(ctx context.Context, url string, v any) error {
	if err := p.lim.Wait(ctx); err != nil {
		return err
	}
	return p.c.FetchJSON(ctx, url, v)
}
