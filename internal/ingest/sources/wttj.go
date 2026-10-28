package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// wttj adapts Welcome to the Jungle (welcometothejungle.com), a large European job board that
// lists postings from many employers. Board-based aggregator: a board is one of WTTJ's own
// profession SUB-CATEGORY references (e.g. "software-web-development-iMzA4"). The platform files
// every posting under exactly one sub-category, so the boards partition rather than overlap.
//
// The listing is the public Algolia index the site's own search page queries. Its search key is
// published at /api/env and is fetched fresh on every crawl rather than hard-coded, because it is
// the site's to rotate. The key is referer-locked to the site, so every query sends its Referer.
//
// Algolia serves at most 1,000 hits per query (paginationLimitedTo), far fewer than a technical
// sub-category holds, so the walk partitions the board by published_at_timestamp into half-open
// windows small enough to read in one request — see collect. That walk is also what earns
// fullBoardListing: every window's hits are checked against the index's own exact count for that
// window, the windows cover the whole board (checked against the board's unwindowed count), and
// any query failing fails the crawl.
//
// The hit carries no body, so this is a HydratingSource: the posting's description comes from
// WTTJ's public posting API, one request per NEW posting. The same response carries the
// employer's own apply URL, which becomes Job.URL when present. No metering observed: a full
// 7,557-posting board hydrated at 16-way concurrency in 8 minutes (2026-10-15) with no 429,
// only 16 client timeouts — so no pacer is wired, and the fan-out stays at the shared 8-way
// default rather than the width that started timing out.
type wttj struct {
	http wttjHTTP
}

// wttjHTTP is the transport wttj needs: the /api/env script as text, the Algolia query as a POST
// carrying its credential headers, and the posting API as JSON.
type wttjHTTP interface {
	TextGetter
	HeaderJSONPoster
	JSONGetter
}

const (
	wttjEnvURL     = "https://www.welcometothejungle.com/api/env"
	wttjReferer    = "https://www.welcometothejungle.com/"
	wttjIndex      = "wttj_jobs_production_en"
	wttjPostingURL = "https://www.welcometothejungle.com/en/companies/%s/jobs/%s"
	wttjDetailURL  = "https://api.welcometothejungle.com/api/v1/organizations/%s/jobs/%s"

	// wttjHitsCap is both the largest hitsPerPage the index serves and its paginationLimitedTo:
	// a window holding more than this cannot be read whole and is split.
	wttjHitsCap = 1000
	// wttjWindowEnd is the exclusive upper bound of the timestamp space the walk partitions
	// (2^32 seconds, the year 2106). The lower bound is 0. Whether every posting falls inside
	// is not assumed — list checks it against the board's unwindowed count.
	wttjWindowEnd int64 = 1 << 32
	// wttjMaxQueries bounds one board's walk, far above what halving a board of tens of
	// thousands of postings needs; reaching it means the split is not converging, and the
	// crawl fails rather than reporting what it has as the whole board.
	wttjMaxQueries = 2000
)

// wttjAttributes is what a hit is asked for. Restricting it keeps a 1,000-hit window near 1 MB
// instead of the several MB the index's full documents (logos, benefits, highlights) weigh.
var wttjAttributes = []string{
	"objectID", "name", "slug", "organization.name", "organization.slug", "offices", "remote",
	"contract_type", "experience_level_minimum", "salary_minimum", "salary_maximum",
	"salary_currency", "salary_period", "published_at",
}

