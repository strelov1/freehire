package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

// tyomarkkinatori adapts Job Market Finland (tyomarkkinatori.fi), the Finnish national job
// portal run by KEHA-keskus (the successor to the retired te-palvelut). Like the other national
// feeds (jobdanmark/jobnet/jobtech) it is boardless (one public API, no per-tenant board) and an
// aggregator (every posting carries its own employer), and it covers every sector — the
// downstream dictionaries and the enrich non-tech gate decide relevance, not the adapter.
//
// The catalogue is served by a same-origin JSON API behind a rewriting gateway, discovered by
// capturing the search SPA's own XHR:
//   - list:   POST /api/jobpostingfulltext/search/v2/search — an empty query returns the whole
//     catalogue freshest-first, but the list item omits the posting body.
//   - detail: GET /api/jobposting-new/v1/public/jobpostings/{id} — the body (plain or markdown),
//     the authoritative employer name, and the publish date.
//
// The search is a thin Elasticsearch proxy: it hard-caps deep paging at (pageNumber*pageSize)+
// pageSize ≤ 10000 (the max_result_window), and the whole portal (~10.7k open postings) exceeds
// that. So the crawl SHARDS by region — every region's slice is well under the window and pages
// fully — and dedups the ids a multi-region posting returns in several shards. Region assignment
// is stable across runs, so a posting we ingest via a region shard stays reachable via the same
// shard, which is what keeps the post-run unseen sweep from false-closing the tail (unlike a
// single freshest-10k sweep, whose oldest open postings would age out of view and be closed).
// Region-less postings (rare, fully foreign with no Finnish municipality) simply never enter the
// catalogue — a coverage gap, not a false-close.
//
// Detail is the expensive fan-out (one request per posting over ~10k postings), so the adapter is
// a HydratingSource: FetchNew fetches a posting's detail only when the catalogue does not already
// have it, refreshing a seen posting's liveness without a detail request (see justjoin).
//
// The detail endpoint rate-limits under sustained load: a cold-start crawl that fetches all ~10k
// details trips a 403 after a few hundred requests. So hydration is GRADUAL — the first 403 latches
// off further detail requests for the rest of the run, and an un-hydrated posting is left
// un-ingested (not stored description-less) so it stays unseen and a later run hydrates it once the
// limit has cooled. Over successive hourly runs the catalogue fills up, every posting with a real
// description; in steady state only the daily delta needs a detail request, well under the limit.
type tyomarkkinatori struct {
	http tmtClient
}

// tmtClient is the transport role this adapter needs: a POST search plus a GET JSON detail.
type tmtClient interface {
	JSONPoster
	JSONGetter
}

const (
	tmtSearchURL = "https://tyomarkkinatori.fi/api/jobpostingfulltext/search/v2/search"
	tmtDetailURL = "https://tyomarkkinatori.fi/api/jobposting-new/v1/public/jobpostings/%s"
	// tmtPublicURL is the human-facing posting page (the API urls are not browsable). The trailing
	// segment is the display language, chosen to match the description we picked.
	tmtPublicURL = "https://tyomarkkinatori.fi/henkiloasiakkaat/avoimet-tyopaikat/%s/%s"
	// tmtPageSize is the API's per-page maximum; tmtMaxPage is its pageNumber ceiling. Their
	// product stays under the 10000-item deep-paging window, and every region shard is smaller
	// still, so a shard always pages to exhaustion before either bound bites.
	tmtPageSize = 90
	tmtMaxPage  = 100
	// tmtDetailWorkers bounds the detail fan-out. It is gentler than the shared default because
	// the detail endpoint rate-limits (403) under sustained load; a lower concurrency hydrates
	// more postings before the 403 latch trips, and the latch stops the rest for the run.
	tmtDetailWorkers = 4
)

// tmtRegions is the fixed set of Finnish region (maakunta) codes the search filters on — the 18
// mainland regions plus Åland (21). The two historically-vacant codes (03, 20) are omitted; a
// live-but-empty region would merely return no postings, so the list is a convenience, not a
// correctness dependency. A posting is matched into a region via its municipality, so the shards
// collectively over-cover the catalogue (a multi-region posting appears in several), which the
// id-dedup in listAll collapses.
var tmtRegions = []string{
	"01", "02", "04", "05", "06", "07", "08", "09", "10",
	"11", "12", "13", "14", "15", "16", "17", "18", "19", "21",
}

