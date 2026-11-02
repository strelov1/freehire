package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// paradox adapts Paradox ("Olivia"), the conversational-hiring platform frontline employers —
// restaurant franchisees, retail chains, care providers — run their hiring through. A posting
// lives at https://<host>/co/<site>/Job?job_id=<id>, and a board is "<host>/<site>", e.g.
// "olivia.paradox.ai/chickensaladchick1". The host is load-bearing: the platform runs
// separate regional instances (olivia.paradox.ai, olivia.eu1.paradox.ai) and per-customer
// ones (darden.paradox.ai), and a site lives on exactly one of them. Site slugs are
// case-insensitive at the platform, so boards are written in lower case.
//
// The pages themselves sit behind an AWS WAF JavaScript challenge, but the JSON API the
// career site's own front end calls does not — keyless, any User-Agent — provided the request
// carries X-Requested-With: XMLHttpRequest; without it the API path answers 404.
//
//	listing: /api/job-list-site-ui/search-job?compsite_slug=<site>&limit=&page=
//	detail:  /api/job-posting/get_widget_extra_settings?compsite_slug=<site>&job_id=<id>
//
// A site that has never configured a public job list answers the listing with a 404 ("Site X
// is not found") even though its postings resolve one by one; such a board cannot be crawled.
//
// The listing row carries the title, the location and a 250-character preview of the body;
// the full HTML body is only in the detail resource, so this is a HydratingSource. The detail
// also embeds a schema.org JobPosting (google_json_ld) on most sites, which is where the
// posting date and a structured country come from — on the sites that omit it, neither is
// stated anywhere.
//
// fullBoardListing: the walk pages to the board's natural end, any failed page fails the
// crawl, and the distinct postings read must equal the board's own total_jobs.
//
// The API is metered by a per-IP request budget that answers 403 once spent: an unpaced
// hydration of one 1,528-posting site at defaultDetailWorkers read ~840 details in under two
// minutes and was refused the remaining 688 (measured 2026-10-20 on olivia.eu1.paradox.ai);
// the refusal lifted by itself within a few minutes. Every request therefore goes through one
// limiter (pacedParadoxGetter), and the first refusal defers the rest of that board's
// hydration rather than spending more requests against a spent budget.
type paradox struct {
	http HeaderJSONGetter
}

// NewParadox builds the Paradox adapter over the given HTTP client.
func NewParadox(c HeaderJSONGetter) Source { return paradox{http: c} }

// paradoxRequestInterval paces every request of a run — listing pages and details alike — to
// ~1.4 req/s, about half the ~850 requests in a few minutes the budget was seen to admit: the
// headroom is for the true window, which a short burst cannot measure, and for the client's
// own retries beneath the limiter.
const (
	paradoxRequestInterval = 700 * time.Millisecond
	paradoxRequestBurst    = 1
	// Hydrated postings are buffered until FetchNew returns. Leave the pipeline
	// time to persist them before the crawl context expires.
	paradoxPersistenceReserve = 5 * time.Minute
)

// pacedParadoxGetter holds every request of a run to one limiter, wired once per registry
// build.
func pacedParadoxGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(paradoxRequestInterval), paradoxRequestBurst),
	}
}

func (paradox) Provider() string { return "paradox" }

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (paradox) fullBoardListing() {}

const (
	// paradoxPageSize is the listing page size. The server spends roughly 50 ms per row — a
	// 100-row page took 4-11 s, a 500-row page 14-21 s and a 2,500-row page timed out on the
	// server's own side — so pages stay small and the client is the long-timeout one.
	paradoxPageSize = 100
	// paradoxMaxPages is a safety ceiling, not a stopping rule: a board still serving full
	// pages at this depth fails the crawl rather than reporting a truncated listing. The
	// largest site measured (InspireBrands2) was 2,064 postings, 21 pages.
	paradoxMaxPages = 300
)

// paradoxXHR is the header the API requires; the same path without it answers 404.
var paradoxXHR = map[string]string{"X-Requested-With": "XMLHttpRequest"}