var (
	// wttjBoardRE is the shape of a sub-category reference: a slug with WTTJ's mixed-case
	// suffix. It also keeps the board from injecting anything into the filter expression.
	wttjBoardRE = regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$`)
	// wttjSlugRE keeps a posting or organization slug to safe path characters, since both are
	// spliced into the posting page and posting API paths.
	wttjSlugRE  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	wttjAppIDRE = regexp.MustCompile(`^[A-Za-z0-9]{6,16}$`)
)

// NewWTTJ builds the Welcome to the Jungle adapter over the given HTTP client.
func NewWTTJ(c wttjHTTP) Source { return wttj{http: c} }

func (wttj) Provider() string { return "wttj" }

// Every posting names its own employer, so wttj stays in the source facet and in the
// cross-source duplicate suppression set.
func (wttj) aggregator() {}

// fullBoardListing: list verifies every window's hits against the index's exact count for that
// window, checks that the windows cover the board's whole unwindowed count, and fails the crawl
// on any query error, on a board that matches nothing, on a single-second window over the cap and
// on reaching wttjMaxQueries. See the fullBoardListing interface (source.go) for the bar.
func (wttj) fullBoardListing() {}

// wttjCreds is the Algolia application id and search key /api/env publishes.
type wttjCreds struct {
	appID, apiKey string
}

func (c wttjCreds) queryURL() string {
	return fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", c.appID, wttjIndex)
}

func (c wttjCreds) headers() map[string]string {
	return map[string]string{
		"X-Algolia-Application-Id": c.appID,
		"X-Algolia-API-Key":        c.apiKey,
		"Referer":                  wttjReferer,
	}
}

// wttjQueryResult is the slice of an Algolia query response the walk reads. NbHits is exact for
// a filter-only query (exhaustiveNbHits was true on every board probed).
type wttjQueryResult struct {
	Hits   []wttjHit `json:"hits"`
	NbHits int       `json:"nbHits"`
}

type wttjOffice struct {
	City        string `json:"city"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
}

// wttjHit is one posting as the index states it. Nullable numbers decode to 0 and nullable
// strings to "", which is what every reader below treats as "not stated".
type wttjHit struct {
	ObjectID     string `json:"objectID"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Organization struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"organization"`
	Offices                []wttjOffice `json:"offices"`
	Remote                 string       `json:"remote"`
	ContractType           string       `json:"contract_type"`
	ExperienceLevelMinimum float64      `json:"experience_level_minimum"`
	SalaryMinimum          float64      `json:"salary_minimum"`
	SalaryMaximum          float64      `json:"salary_maximum"`
	SalaryCurrency         string       `json:"salary_currency"`
	SalaryPeriod           string       `json:"salary_period"`
	PublishedAt            string       `json:"published_at"`
}

// wttjDetail is the posting API's job object, reduced to what the listing lacks.
type wttjDetail struct {
	Job struct {
		WTTJReference      string `json:"wttj_reference"`
		Status             string `json:"status"`
		ApplyURL           string `json:"apply_url"`
		Description        string `json:"description"`
		Profile            string `json:"profile"`
		RecruitmentProcess string `json:"recruitment_process"`
	} `json:"job"`
}

// Fetch hydrates every listed posting. The live pipeline prefers FetchNew; a caller that cannot
// supply a seen set gets the same crawl with nothing seen, because the listing carries no body
// and a body-less row is never searchable.
func (s wttj) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole board and buys a posting's body only when the catalogue lacks it.
func (s wttj) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	hits, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, hits, seen, nil, nil), nil
}

// FetchNewGated is FetchNew told which postings the pipeline will discard anyway. A measured
// third of a technical board applies through an ATS freehire already crawls first-party
// (greenhouse, ashby, smartrecruiters, lever, workday, workable — 1,000 data postings sampled
// 2026-10-14), and those postings are never stored, so without the coverage answer their
// bodies would be bought and thrown away on every run.
func (s wttj) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	hits, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		names := make([]string, 0, len(hits))
		for _, h := range hits {
			names = append(names, h.Organization.Name)
		}
		covered = gate.Covered(names)
	}
	return s.hydrate(ctx, hits, seen, covered, gate.Rejected), nil
}

