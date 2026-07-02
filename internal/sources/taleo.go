package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// taleo adapts Oracle Taleo Enterprise career sites. The board id is the tenant host and
// public careersection number, e.g. "valero.taleo.net/2". Taleo's job feed is keyless but
// session-bound: a GET on the careersection sets a JSESSIONID cookie and embeds the
// careersection's portal code, which the searchjobs POST needs to authorize. The list
// carries no description, so each requisition's detail page is fetched and its description
// decoded from the hidden initialHistory field.
//
// Known seam: some tenants front Taleo behind their own domain (no guessable careersection
// path), and some return "An Error Occurred in TEE" to the searchjobs POST even with a valid
// portal — those are onboarded per-tenant only when the recipe verifies live (see sources/taleo.yml).
type taleo struct {
	http taleoHTTP
	// hosts serializes crawls of the same host. The client's cookie jar is host-scoped, so
	// the JSESSIONID a careersection GET sets is shared by every board on that host; two
	// careersections of one tenant (the host/section board id explicitly allows this) crawled
	// concurrently would clobber each other's session in the jar. Locking per host keeps each
	// board's GET→searchjobs→detail sequence on its own session; distinct hosts still run
	// concurrently and never share a cookie.
	hosts *keyedMutex
}

// taleoHTTP is the transport Taleo needs: a cookie-aware GET (careersection + detail HTML)
// and a header-carrying POST (searchjobs needs the tz headers and the session cookie the GET
// established). The real client must persist cookies across calls; a Go cookiejar is
// host-scoped, so one client segregates cookies across tenants without per-tenant sessions.
type taleoHTTP interface {
	TextGetter
	HeaderJSONPoster
}

// NewTaleo builds the Taleo adapter. In production it is wired with a cookie-persisting
// client (see newCookieClient) so the session cookie carries from the careersection GET
// into the searchjobs POST.
func NewTaleo(c taleoHTTP) Source { return taleo{http: c, hosts: newKeyedMutex()} }

func (taleo) Provider() string { return "taleo" }

// taleoBoard is a configured board split into the tenant host and careersection number.
type taleoBoard struct {
	host, section string
}

// parseTaleoBoard splits "host/section" (e.g. "valero.taleo.net/2").
func parseTaleoBoard(board string) (taleoBoard, error) {
	host, section, ok := strings.Cut(board, "/")
	if !ok || host == "" || section == "" {
		return taleoBoard{}, fmt.Errorf("taleo: board %q must be \"host/section\"", board)
	}
	return taleoBoard{host: host, section: section}, nil
}

// taleoPortalRe extracts the careersection's portal code, embedded in the page as
// portalNo: '101430233'. The portal authorizes the searchjobs POST.
var taleoPortalRe = regexp.MustCompile(`portalNo:\s*'(\d+)'`)

// taleoRequisition is one item from the searchjobs list. column is a positional array whose
// layout is set per careersection: column[0] is always the title, but which cells hold the
// location and posted date varies by tenant (some list neither). locationsColumns is the API's
// own index of the location cell(s), so location is read from it rather than a fixed position.
type taleoRequisition struct {
	JobID            string   `json:"jobId"`
	ContestNo        string   `json:"contestNo"`
	Column           []string `json:"column"`
	LocationsColumns []int    `json:"locationsColumns"`
}

type taleoListResponse struct {
	RequisitionList []taleoRequisition `json:"requisitionList"`
	PagingData      struct {
		TotalCount int `json:"totalCount"`
	} `json:"pagingData"`
}

func (s taleo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	b, err := parseTaleoBoard(e.Board)
	if err != nil {
		return nil, err
	}

	// Hold the host lock across the whole session (careersection GET → searchjobs → detail
	// fan-out) so a same-host sibling board cannot overwrite our JSESSIONID mid-crawl.
	defer s.hosts.lock(b.host)()

	portal, err := s.openSection(ctx, b)
	if err != nil {
		return nil, err
	}

	reqs, err := s.listRequisitions(ctx, b, portal)
	if err != nil {
		return nil, err
	}

	// Description is best-effort: the listing already yields a valid job (title, location,
	// date), so a failed detail fetch leaves the job with an empty description rather than
	// dropping it.
	return fetchDetails(reqs, defaultDetailWorkers, func(r taleoRequisition) (Job, bool) {
		return s.toJob(ctx, e, b, r), true
	}), nil
}

// openSection GETs the careersection to establish the session cookie and returns the portal
// code scraped from the page. Both are required by the searchjobs POST.
func (s taleo) openSection(ctx context.Context, b taleoBoard) (string, error) {
	sectionURL := fmt.Sprintf("https://%s/careersection/%s/jobsearch.ftl?lang=en", b.host, b.section)
	page, err := s.http.GetText(ctx, sectionURL)
	if err != nil {
		return "", fmt.Errorf("taleo: open careersection %s/%s: %w", b.host, b.section, err)
	}
	m := taleoPortalRe.FindStringSubmatch(page)
	if m == nil {
		return "", fmt.Errorf("taleo: no portalNo on careersection %s/%s (not a public section?)", b.host, b.section)
	}
	return m[1], nil
}

