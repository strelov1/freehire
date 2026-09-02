package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
)

// dayforce adapts Dayforce (formerly Ceridian) career sites, every one of which is served
// from the single host jobs.dayforcehcm.com under "/<culture>/<tenant>/<site>".
//
// A board is "<tenant>/<site>" — the two path segments a career site's own URL carries, so
// https://jobs.dayforcehcm.com/en-US/dcrusa/join-us is the board "dcrusa/join-us". One tenant
// may run several sites (a public candidateportal plus a branded one), and each is its own
// board. Tenant, site and culture are all case-insensitive at the platform.
//
// An optional third segment names the site's CULTURE ("gm/candidateportal/fr-CA"). The
// listing is scoped to one culture and a site publishes each posting only in the cultures it
// was translated into, so asking a site for one it does not publish in answers an empty list
// — or, on the sites that reject an unconfigured culture outright, HTTP 400. Omitted, the
// culture is en-US, which is what most sites answer to (the English variants collapse: en-GB
// and en-US return the same set); a site that answers to neither names its own.
//
// The culture selects the slice but is NOT part of the board's identity: one posting keeps
// the SAME jobPostingId in every culture it appears in, so two entries differing only in
// culture would store each shared posting twice under two external_id namespaces and then
// close each other's rows on the company-scoped unseen sweep. dayforceSiteID folds the
// culture off so config.go collapses such a pair into one board (see boardIdentity).
//
// The listing is a POST JSON API guarded by next-auth's double-submit CSRF pair: a cookie
// /api/auth/csrf sets, whose token must be echoed back in an X-CSRF-TOKEN header. It carries
// each posting's whole description — the same body the detail endpoint serves, with the HTML
// tags stripped and without the per-board header/footer boilerplate every posting on a board
// repeats — so a crawl needs no per-posting detail request at all.
type dayforce struct {
	http dayforceHTTP
	csrf *dayforceCSRF
}

// dayforceHTTP is the transport dayforce needs: a GET to mint the CSRF pair and a
// header-carrying POST for the listing, where the token rides an X-CSRF-TOKEN header beside
// the cookie the GET set. The real client must persist cookies across calls — see
// cookieSessionSource in registry.go.
type dayforceHTTP interface {
	JSONGetter
	HeaderJSONPoster
}

// NewDayforce builds the Dayforce adapter. In production it is wired with a
// cookie-persisting client (see cookieSessionSource) so the CSRF cookie the bootstrap GET
// sets is sent back with every listing POST.
func NewDayforce(c dayforceHTTP) Source { return dayforce{http: c, csrf: &dayforceCSRF{}} }

func (dayforce) Provider() string { return "dayforce" }

const (
	// dayforceBaseURL is the one host every tenant's career site is served from.
	dayforceBaseURL = "https://jobs.dayforcehcm.com"
	// dayforceDefaultCulture is the culture a board that names none is crawled in.
	dayforceDefaultCulture = "en-US"
	// dayforcePageSize is the listing's page size. It is fixed by the platform — the request
	// carries only a record offset, and a page-size field sent alongside is ignored — so it is
	// the stride list advances by, not a value we choose.
	dayforcePageSize = 25
	// dayforceCSRFPath mints the double-submit pair: it sets the cookie half and returns the
	// token half in its body.
	dayforceCSRFPath = "/api/auth/csrf"
	// dayforceCSRFHeader carries the echoed token that authorizes the listing POST.
	dayforceCSRFHeader = "X-CSRF-TOKEN"
)

// dayforceBoard is a configured board split into the career site's own coordinates.
type dayforceBoard struct {
	tenant, site, culture string
}

// parseDayforceBoard splits "<tenant>/<site>" or "<tenant>/<site>/<culture>", defaulting the
// culture to en-US.
func parseDayforceBoard(board string) (dayforceBoard, error) {
	parts := strings.Split(board, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return dayforceBoard{}, fmt.Errorf("dayforce: board %q must be \"tenant/site\" or \"tenant/site/culture\"", board)
	}
	b := dayforceBoard{tenant: parts[0], site: parts[1], culture: dayforceDefaultCulture}
	if len(parts) == 3 && parts[2] != "" {
		b.culture = parts[2]
	}
	return b, nil
}