// hydrate maps the listed hits to Jobs. A seen posting is a liveness refresh with no request; a
// posting the pipeline will discard — its employer covered, or its title rejected as
// non-technical — is yielded body-less for the pipeline to count; every other posting costs one
// posting-API request.
func (s wttj) hydrate(ctx context.Context, hits []wttjHit, seen func(string) bool,
	covered map[string]bool, rejected func(Job) bool) []Job {
	return fetchDetails(hits, defaultDetailWorkers, func(h wttjHit) (Job, bool) {
		base := h.toJob()
		if seen(h.ObjectID) {
			base.SeenRefresh = true
			return base, true
		}
		if covered[h.Organization.Name] || (rejected != nil && rejected(base)) {
			return base, true
		}
		return s.withDetail(ctx, h, base)
	})
}

// withDetail adds the posting API's body and apply URL. A posting the API reports gone (404/410,
// or a status other than published) is dropped. Any other failure defers the NEW posting to the
// next crawl rather than storing a body-less row, which would be re-listed as SeenRefresh forever
// and never hydrate.
func (s wttj) withDetail(ctx context.Context, h wttjHit, base Job) (Job, bool) {
	var d wttjDetail
	err := s.http.GetJSON(ctx, fmt.Sprintf(wttjDetailURL, h.Organization.Slug, h.Slug), &d)
	if err != nil {
		var se *StatusError
		if !errors.As(err, &se) || (se.Code != http.StatusNotFound && se.Code != http.StatusGone) {
			log.Printf("wttj: detail %s failed; deferring: %v", h.ObjectID, err)
		}
		return Job{}, false
	}
	j := d.Job
	// The API is addressed by slugs, the index by uuid: a slug reused for a different posting
	// must not file one posting's body under another's identity.
	if j.WTTJReference != h.ObjectID || j.Status != "published" {
		return Job{}, false
	}
	var parts []string
	for _, p := range []string{j.Description, j.Profile, j.RecruitmentProcess} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	body := sanitizeHTML(strings.Join(parts, "\n"))
	if strings.TrimSpace(body) == "" {
		log.Printf("wttj: detail %s has no body; deferring", h.ObjectID)
		return Job{}, false
	}
	base.Description = body
	if validHTTPURL(j.ApplyURL) {
		base.URL = strings.TrimSpace(j.ApplyURL)
	}
	return base, true
}

// list reads every posting the board holds. It first proves the timestamp windows cover the
// board — the count over [0, wttjWindowEnd) must equal the board's unwindowed count, so a posting
// with no or an out-of-range timestamp cannot silently fall outside every window — and then
// walks the windows.
func (s wttj) list(ctx context.Context, board string) ([]wttjHit, error) {
	if !wttjBoardRE.MatchString(board) {
		return nil, fmt.Errorf("wttj: board %q is not a profession sub-category reference", board)
	}
	creds, err := s.creds(ctx)
	if err != nil {
		return nil, err
	}
	w := wttjWalk{s: s, creds: creds, board: board, budget: wttjMaxQueries, seen: map[string]bool{}}
	total, err := w.query(ctx, nil, 0)
	if err != nil {
		return nil, err
	}
	// A renamed or mistyped reference matches nothing, and must not read as a board whose
	// every posting closed — this adapter's boards are swept board-scoped.
	if total.NbHits == 0 {
		return nil, fmt.Errorf("wttj: board %q matches no postings", board)
	}
	windowed, err := w.query(ctx, &[2]int64{0, wttjWindowEnd}, 0)
	if err != nil {
		return nil, err
	}
	if windowed.NbHits != total.NbHits {
		return nil, fmt.Errorf("wttj: board %q: %d postings, but %d inside the timestamp windows",
			board, total.NbHits, windowed.NbHits)
	}
	if err := w.collect(ctx, 0, wttjWindowEnd, windowed.NbHits); err != nil {
		return nil, err
	}
	return w.out, nil
}

// wttjWalk is one board's window walk: the credentials, the query budget left, and the usable
// hits gathered so far, deduplicated by objectID (a posting republished mid-walk can move from a
// window not yet read into one already read, or the other way).
type wttjWalk struct {
	s      wttj
	creds  wttjCreds
	board  string
	budget int
	seen   map[string]bool
	out    []wttjHit
}