// listRequisitions pages through the board's requisitions via searchjobs, stopping when a
// page is empty or the collected count reaches totalCount.
func (s taleo) listRequisitions(ctx context.Context, b taleoBoard, portal string) ([]taleoRequisition, error) {
	url := fmt.Sprintf("https://%s/careersection/rest/jobboard/searchjobs?lang=en&portal=%s", b.host, portal)
	headers := map[string]string{"tz": "GMT+00:00", "tzname": "UTC"}

	var reqs []taleoRequisition
	// taleoMaxPages caps the pagination loop so a bogus totalCount can never spin the crawl
	// indefinitely (cf. the yandex runaway-cursor incident). 200 pages ≈ 5k jobs covers every
	// board we onboard; the loop normally stops far earlier on totalCount or an empty page.
	const taleoMaxPages = 200
	for page := 1; page <= taleoMaxPages; page++ {
		body := taleoSearchBody(page)
		var resp taleoListResponse
		if err := s.http.PostJSONWithHeaders(ctx, url, headers, body, &resp); err != nil {
			return nil, fmt.Errorf("taleo: searchjobs %s page %d: %w", b.host, page, err)
		}
		if len(resp.RequisitionList) == 0 {
			break
		}
		reqs = append(reqs, resp.RequisitionList...)
		if len(reqs) >= resp.PagingData.TotalCount {
			break
		}
	}
	return reqs, nil
}

// taleoSearchBody is the searchjobs request payload for one page: the default (unfiltered)
// search sorted by posting date, at the given 1-based page.
func taleoSearchBody(page int) map[string]any {
	return map[string]any{
		"multilineEnabled": false,
		"sortingSelection": map[string]any{
			"sortBySelectionParam":  "3",
			"ascendingSortingOrder": false,
		},
		"fieldData":                           map[string]any{},
		"filterSelectionParam":                map[string]any{"searchFilterSelections": []any{}},
		"advancedSearchFiltersSelectionParam": map[string]any{"searchFilterSelections": []any{}},
		"pageNo":                              page,
	}
}

// toJob maps one requisition to a Job, fetching its detail page for the description
// (best-effort — a failed or empty detail leaves the description blank).
func (s taleo) toJob(ctx context.Context, e CompanyEntry, b taleoBoard, r taleoRequisition) Job {
	title, location, posted := taleoFields(r)
	detailURL := fmt.Sprintf("https://%s/careersection/%s/jobdetail.ftl?job=%s&lang=en", b.host, b.section, r.ContestNo)

	var description string
	if page, err := s.http.GetText(ctx, detailURL); err == nil {
		description = taleoDescription(page)
	}

	return Job{
		ExternalID:  r.ContestNo,
		URL:         detailURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: description,
		Remote:      isRemote(location),
		PostedAt:    posted,
	}
}

// taleoFields reads a requisition's title, location, and posted date from its variable column
// layout. Title is always column[0]. Location is read from the cells the API names in
// locationsColumns (never a guessed position), each a JSON string-array or a plain value. The
// posted date is the first cell that parses as Taleo's "Jan 2, 2006" listing format, so an
// id/other cell is never misread as a date; a tenant listing no date yields nil.
func taleoFields(r taleoRequisition) (title, location string, posted *time.Time) {
	if len(r.Column) > 0 {
		title = r.Column[0]
	}
	var parts []string
	for _, idx := range r.LocationsColumns {
		if idx >= 0 && idx < len(r.Column) {
			parts = append(parts, taleoLocation(r.Column[idx]))
		}
	}
	location = joinNonEmpty(parts...)
	for _, cell := range r.Column {
		if t := parseLayout("Jan 2, 2006", cell); t != nil {
			posted = t
			break
		}
	}
	return title, location, posted
}

// taleoLocation flattens the JSON string-array location cell into a comma-joined string,
// falling back to the raw cell when it is not a JSON array.
func taleoLocation(cell string) string {
	var locs []string
	if err := json.Unmarshal([]byte(cell), &locs); err != nil {
		return cell
	}
	return joinNonEmpty(locs...)
}

// taleoDescription decodes a jobdetail.ftl page's description. Taleo embeds it in the hidden
// initialHistory input: state fields are "!|!"-delimited and the URL-encoded description HTML
// follows the "!*!" marker. We take the segment after "!*!" up to the next "!|!", percent-decode
// it (PathUnescape leaves "+" intact so tokens like "C++" survive), and sanitize.
func taleoDescription(page string) string {
	value := taleoInitialHistory(page)
	if value == "" {
		return ""
	}
	_, after, ok := strings.Cut(value, "!*!")
	if !ok {
		return ""
	}
	if i := strings.Index(after, "!|!"); i >= 0 {
		after = after[:i]
	}
	decoded, err := url.PathUnescape(after)
	if err != nil {
		decoded = after // keep the encoded form rather than dropping the description
	}
	return sanitizeHTML(decoded)
}

// keyedMutex hands out one mutex per string key, created on demand, so callers serialize work
// sharing a key while distinct keys proceed concurrently.
type keyedMutex struct{ m sync.Map }

func newKeyedMutex() *keyedMutex { return &keyedMutex{} }

// lock acquires the mutex for key and returns its unlock func.
func (k *keyedMutex) lock(key string) func() {
	mu, _ := k.m.LoadOrStore(key, &sync.Mutex{})
	l := mu.(*sync.Mutex)
	l.Lock()
	return l.Unlock
}

// taleoInitialHistory returns the value of the hidden <input name="initialHistory">, or "".
func taleoInitialHistory(page string) string {
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	var value string
	walk(root, func(n *html.Node) bool {
		if value != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "input" && attr(n, "name") == "initialHistory" {
			value = attr(n, "value")
			return false
		}
		return true
	})
	return value
}