var (
	paradoxHostRE = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)*\.paradox\.ai$`)
	paradoxSiteRE = regexp.MustCompile(`^[a-z0-9_.-]+$`)
)

// paradoxBoard is a parsed board id: the platform instance's host and the career site's slug.
type paradoxBoard struct {
	host, site string
}

// parseParadoxBoard splits a "<host>/<site>" board id. The host must be a paradox.ai host: the
// board chooses where the crawl sends requests, so a board naming any other host is refused
// rather than fetched.
func parseParadoxBoard(board string) (paradoxBoard, error) {
	host, site, _ := strings.Cut(strings.ToLower(strings.TrimSpace(board)), "/")
	if !paradoxHostRE.MatchString(host) || !paradoxSiteRE.MatchString(site) {
		return paradoxBoard{}, fmt.Errorf("paradox: board %q must be \"<host>.paradox.ai/<site>\"", board)
	}
	return paradoxBoard{host: host, site: site}, nil
}

func (b paradoxBoard) listingURL(page int) string {
	q := url.Values{}
	q.Set("compsite_slug", b.site)
	q.Set("limit", strconv.Itoa(paradoxPageSize))
	q.Set("page", strconv.Itoa(page))
	return "https://" + b.host + "/api/job-list-site-ui/search-job?" + q.Encode()
}

func (b paradoxBoard) detailURL(id string) string {
	q := url.Values{}
	q.Set("compsite_slug", b.site)
	q.Set("job_id", id)
	return "https://" + b.host + "/api/job-posting/get_widget_extra_settings?" + q.Encode()
}

// paradoxListing is one page of the site's job list.
type paradoxListing struct {
	Success   bool                `json:"success"`
	TotalJobs int                 `json:"total_jobs"`
	Jobs      []paradoxListingRow `json:"jobs"`
}

type paradoxListingRow struct {
	Title    string `json:"job_title"`
	ApplyURL string `json:"original_apply_url"`
	// Locations holds one street address per location the posting is open at
	// ("18900 Limestone Commercial Dr, Suite 100, PFLUGERVILLE TX 78660, United States").
	Locations []string `json:"location_name"`
	// EmploymentTypes is "" when the site does not state one and a list of labels when it
	// does (["Full Time"]), so it is decoded loosely.
	EmploymentTypes any `json:"employment_types"`
}

// paradoxPosting is a listed posting with its identity resolved.
type paradoxPosting struct {
	id  string
	url string
	row paradoxListingRow
}

// paradoxDetail is the posting resource. A job_id the site does not know answers 200 with a
// job object carrying neither a name nor a body.
type paradoxDetail struct {
	Job *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"job"`
	// GoogleJSONLD is a whole <script type="application/ld+json"> element, as a string.
	GoogleJSONLD string `json:"google_json_ld"`
}