// collect reads the window [lo, hi), whose count is already known. A window within the cap is
// read in one request and its hit count must equal the index's count for it; a larger one is
// halved. A one-second window over the cap cannot be split and fails the crawl.
func (w *wttjWalk) collect(ctx context.Context, lo, hi int64, count int) error {
	if count == 0 {
		return nil
	}
	if count <= wttjHitsCap {
		r, err := w.query(ctx, &[2]int64{lo, hi}, wttjHitsCap)
		if err != nil {
			return err
		}
		if r.NbHits <= wttjHitsCap {
			if len(r.Hits) != r.NbHits {
				return fmt.Errorf("wttj: board %q window [%d,%d): read %d of %d postings",
					w.board, lo, hi, len(r.Hits), r.NbHits)
			}
			w.keep(r.Hits)
			return nil
		}
		count = r.NbHits // it grew past the cap since it was counted: split it below
	}
	if hi-lo <= 1 {
		return fmt.Errorf("wttj: board %q: %d postings share published second %d, over the %d-hit cap",
			w.board, count, lo, wttjHitsCap)
	}
	mid := lo + (hi-lo)/2
	for _, win := range [][2]int64{{lo, mid}, {mid, hi}} {
		r, err := w.query(ctx, &win, 0)
		if err != nil {
			return err
		}
		if err := w.collect(ctx, win[0], win[1], r.NbHits); err != nil {
			return err
		}
	}
	return nil
}

func (w *wttjWalk) keep(hits []wttjHit) {
	for _, h := range hits {
		if !h.usable() || w.seen[h.ObjectID] {
			continue
		}
		w.seen[h.ObjectID] = true
		w.out = append(w.out, h)
	}
}

// query runs one Algolia query over the board, optionally narrowed to the half-open timestamp
// window win. hitsPerPage 0 is a count. distinct is turned off explicitly: the index has an
// attributeForDistinct configured, and with distinct on, a board of 7,577 postings answers 1,531.
func (w *wttjWalk) query(ctx context.Context, win *[2]int64, hitsPerPage int) (wttjQueryResult, error) {
	if w.budget <= 0 {
		return wttjQueryResult{}, fmt.Errorf("wttj: board %q: query budget of %d exhausted before the walk ended",
			w.board, wttjMaxQueries)
	}
	w.budget--
	filter := fmt.Sprintf(`new_profession.sub_category_reference:"%s"`, w.board)
	if win != nil {
		filter += fmt.Sprintf(" AND published_at_timestamp >= %d AND published_at_timestamp < %d", win[0], win[1])
	}
	attrs, _ := json.Marshal(wttjAttributes)
	p := url.Values{}
	p.Set("query", "")
	p.Set("filters", filter)
	p.Set("hitsPerPage", strconv.Itoa(hitsPerPage))
	p.Set("distinct", "false")
	p.Set("analytics", "false")
	p.Set("attributesToRetrieve", string(attrs))
	p.Set("attributesToHighlight", "[]")
	var r wttjQueryResult
	if err := w.s.http.PostJSONWithHeaders(ctx, w.creds.queryURL(), w.creds.headers(),
		map[string]string{"params": p.Encode()}, &r); err != nil {
		return wttjQueryResult{}, fmt.Errorf("wttj: board %q query: %w", w.board, err)
	}
	return r, nil
}

// creds reads the Algolia application id and search key from /api/env, which serves them as a
// "window.env = {...};" script.
func (s wttj) creds(ctx context.Context) (wttjCreds, error) {
	text, err := s.http.GetText(ctx, wttjEnvURL)
	if err != nil {
		return wttjCreds{}, fmt.Errorf("wttj: env: %w", err)
	}
	return parseWTTJEnv(text)
}