// dayforceSiteID folds a board to the career site it addresses, dropping the optional
// culture segment. It is boardIdentity's entry for this provider: the culture chooses which
// translations of a site's postings to read, and the same posting keeps one id across them,
// so two entries differing only in culture are one crawl target — the same reasoning that
// folds iCIMS' two spellings of one host. A board that is not "tenant/site[/culture]" is
// returned unchanged; Fetch rejects it.
func dayforceSiteID(board string) string {
	parts := strings.Split(board, "/")
	if len(parts) != 3 {
		return board
	}
	return parts[0] + "/" + parts[1]
}

// dayforcePosting is one posting in a listing page. jobDescription is the complete body as
// plain text (see the type comment), and postingLocations is null on a posting that has only
// a virtual location.
type dayforcePosting struct {
	JobPostingID     int               `json:"jobPostingId"`
	JobTitle         string            `json:"jobTitle"`
	JobDescription   string            `json:"jobDescription"`
	PostingStartUTC  string            `json:"postingStartTimestampUTC"`
	VirtualLocation  bool              `json:"hasVirtualLocation"`
	PostingLocations []dayforceAddress `json:"postingLocations"`
}

// dayforceAddress is one of a posting's locations. formattedAddress is a full street address
// (postcode included), so the display location is built from the named parts instead.
type dayforceAddress struct {
	CityName       string `json:"cityName"`
	StateCode      string `json:"stateCode"`
	ISOCountryCode string `json:"isoCountryCode"`
}

// label renders one address as "City, State, Country-code", skipping the parts the platform
// left blank.
func (a dayforceAddress) label() string {
	return joinNonEmpty(a.CityName, a.StateCode, a.ISOCountryCode)
}

// location renders the posting's places, several of them joined by "; ". "" for a posting
// with no place at all, which is what a purely virtual one carries.
func (p dayforcePosting) location() string {
	return distinctJoin(p.PostingLocations, "; ", dayforceAddress.label)
}

// countryCodes lists the posting's ISO country codes in listed order, for Job.Countries. A
// posting states as many countries as it states places, and keeping only the first would hide
// it from a filter on any of the others.
func (p dayforcePosting) countryCodes() []string {
	codes := make([]string, 0, len(p.PostingLocations))
	for _, a := range p.PostingLocations {
		codes = append(codes, a.ISOCountryCode)
	}
	return codes
}

// dayforceSearchResponse is one listing page: the postings plus the site's exact live count,
// which bounds pagination.
type dayforceSearchResponse struct {
	JobPostings []dayforcePosting `json:"jobPostings"`
	MaxCount    int               `json:"maxCount"`
}

func (d dayforce) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	b, err := parseDayforceBoard(e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := d.listWithFreshToken(ctx, b)
	if err != nil {
		// The board string, not b's parts: an unconfigured culture is one of the ways a board
		// fails here, so the message has to carry the segment that chose it.
		return nil, fmt.Errorf("dayforce: board %s: %w", e.Board, err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, d.toJob(b, e, p))
	}
	return jobs, nil
}

// listWithFreshToken crawls the board, re-minting the shared CSRF token and trying once more
// if the platform refused. The token is minted once for the whole run, so a token that goes
// stale mid-run would otherwise refuse every board left in it — and only the FIRST page's
// failure is an error here, so the retry re-walks a board that yielded nothing anyway.
func (d dayforce) listWithFreshToken(ctx context.Context, b dayforceBoard) ([]dayforcePosting, error) {
	token, err := d.csrf.get(ctx, d.http, "")
	if err != nil {
		return nil, err
	}
	postings, err := d.list(ctx, token, b)
	if err == nil || !dayforceRefused(err) {
		return postings, err
	}
	if token, err = d.csrf.get(ctx, d.http, token); err != nil {
		return nil, err
	}
	return d.list(ctx, token, b)
}

// list pages the site's listing until maxCount postings have been collected. maxCount is
// exact (confirmed live against a 3,508-posting site: the last page is short and the next one
// is empty), and an empty page also stops the walk, so a count that ever went wrong cannot
// spin the loop. The first page failing is a board-level error; a later page failing ends the
// walk with what was gathered, the repo-wide rule for a paginated listing.
func (d dayforce) list(ctx context.Context, token string, b dayforceBoard) ([]dayforcePosting, error) {
	var postings []dayforcePosting
	for start := 0; ; start += dayforcePageSize {
		page, err := d.searchPage(ctx, token, b, start)
		if err != nil {
			if start == 0 {
				return nil, err
			}
			break
		}
		if len(page.JobPostings) == 0 {
			break
		}
		postings = append(postings, page.JobPostings...)
		if len(postings) >= page.MaxCount {
			break
		}
	}
	return postings, nil
}

