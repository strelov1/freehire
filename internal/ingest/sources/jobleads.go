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

// jobleads adapts jobleads.com, a multi-country job aggregator, crawled as a multi-company
// source. Its search API is keyword-first and POST-only: there is no per-employer board, so
// an entry's BOARD is the search keyword and its REGION is the market the search is scoped
// to (a location filter on the country's alpha-2 code), the same board-as-slice shape as
// whatjobs/hh. The edge fronts every path with Cloudflare and rejects the default Go
// TLS+HTTP2 fingerprint ("Attention Required" for curl/plain-Go, 200 served to a
// Chrome-shaped one — verified live 2026-08-30), which is why the transport is
// fingerprintHTTP and not the shared client. Keyless: no cookies are needed.
//
// The feed's own `totalResultCount` is always the current page's count — it can neither
// drive pagination nor detect truncation — so the walk stops on a page that yields nothing
// new, the repo's canonical `added == 0` rule. The vdb result window end is volatile
// (measured between page ~200 and ~300 at limit=100 on one query) and the ranking rotates
// between runs (page 1 shared 99 of 100 ids across two identical requests, in a different
// order), so a posting can drift out of and back into a keyword's window: the provider is
// swept on a 14-day grace rather than the default 48 hours.
//
// Descriptions and the company name are PAYWALLED on the detail API for a logged-out
// caller: `description` is always "" and `company` is the "Solo per membri registrati"
// placeholder, so neither is ever read. The public sections it does serve — jobSummary
// (HTML), responsibilities, qualifications, benefits — assemble into the stored
// description; the company always comes from the search feed, which carries it. ApplyForm/
// Skills/Tools/Education are likewise not mapped: the feed's skills/benefits are free-text
// marketing copy (soft skills included), and freehire's dictionaries decide.
//
// Salary is deliberately NOT mapped. The feed states minSalary/maxSalary/salaryCurrency as
// structured fields, but no period anywhere (contractType "full_time" is not one), and this
// repo's rule — remotedotcom's applySalary — is to drop the whole salary rather than file an
// amount under a period the platform does not state; the enrichment pass's own guess decides.
type jobleads struct {
	http jobleadsHTTP
}

const (
	jobleadsBaseURL    = "https://www.jobleads.com"
	jobleadsSearchURL  = jobleadsBaseURL + "/job-search/search"
	jobleadsDetailURL  = jobleadsBaseURL + "/api/v3/job/detailsForAppNew/en/%s"
	jobleadsPageSize   = 100
	jobleadsSweepGrace = 14 * 24 * time.Hour
)

// NewJobleads builds the JobLeads adapter over the given transport.
func NewJobleads(c jobleadsHTTP) Source { return jobleads{http: c} }

func (jobleads) Provider() string { return "jobleads" }

// jobleads aggregates postings from many companies, so it stays in the source facet.
func (jobleads) aggregator() {}

// jobleads is NOT boardless: an entry's board is the search keyword selecting its slice of
// the catalogue, like whatjobs/hh/seek.
//
// The post-run unseen sweep waits jobleadsSweepGrace instead of the 48h default because the
// vdb ranking rotates between runs and the window end is volatile — see the type doc. The
// API cannot be liveness-probed either (the posting pages sit behind the same Cloudflare
// edge), which is the other precondition for a widened grace.
func (jobleads) sweepGrace() time.Duration { return jobleadsSweepGrace }

// jobleadsSearchReq is the search request the site's own frontend sends: keywords plus
// optional filters. engineOptions IS load-bearing — every successful request verified live
// carried engineType "vdbSearch"; a keyword-only body without it was never tested, so the
// adapter always states it.
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
// value uses the alpha-2 code alone (a geo circle is optional — verified live that the bare
// code answers filtered results).
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
// null in the feed; companyName is the real employer (unlike the detail API, where company
// is the paywall placeholder).
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

func (s jobleads) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		results, err := s.search(ctx, e, page)
		if err != nil {
			// The canonical crawlPagedLinks rule: the FIRST page failing is a board-level
			// error, a LATER page failing ends the walk with the links gathered so far.
			if page == 1 {
				return nil, fmt.Errorf("jobleads: list: %w", err)
			}
			return jobs, nil
		}
		added := 0
		for _, p := range results {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
				added++
			}
		}
		if added == 0 {
			// The window ended (a post-window page answers empty, verified live).
			return jobs, nil
		}
	}
}

// FetchNew is the hydrating crawl: it walks the same search pages but fetches a posting's
// detail only when the catalogue does not already have it. A posting already ingested is
// re-listed as a liveness refresh (SeenRefresh, no body — a content-less re-upsert would
// wipe the description and the facets derived from it when it was new); a detail fetch that
// fails still keeps the posting list-only, the repo's standard rule — the HydrationRetryWindow
// re-offers its body on a later run.
func (s jobleads) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		results, err := s.search(ctx, e, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("jobleads: list: %w", err)
			}
			return jobs, nil
		}
		added := 0
		for _, p := range results {
			base, ok := p.toJob()
			if !ok {
				continue
			}
			added++
			if seen(p.ID) {
				base.SeenRefresh = true
				jobs = append(jobs, base)
				continue
			}
			d, err := s.fetchDetail(ctx, jobleadsDetailID(p.JobLink))
			if err != nil {
				log.Printf("jobleads: detail %s failed; ingesting list-only: %v", p.ID, err)
			} else {
				base.Description = d.description()
			}
			jobs = append(jobs, base)
		}
		if added == 0 {
			return jobs, nil
		}
	}
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
// variant-scoped internal handle (two feed ids can sit on one slug, and the slug hash is a
// third value again, all verified live 2026-08-30): the slug hash serves 200 while the feed
// id serves 404 on the same posting.
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

// jobleadsDetailID extracts the posting's canonical hash from its jobLink slug — the
// maximal trailing hex run of the path's last segment ("/it/job/<title>--<hex>"). The
// slug hash is 33 hex while the feed's id shows the same value minus its leading char
// (verified live: slug "…--ecde1148…" serves 200, feed id "external-cde1148…" serves
// 404), so a fixed-length cut truncates. The feed id must never be used for the detail
// key (see fetchDetail); a link whose trailing run is too short to be a hash answers ""
// and the caller skips the detail call rather than burning a 404.
func jobleadsDetailID(jobLink string) string {
	s := strings.TrimSuffix(jobLink, "/")
	i := len(s)
	for i > 0 && strings.ContainsRune(hexDigits, rune(s[i-1])) {
		i--
	}
	run := s[i:]
	if len(run) < 32 {
		return ""
	}
	return run
}

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

// jobleadsLocation joins a posting's city and region names into the free-text location
// field. A single city + region ("Ferrara, Emilia-Romagna") is the common case; a posting
// spread over several cities keeps just the cities, since no one region covers them.
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
// token bucket — both paths hit the same Cloudflare-scored edge. The rate is deliberately
// gentle: the true window budget is unknown, and under-shooting only lengthens a run while
// over-shooting re-enters the bot-scoring that 403s the whole edge. ~40 requests at 2/s
// were served clean in the spike; the pace stays under that.
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