func parseWTTJEnv(text string) (wttjCreds, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return wttjCreds{}, fmt.Errorf("wttj: env carries no JSON object")
	}
	var env struct {
		AppID  string `json:"PUBLIC_ALGOLIA_APPLICATION_ID"`
		APIKey string `json:"PUBLIC_ALGOLIA_API_KEY_CLIENT"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &env); err != nil {
		return wttjCreds{}, fmt.Errorf("wttj: env: %w", err)
	}
	appID, key := strings.TrimSpace(env.AppID), strings.TrimSpace(env.APIKey)
	// The app id becomes a hostname, so its shape is checked rather than trusted.
	if !wttjAppIDRE.MatchString(appID) || key == "" {
		return wttjCreds{}, fmt.Errorf("wttj: env carries no usable Algolia credentials")
	}
	return wttjCreds{appID: appID, apiKey: key}, nil
}

// usable reports whether a hit states enough to become a Job: the uuid the dedup key is built
// on, a title, the employer, and the two slugs both the posting page and the posting API are
// addressed by.
func (h wttjHit) usable() bool {
	return h.ObjectID != "" && strings.TrimSpace(h.Name) != "" && strings.TrimSpace(h.Organization.Name) != "" &&
		wttjSlugRE.MatchString(h.Slug) && wttjSlugRE.MatchString(h.Organization.Slug)
}

// toJob maps a listed hit to a Job without a body; the URL is the posting's WTTJ page until the
// posting API names the employer's own apply URL.
func (h wttjHit) toJob() Job {
	codes := make([]string, 0, len(h.Offices))
	for _, o := range h.Offices {
		codes = append(codes, o.CountryCode)
	}
	job := Job{
		ExternalID: h.ObjectID,
		URL:        fmt.Sprintf(wttjPostingURL, h.Organization.Slug, h.Slug),
		Title:      strings.TrimSpace(h.Name),
		Company:    strings.TrimSpace(h.Organization.Name),
		Location: distinctJoin(h.Offices, "; ", func(o wttjOffice) string {
			return joinNonEmpty(o.City, o.Country)
		}),
		WorkMode:       wttjWorkMode(h.Remote),
		EmploymentType: wttjEmploymentType(h.ContractType),
		Countries:      countriesFromCodes(codes),
		PostedAt:       parseRFC3339(h.PublishedAt),
	}
	job.Remote = job.WorkMode == "remote"
	if y := h.ExperienceLevelMinimum; y > 0 && y == math.Trunc(y) {
		years := int(y)
		job.ExperienceYearsMin = &years
	}
	if period := wttjSalaryPeriod(h.SalaryPeriod); period != "" && h.SalaryCurrency != "" {
		min, max := roundSalaryPart(h.SalaryMinimum), roundSalaryPart(h.SalaryMaximum)
		if min != nil || max != nil {
			job.SalaryMin, job.SalaryMax = min, max
			job.SalaryCurrency, job.SalaryPeriod = strings.ToUpper(h.SalaryCurrency), period
		}
	}
	return job
}

// wttjWorkMode maps WTTJ's remote policy. "punctual" (occasional remote work allowed) is not a
// working arrangement and is left to the description, as is "unknown".
func wttjWorkMode(remote string) string {
	switch remote {
	case "fulltime":
		return "remote"
	case "partial":
		return "hybrid"
	case "no":
		return "onsite"
	default:
		return ""
	}
}

// wttjEmploymentType maps WTTJ's contract type onto vocab.EmploymentTypeValues. "temporary" is a
// French CDD (a fixed-term contract) and "freelance" a contractor; apprenticeship, VIE, graduate
// programme and "other" have no value in the vocabulary.
func wttjEmploymentType(t string) string {
	switch t {
	case "full_time":
		return "full_time"
	case "part_time":
		return "part_time"
	case "internship":
		return "internship"
	case "freelance", "temporary":
		return "contract"
	default:
		return ""
	}
}

// wttjSalaryPeriod maps WTTJ's salary period onto vocab.SalaryPeriodValues; an unknown period
// drops the whole salary rather than filing an amount under the wrong one.
func wttjSalaryPeriod(p string) string {
	switch p {
	case "yearly":
		return "year"
	case "monthly":
		return "month"
	case "daily":
		return "day"
	case "hourly":
		return "hour"
	default:
		return ""
	}
}