// searchPage requests one listing page. The request body is the site's coordinates plus the
// offset — there is no page-size field, and one sent alongside is ignored.
func (d dayforce) searchPage(ctx context.Context, token string, b dayforceBoard, start int) (dayforceSearchResponse, error) {
	body := map[string]any{
		"clientNamespace": b.tenant,
		"jobBoardCode":    b.site,
		"cultureCode":     b.culture,
		"paginationStart": start,
	}
	endpoint := fmt.Sprintf("%s/api/geo/%s/jobposting/search", dayforceBaseURL, b.tenant)
	var page dayforceSearchResponse
	err := d.http.PostJSONWithHeaders(ctx, endpoint, map[string]string{dayforceCSRFHeader: token}, body, &page)
	return page, err
}

// toJob maps one listing posting to a Job. Everything the catalogue needs is in the listing,
// so nothing here issues a request.
func (d dayforce) toJob(b dayforceBoard, e CompanyEntry, p dayforcePosting) Job {
	location := p.location()
	return Job{
		ExternalID: strconv.Itoa(p.JobPostingID),
		URL: fmt.Sprintf("%s/%s/%s/%s/jobs/%d",
			dayforceBaseURL, b.culture, b.tenant, b.site, p.JobPostingID),
		Title:       strings.TrimSpace(p.JobTitle),
		Company:     e.Company,
		Location:    location,
		Description: dayforceDescription(p.JobDescription),
		// hasVirtualLocation is the platform's own remote flag, and it is the only STRUCTURED
		// one: a posting can be both virtual and sited, so its offices stay in Location, and a
		// place merely named "Remote" is the shared heuristic, which must not reach WorkMode.
		Remote:    p.VirtualLocation || isRemote(location),
		WorkMode:  workModeFromRemote(p.VirtualLocation),
		Countries: countriesFromCodes(p.countryCodes()),
		PostedAt:  parseRFC3339(p.PostingStartUTC),
	}
}

// dayforceDescription rebuilds the posting body's structure. The listing serves it as
// entity-encoded plain text — Dayforce strips the tags but not the entities — so the
// entities are decoded first, and the newline-delimited result is rebuilt into paragraphs
// and lists, which plainTextToHTML re-escapes as it goes.
func dayforceDescription(body string) string {
	return sanitizeHTML(plainTextToHTML(html.UnescapeString(body)))
}

// dayforceCSRF caches the double-submit token the whole crawl shares, so a run crawling
// thousands of boards mints it once rather than once per board. The token's other half — the
// cookie — lives in the client's jar, set by the same bootstrap GET, which is why the two
// cannot drift: replacing one replaces the other.
type dayforceCSRF struct {
	mu    sync.Mutex
	token string
}

// token returns the cached token, minting one when the cache is empty. stale is the token the
// caller was refused with (empty when it is simply asking for the first time): the cache is
// dropped only when it still holds that exact token, so however many concurrent boards a
// stale token refused, they re-mint once between them and the rest are handed the new one.
func (c *dayforceCSRF) get(ctx context.Context, http JSONGetter, stale string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.token != stale {
		return c.token, nil
	}
	var resp struct {
		CSRFToken string `json:"csrfToken"`
	}
	c.token = ""
	if err := http.GetJSON(ctx, dayforceBaseURL+dayforceCSRFPath, &resp); err != nil {
		return "", fmt.Errorf("csrf bootstrap: %w", err)
	}
	if resp.CSRFToken == "" {
		return "", fmt.Errorf("csrf bootstrap: %s issued no token", dayforceBaseURL+dayforceCSRFPath)
	}
	c.token = resp.CSRFToken
	return c.token, nil
}

// dayforceRefused reports whether err is the platform REFUSING the request rather than the
// board being absent (404) or the transport failing. 403 is the only shape a stale or missing
// CSRF token takes, so it is the one error worth re-minting for.
func dayforceRefused(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == 403
}