// paradoxLD is the subset of the embedded JobPosting the adapter reads.
type paradoxLD struct {
	DatePosted  string `json:"datePosted"`
	JobLocation struct {
		Address struct {
			Country string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

func (s paradox) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole site and fetches a body only for a posting the catalogue does not
// already hold; a seen posting is a liveness refresh that costs no request.
func (s paradox) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseParadoxBoard(e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, b)
	if err != nil {
		return nil, err
	}
	detailCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		detailCtx, cancel = context.WithDeadline(ctx, deadline.Add(-paradoxPersistenceReserve))
		defer cancel()
	}
	// refused is set by the first 403/429: the budget is the IP's, so every further detail
	// request would only be refused too and keep it spent. The rest wait for the next crawl.
	var refused atomic.Bool
	var outOfTime atomic.Bool
	var deferred atomic.Int64
	jobs := fetchDetails(postings, defaultDetailWorkers, func(p paradoxPosting) (Job, bool) {
		if seen(p.id) {
			return Job{ExternalID: p.id, URL: p.url, Title: p.row.Title, Company: e.Company, SeenRefresh: true}, true
		}
		if outOfTime.Load() || detailCtx.Err() != nil {
			deferred.Add(1)
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		if refused.Load() {
			return Job{}, false
		}
		job, ok, err := s.detail(detailCtx, b, e, p)
		// rate.Wait refuses before cancellation if the next token is past the
		// deadline. It exposes no sentinel; the pipeline recognizes the same text.
		if err != nil && (detailCtx.Err() != nil || strings.Contains(err.Error(), "would exceed context deadline")) {
			outOfTime.Store(true)
			deferred.Add(1)
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		if isRateLimited(err) && !refused.Swap(true) {
			log.Printf("paradox: %s: detail refused (%v); deferring the rest of this board's hydration", b, err)
		}
		return job, ok
	})
	if n := deferred.Load(); n > 0 {
		log.Printf("paradox: %s: %d postings deferred to leave time for persistence", b, n)
	}
	return jobs, nil
}

// list walks the site's job list to its natural end — a page shorter than asked for — and
// checks what it read against the total the first page states. Any failed page fails the
// crawl: the adapter claims fullBoardListing, so a truncated walk must never pass for a board
// that shrank.
func (s paradox) list(ctx context.Context, b paradoxBoard) ([]paradoxPosting, error) {
	var (
		out   []paradoxPosting
		ids   = map[string]bool{}
		total int
	)
	for page := 1; ; page++ {
		if page > paradoxMaxPages {
			return nil, fmt.Errorf("paradox: %s: still listing postings after %d pages", b, paradoxMaxPages)
		}
		var r paradoxListing
		if err := s.http.GetJSONWithHeaders(ctx, b.listingURL(page), paradoxXHR, &r); err != nil {
			return nil, fmt.Errorf("paradox: %s: listing page %d: %w", b, page, err)
		}
		if !r.Success {
			return nil, fmt.Errorf("paradox: %s: listing page %d answered success=false", b, page)
		}
		if page == 1 {
			// A page past the end states a total of 0, so only the first page's is the board's.
			total = r.TotalJobs
		}
		for _, row := range r.Jobs {
			p, err := b.posting(row)
			if err != nil {
				return nil, err
			}
			if !ids[p.id] {
				ids[p.id] = true
				out = append(out, p)
			}
		}
		if len(r.Jobs) < paradoxPageSize {
			break
		}
	}
	if len(out) != total {
		return nil, fmt.Errorf("paradox: %s: read %d distinct postings of the %d the site lists", b, len(out), total)
	}
	return out, nil
}

// String names the board in an error.
func (b paradoxBoard) String() string { return b.host + "/" + b.site }

// posting resolves a listing row's identity from its own apply URL, the posting page every
// row links: its job_id is the id the page and the detail resource answer to. A row linking
// anywhere but a posting on this site is not something the adapter understands, so it fails
// the crawl rather than being skipped — a skipped row would read as a closed posting.
func (b paradoxBoard) posting(row paradoxListingRow) (paradoxPosting, error) {
	u, err := url.Parse(row.ApplyURL)
	if err == nil && strings.EqualFold(u.Hostname(), b.host) &&
		strings.EqualFold(u.Path, "/co/"+b.site+"/Job") {
		if id := u.Query().Get("job_id"); id != "" {
			return paradoxPosting{id: id, url: row.ApplyURL, row: row}, nil
		}
	}
	return paradoxPosting{}, fmt.Errorf("paradox: %s: listing row %q links %q, not a posting on this site", b, row.Title, row.ApplyURL)
}

// detail hydrates a new posting. A failure, or an answer carrying no body, defers the posting
// to the next crawl rather than storing the listing's 250-character preview: a hydrating
// adapter never re-reads a body it has stored, so a stub would be permanent. The request's
// error is returned so the caller can tell a refusal from any other failure.
func (s paradox) detail(ctx context.Context, b paradoxBoard, e CompanyEntry, p paradoxPosting) (Job, bool, error) {
	var d paradoxDetail
	if err := s.http.GetJSONWithHeaders(ctx, b.detailURL(p.id), paradoxXHR, &d); err != nil {
		log.Printf("paradox: detail %s on %s failed; deferring: %v", p.id, b, err)
		return Job{}, false, err
	}
	if d.Job == nil || strings.TrimSpace(d.Job.Description) == "" {
		log.Printf("paradox: detail %s on %s carries no body; deferring", p.id, b)
		return Job{}, false, nil
	}
	title := strings.Join(strings.Fields(firstNonEmpty(p.row.Title, d.Job.Name)), " ")
	if title == "" || e.Company == "" {
		log.Printf("paradox: detail %s on %s carries no title or company; deferring", p.id, b)
		return Job{}, false, nil
	}
	location := distinctJoin(p.row.Locations, "; ", strings.TrimSpace)
	job := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(d.Job.Description),
		Remote:         isRemote(location),
		EmploymentType: paradoxEmploymentType(p.row.EmploymentTypes),
	}
	if ld, ok := paradoxJobPosting(d.GoogleJSONLD); ok {
		job.PostedAt = paradoxDate(ld.DatePosted)
		job.Countries = countryFromCode(ld.JobLocation.Address.Country)
	}
	return job, true, nil
}

// paradoxJobPosting decodes the JobPosting the detail embeds as a <script> element string.
func paradoxJobPosting(script string) (paradoxLD, bool) {
	var ld paradoxLD
	if strings.TrimSpace(script) == "" {
		return ld, false
	}
	root, err := html.Parse(strings.NewReader(script))
	if err != nil {
		return ld, false
	}
	return ld, ldJobPosting(root, &ld)
}

// paradoxDate parses the JobPosting's datePosted, which the platform writes with a two-digit
// year ("26-09-23" for 2026-09-23 — every date sampled led with the current year). A
// four-digit ISO date is accepted too, in case a site emits the schema.org form.
func paradoxDate(s string) *time.Time {
	if t := parseDate(s); t != nil {
		return t
	}
	return parseLayout("06-01-02", s)
}

// paradoxEmploymentType maps the listing's employment-type labels onto the vocabulary, only
// when the row states exactly one: "Full Time" was the one label seen across 1,430 rows of 26
// sites, and a row naming several says nothing a single value could carry.
func paradoxEmploymentType(v any) string {
	labels, ok := v.([]any)
	if !ok || len(labels) != 1 {
		return ""
	}
	label, _ := labels[0].(string)
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "full time", "full-time":
		return "full_time"
	case "part time", "part-time":
		return "part_time"
	default:
		return ""
	}
}
