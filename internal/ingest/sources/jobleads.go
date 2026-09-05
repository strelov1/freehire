package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// jobleadsHTTP is the transport surface jobleads needs: a Chrome-fingerprint JSON POST for
// the search endpoint and a JSON GET for the detail endpoint. fingerprintHTTP satisfies it,
// and pacedJobleadsPoster wraps it without widening the interface.
type jobleadsHTTP interface {
	PostJSON(ctx context.Context, url string, body, v any) error
	GetJSON(ctx context.Context, url string, v any) error
}

// jobleads adapts jobleads.com, a multi-country job aggregator. Its search API is
// keyword-first and POST-only: an entry's BOARD is the search keyword, its REGION the
// country filter (alpha-2 code), the same board-as-slice shape as whatjobs/hh. Cloudflare
// rejects the default Go TLS+HTTP2 fingerprint (403 for plain-Go, 200 to a Chrome-shaped
// one — verified live 2026-08-30), so the transport is fingerprintHTTP; keyless, no
// cookies. `totalResultCount` is page-scoped, the vdb ranking rotates between runs, and
// on-topic density decays fast (96% topical p1 → ~0% p50, measured) — hence the walk's
// `added == 0`/`jobleadsMaxPages` bounds and the 14-day sweepGrace.
//
// The detail API is paywalled for a logged-out caller: `description` is always "" and
// `company` the "Solo per membri registrati" placeholder, so neither is read. The body is
// assembled from the public jobSummary/responsibilities/qualifications/benefits sections;
// the company always comes from the search feed. Salary is NOT mapped: the feed states
// amounts but no period, and the repo's rule (remotedotcom) drops a period-less salary
// rather than filing it under a period the platform does not state.
type jobleads struct {
	http jobleadsHTTP
}

const (
	jobleadsBaseURL    = "https://www.jobleads.com"
	jobleadsSearchURL  = jobleadsBaseURL + "/job-search/search"
	jobleadsDetailURL  = jobleadsBaseURL + "/api/v3/job/detailsForAppNew/en/%s"
	jobleadsPageSize   = 100
	jobleadsSweepGrace = 14 * 24 * time.Hour
	// jobleadsMaxPages bounds the page walk, measured live 2026-08-30. (1) The loop-safety
	// every paged adapter carries (adp's "bound the paging so a tenant that ignores $skip
	// can't loop"). (2) The relevance cutoff: the vdb's on-topic density decays fast
	// ("Frontend Developer" IT: 96% p1 → 7% p25 → ~0% p50), so page 50 keeps the walk inside
	// the genuinely-dev pool — a posting cut here reappears near the top of another
	// keyword's window, and the 14-day sweepGrace absorbs the partial walk. The walk still
	// ends on an empty page before this when a real window is shallower.
	jobleadsMaxPages = 50
)

// NewJobleads builds the JobLeads adapter over the given transport.
func NewJobleads(c jobleadsHTTP) Source { return jobleads{http: c} }

func (jobleads) Provider() string { return "jobleads" }

// jobleads aggregates postings from many companies, so it stays in the source facet.
func (jobleads) aggregator() {}

// jobleads is NOT boardless: an entry's board is the search keyword selecting its slice of
// the catalogue, like whatjobs/hh/seek.
//
// The post-run unseen sweep waits jobleadsSweepGrace (14d) instead of the 48h default: the
// vdb ranking rotates between runs and the window end is volatile, so a posting can drift
// out of and back into a keyword's window; the API cannot be liveness-probed either (the
// posting pages sit behind the same Cloudflare edge).
func (jobleads) sweepGrace() time.Duration { return jobleadsSweepGrace }

// jobleadsSearchReq is the site's own frontend request. engineOptions IS load-bearing:
// every 200 verified live carried engineType "vdbSearch", so the adapter always states it.
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
// bare alpha-2 code suffices (a geo circle is optional — verified live).
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
// ends on a page with none (a post-window page answers empty — measured), on a later page's
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
// extractable hash is kept list-only without even the request (see jobleadsDetailID).
func (s jobleads) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	var jobs []Job
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
		if detailID := jobleadsDetailID(p.JobLink); detailID != "" {
			d, err := s.fetchDetail(ctx, detailID)
			if err != nil {
				log.Printf("jobleads: detail %s failed; ingesting list-only: %v", p.ID, err)
			} else {
				base.Description = d.description()
			}
		}
		jobs = append(jobs, base)
		return true
	})
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
// the same posting, verified live 2026-08-30).
func (s jobleads) fetchDetail(ctx context.Context, detailID string) (jobleadsDetail, error) {
	var resp struct {
		Payload struct {
			Content jobleadsDetail `json:"content"`
		} `json:"payload"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(jobleadsDetailURL, detailID), &resp); err != nil {
		return jobleadsDetail{}, err
	}
	return resp.Payload.Content, nil
}

// jobleadsDetailID extracts the slug's canonical hash — the maximal trailing hex run of
// the path's last segment ("/it/job/<title>--<hex>"). The slug hash is 33 hex while the
// feed id (confirmed 32 hex) equals it minus one leading char for the canonical variant, so
// a fixed-length cut truncates and the feed id must never be used (see fetchDetail). A
// link whose trailing run is too short to be a hash answers "" and the caller skips the
// detail call rather than burning a 404.
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
// one registry build, so every board's search pages and detail fan-out compete for the same
// token bucket (both paths hit the same Cloudflare-scored edge). The rate is deliberately
// gentle: the true window budget is unknown — under-shooting only lengthens a run, while
// over-shooting re-enters the bot-scoring that 403s/429s the edge (~2 req/s served clean;
// a burstier ~20 req/s verification run 429'd the detail endpoint on the spot, measured
// 2026-09-04). The pace stays on the clean side of that.
func pacedJobleadsPoster(c jobleadsHTTP) jobleadsHTTP {
	return pacedJobleads{
		c:   c,
		lim: rate.NewLimiter(rate.Every(jobleadsRequestInterval), jobleadsRequestBurst),
	}
}

const (
	jobleadsRequestInterval = time.Second // ~1 req/s
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

func (p pacedJobleads) GetJSON(ctx context.Context, url string, v any) error {
	if err := p.lim.Wait(ctx); err != nil {
		return err
	}
	return p.c.GetJSON(ctx, url, v)
}