// NewTyomarkkinatori builds the Job Market Finland adapter over the given POST+GET JSON client.
func NewTyomarkkinatori(c tmtClient) Source { return tyomarkkinatori{http: c} }

func (tyomarkkinatori) Provider() string { return "tyomarkkinatori" }

// tyomarkkinatori is a national portal with one global feed, so its config entry carries no board.
func (tyomarkkinatori) boardless() {}

// tyomarkkinatori aggregates postings from many employers, so it stays in the source facet.
func (tyomarkkinatori) aggregator() {}

// tmtLang is a Finnish public-sector multilingual value: a map of ISO-639-1 code → text, present
// for whichever languages the employer filled in ("fi", "sv", "en", commonly just one).
type tmtLang map[string]string

// tmtLangOrder is the preference order pick/lang walk: English first (this is an English-facing
// aggregator), then Finnish, then Swedish (Finland's second official language).
var tmtLangOrder = []string{"en", "fi", "sv"}

// pick returns the best-available text: the first non-empty value in tmtLangOrder, then any other
// language present, else "".
func (m tmtLang) pick() string {
	for _, k := range tmtLangOrder {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	for _, v := range m {
		if v := strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// lang returns the language code pick would draw from, so the public URL's language segment
// matches the text shown. It defaults to "fi" for an empty value (the portal's primary language).
func (m tmtLang) lang() string {
	for _, k := range tmtLangOrder {
		if strings.TrimSpace(m[k]) != "" {
			return k
		}
	}
	for k, v := range m {
		if strings.TrimSpace(v) != "" {
			return k
		}
	}
	return "fi"
}

// tmtSearchBody is the POST search request. An empty query with a single-region filter returns
// that region's whole slice freshest-first (sorting LATEST).
type tmtSearchBody struct {
	Query   string     `json:"query"`
	Filters tmtFilters `json:"filters"`
	Paging  tmtPaging  `json:"paging"`
	Sorting string     `json:"sorting"`
}

type tmtFilters struct {
	Regions []string `json:"regions"`
}

type tmtPaging struct {
	PageNumber int `json:"pageNumber"`
	PageSize   int `json:"pageSize"`
}

// tmtSearchResponse is one search page: content is the postings, lastPage bounds pagination.
type tmtSearchResponse struct {
	Content  []tmtListItem `json:"content"`
	LastPage int           `json:"lastPage"`
}

// tmtListItem is one list posting. It carries the id, a multilingual title, the employer, the
// location (the only place municipality labels appear — the detail exposes only their codes), and
// the publish date. The body is not in the list; it comes from the detail.
type tmtListItem struct {
	ID       string  `json:"id"`
	Title    tmtLang `json:"title"`
	Employer struct {
		OwnerName       tmtLang `json:"ownerName"`
		OwnerOfficeName string  `json:"ownerOfficeName"`
	} `json:"employer"`
	Location    tmtListLocation `json:"location"`
	PublishDate string          `json:"publishDate"`
}

// tmtListLocation holds the geographic fields the list exposes: municipalities carry their own
// labels (unlike the detail, which has only codes), so the free-text location is built from here.
type tmtListLocation struct {
	ForeignCountry bool `json:"foreignCountry"`
	Municipalities []struct {
		Label tmtLang `json:"label"`
	} `json:"municipalities"`
}

// text builds the free-text location from the municipality labels, appending "Finland" for a
// domestic posting so the geo dictionary resolves the country (a foreign posting keeps only its
// municipality text, which carries its own country).
func (l tmtListLocation) text() string {
	parts := make([]string, 0, len(l.Municipalities)+1)
	for _, m := range l.Municipalities {
		if s := m.Label.pick(); s != "" {
			parts = append(parts, s)
		}
	}
	if !l.ForeignCountry {
		parts = append(parts, "Finland")
	}
	return joinNonEmpty(parts...)
}

// Fetch is the full-hydration crawl (each posting completed with its detail), the fallback for
// callers that do not drive incremental hydration. FetchNew is the hydrating path used by ingest.
func (s tyomarkkinatori) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	items, err := s.listAll(ctx)
	if err != nil {
		return nil, err
	}
	var limited atomic.Bool
	return fetchDetails(items, tmtDetailWorkers, func(it tmtListItem) (Job, bool) {
		return s.hydrate(ctx, it, nil, &limited)
	}), nil
}

// FetchNew is the hydrating crawl: it lists every region shard, but fetches a posting's detail
// (the body the list omits) only for a posting the catalogue does not already have — seen reports
// whether an id is already ingested. A seen posting yields the list-only job flagged SeenRefresh
// (liveness refresh, no detail request, content preserved); an unseen posting is hydrated with
// its detail; a single detail failure is isolated (logged, falling back to list-only).
func (s tyomarkkinatori) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := s.listAll(ctx)
	if err != nil {
		return nil, err
	}
	var limited atomic.Bool
	return fetchDetails(items, tmtDetailWorkers, func(it tmtListItem) (Job, bool) {
		return s.hydrate(ctx, it, seen, &limited)
	}), nil
}

// hydrate maps one list item to a Job. A seen posting is refreshed (SeenRefresh, no detail
// request). An unseen posting is hydrated with its detail — except once the detail endpoint has
// rate-limited this run (limited latched by an earlier 403), an unseen posting is SKIPPED
// (ok=false) so it is left un-ingested and stays unseen for a later run to hydrate. A non-403
// detail failure falls back to list-only so a genuinely broken detail does not lose the posting.
// A nil seen forces hydration for every posting (the Fetch path).
func (s tyomarkkinatori) hydrate(ctx context.Context, it tmtListItem, seen func(string) bool, limited *atomic.Bool) (Job, bool) {
	base, ok := it.toJob()
	if !ok {
		return Job{}, false
	}
	if seen != nil && seen(it.ID) {
		// Already ingested: refresh liveness only. Re-upserting content would wipe the
		// description/facets hydrated when the posting was new (an empty body re-derives to
		// empty facets). base carries just the identity fields.
		base.SeenRefresh = true
		return base, true
	}
	if limited.Load() {
		// The detail endpoint pushed back earlier this run; leave this posting unseen so a
		// later run hydrates it with a real description rather than storing it description-less.
		return Job{}, false
	}
	d, ok, rateLimited := s.detail(ctx, it.ID)
	if rateLimited {
		limited.Store(true)
		return Job{}, false // skip; a later run hydrates it once the limit has cooled
	}
	if !ok {
		log.Printf("tyomarkkinatori: detail %q failed; ingesting list-only", it.ID)
		return base, true
	}
	return d.apply(base), true
}

// listAll pages every region shard and returns the deduplicated list items. A multi-region
// posting is kept once (first shard wins). A first-shard failure aborts the crawl (the API is
// down); a later shard failing ends enumeration with what was gathered, so one region's outage
// does not lose the rest.
func (s tyomarkkinatori) listAll(ctx context.Context) ([]tmtListItem, error) {
	var items []tmtListItem
	seen := make(map[string]bool)
	for i, region := range tmtRegions {
		shard, err := s.listRegion(ctx, region)
		if err != nil {
			if i == 0 {
				return nil, err
			}
			break
		}
		for _, it := range shard {
			if it.ID == "" || seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			items = append(items, it)
		}
	}
	return items, nil
}

// listRegion pages one region's slice freshest-first, stopping on an empty page or the last page.
// Every region is smaller than the deep-paging window, so the loop always terminates on content
// rather than the tmtMaxPage backstop.
func (s tyomarkkinatori) listRegion(ctx context.Context, region string) ([]tmtListItem, error) {
	var items []tmtListItem
	for page := 0; page <= tmtMaxPage; page++ {
		body := tmtSearchBody{
			Filters: tmtFilters{Regions: []string{region}},
			Paging:  tmtPaging{PageNumber: page, PageSize: tmtPageSize},
			Sorting: "LATEST",
		}
		var resp tmtSearchResponse
		if err := s.http.PostJSON(ctx, tmtSearchURL, body, &resp); err != nil {
			return nil, fmt.Errorf("tyomarkkinatori: search region %s page %d: %w", region, page, err)
		}
		if len(resp.Content) == 0 {
			break
		}
		items = append(items, resp.Content...)
		if page >= resp.LastPage {
			break
		}
	}
	return items, nil
}

// toJob maps a list item to the list-only Job (no body): the identity, title, employer, location,
// and publish date. It returns ok=false for an item with no id (would collide on the dedup key),
// no title, or no employer (which would break the company slug). The URL is the public posting
// page in the title's language.
func (it tmtListItem) toJob() (Job, bool) {
	title := it.Title.pick()
	company := firstNonEmpty(it.Employer.OwnerName.pick(), strings.TrimSpace(it.Employer.OwnerOfficeName))
	if it.ID == "" || title == "" || company == "" {
		return Job{}, false
	}
	location := it.Location.text()
	return Job{
		ExternalID: it.ID,
		URL:        fmt.Sprintf(tmtPublicURL, it.ID, it.Title.lang()),
		Title:      title,
		Company:    company,
		Location:   location,
		Remote:     isRemote(location),
		PostedAt:   parseRFC3339(it.PublishDate),
	}, true
}

// tmtDetail is the per-posting detail payload (GET public/jobpostings/{id}). It carries the body
// (plain or markdown, flagged by descriptionsContentType), the authoritative employer name
// (owner.company, present even when the list hides it), and the publish date.
type tmtDetail struct {
	DescriptionsContentType string `json:"descriptionsContentType"`
	Position                struct {
		Title          tmtLang `json:"title"`
		JobDescription tmtLang `json:"jobDescription"`
	} `json:"position"`
	Owner struct {
		Company tmtLang `json:"company"`
	} `json:"owner"`
	Application struct {
		Published string `json:"published"`
	} `json:"application"`
}

// detail fetches a posting's detail. It returns ok=false on a failed request, and rateLimited=true
// specifically when the endpoint answered 403 (its rate-limit response) — the caller latches on
// that to stop hydrating for the rest of the run. Any other failure leaves rateLimited false so the
// caller falls back to list-only.
func (s tyomarkkinatori) detail(ctx context.Context, id string) (d tmtDetail, ok bool, rateLimited bool) {
	if err := s.http.GetJSON(ctx, fmt.Sprintf(tmtDetailURL, id), &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusForbidden {
			return tmtDetail{}, false, true
		}
		return tmtDetail{}, false, false
	}
	return d, true, false
}

// apply enriches a list-derived job with the detail payload: the rendered body becomes the
// description, and the detail's authoritative title/employer/publish date win over the list's
// (each only when present, so a sparse detail never blanks a good list value).
func (d tmtDetail) apply(base Job) Job {
	base.Description = tmtDescription(d.DescriptionsContentType, d.Position.JobDescription.pick())
	if t := d.Position.Title.pick(); t != "" {
		base.Title = t
	}
	if c := d.Owner.Company.pick(); c != "" {
		base.Company = c
	}
	if p := parseRFC3339(d.Application.Published); p != nil {
		base.PostedAt = p
	}
	return base
}

// tmtDescription renders a posting's description body into the sanitized HTML the catalogue
// stores. The body arrives as either plain text or CommonMark markdown, flagged by the detail's
// descriptionsContentType ("plain" or "markdown"):
//   - markdown is rendered (markdownToHTML), so "**bold**" and lists become real tags.
//   - plain text is escaped (it is literal, not HTML) with its newlines turned into <br> — the
//     postings lay out fields line by line ("Location: …\nWork mode: …"), and feeding plain text
//     through the CommonMark renderer would collapse those single newlines into spaces (soft
//     breaks) and merge the lines.
//
// An empty body yields "". The result is always sanitized.
func tmtDescription(contentType, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if strings.EqualFold(contentType, "markdown") {
		return sanitizeHTML(markdownToHTML(body))
	}
	return sanitizeHTML("<p>" + strings.ReplaceAll(html.EscapeString(body), "\n", "<br>") + "</p>")
}
