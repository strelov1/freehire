package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// doverBaseURL is the Dover public career-page API root, shared by all three endpoints this
// adapter calls: slug resolution, paginated listing, and per-posting detail. All three answer a
// bare, unauthenticated GET with no bot protection on the data itself (verified live 2026-09-18)
// — only the browser apply-submission flow gates behind Cloudflare Turnstile, which this adapter
// never touches.
const doverBaseURL = "https://app.dover.com/api/v1"

// doverListPageSize is the listing page size; the API accepts a large value (300 observed live)
// and returns a "next" URL when more remain.
const doverListPageSize = 300

// doverClientIDPattern is the platform's client-id shape, the second segment of a
// jobs.dover.com board pair ("Deepspatial/a5546acf-…"); resolveClientID uses it to tell such a
// pair from a plain app.dover.com slug (which may legally carry a slash-free uuid shape too,
// hence the found-from-Cut guard rather than a bare MatchString).
var doverClientIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// doverMaxPages bounds pagination so a server that never empties its "next" link cannot loop
// forever; the same structural cap manatal.go uses for the identical next-URL-walk shape.
const doverMaxPages = 200

// dover adapts Dover-hosted company career pages (app.dover.com). The board id is the company's
// Dover slug (the path segment in /apply/<slug>/<jobId> and /jobs/<slug>); the platform's own
// client id is an internal UUID with no public discovery path other than resolving it from the
// slug, so it is resolved fresh on every crawl rather than cached.
//
// The listing endpoint carries identity and structured facets but no description, compensation,
// or employment type — those live only on the per-job detail call — so dover is a HydratingSource,
// modeled on getro.go: FetchNew fetches detail only for postings the catalogue does not already
// have, and a failed detail fetch falls back to the list-only posting (see detail's doc comment)
// rather than dropping it, matching getro's convention and composing with the pipeline's own
// HYDRATION_RETRY_DAYS retry of description-less rows.
type dover struct {
	http JSONGetter
}

// NewDover builds the Dover adapter over the given HTTP client.
func NewDover(c JSONGetter) Source { return dover{http: c} }

func (dover) Provider() string { return "dover" }

// Fetch is the list-only crawl (no description): kept as the fallback for callers that do not
// drive hydration.
func (s dover) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	clientID, err := s.resolveClientID(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, clientID)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, p.toJob(e))
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it resolves the board's client id, lists its published,
// non-sample postings, then fetches detail (bounded concurrency) only for postings the catalogue
// does not already have. A seen posting is re-listed as a liveness refresh with no detail
// request.
func (s dover) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	clientID, err := s.resolveClientID(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p doverListingPosting) (Job, bool) {
		base := p.toJob(e)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		detail, ok := s.detail(ctx, p.ID)
		if !ok {
			log.Printf("dover: detail %s failed; ingesting list-only", p.ID)
			return base, true
		}
		if !detail.Active || detail.IsPrivate {
			return Job{}, false
		}
		job := detail.toJob(e, base.URL)
		return job, true
	}), nil
}

// resolveClientID resolves a board's Dover slug to the platform's internal client id. A board
// harvested off jobs.dover.com arrives as the pair "<slug>/<client uuid>" — the uuid IS the
// client id, the exact identity the listing endpoint pages under — so it is used directly and
// costs no lookup (and no dependence on the slug still resolving, which the marketing shell
// serves for ANY slug and thus cannot prove).
func (s dover) resolveClientID(ctx context.Context, slug string) (string, error) {
	if _, id, found := strings.Cut(slug, "/"); found && doverClientIDPattern.MatchString(id) {
		return id, nil
	}
	var resp struct {
		ID string `json:"id"`
	}
	url := fmt.Sprintf("%s/careers-page-slug/%s", doverBaseURL, slug)
	if err := s.http.GetJSON(ctx, url, &resp); err != nil {
		return "", fmt.Errorf("dover: resolve slug %s: %w", slug, err)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("dover: slug %s resolved to no client id", slug)
	}
	return resp.ID, nil
}

// doverLocation is one entry of a posting's locations[]: a work arrangement (REMOTE/ONSITE/
// HYBRID) paired with a place, which is either a region (no ISO country) or a specific country.
type doverLocation struct {
	LocationType   string `json:"location_type"`
	LocationOption struct {
		DisplayName  string `json:"display_name"`
		LocationType string `json:"location_type"`
		City         string `json:"city"`
		State        string `json:"state"`
		Country      string `json:"country"`
	} `json:"location_option"`
}

// doverListingPosting is one entry from the paginated listing: identity and structured facets,
// no description.
type doverListingPosting struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Locations     []doverLocation `json:"locations"`
	WorkplaceType string          `json:"workplace_type"`
	IsPublished   bool            `json:"is_published"`
	IsSample      bool            `json:"is_sample"`
}

// doverListingResponse is one page of the paginated listing endpoint.
type doverListingResponse struct {
	Next    string                `json:"next"`
	Results []doverListingPosting `json:"results"`
}

// list pages the listing endpoint by following its own "next" URL literally (as manatal.go
// does) until it reports none, capped at doverMaxPages so a misbehaving "next" cannot loop
// forever. Sample and unpublished postings are excluded here, at the listing stage, so they
// never reach detail hydration.
func (s dover) list(ctx context.Context, clientID string) ([]doverListingPosting, error) {
	next := fmt.Sprintf("%s/careers-page/%s/jobs?limit=%d&offset=0", doverBaseURL, clientID, doverListPageSize)
	var all []doverListingPosting
	for page := 0; page < doverMaxPages && next != ""; page++ {
		var resp doverListingResponse
		if err := s.http.GetJSON(ctx, next, &resp); err != nil {
			return nil, fmt.Errorf("dover: list client %s page %d: %w", clientID, page, err)
		}
		for _, p := range resp.Results {
			if p.IsPublished && !p.IsSample {
				all = append(all, p)
			}
		}
		next = resp.Next
	}
	return all, nil
}

// doverApplyURL builds a posting's public apply-page address, matching the platform's own
// /apply/<slug>/<jobId> shape.
func doverApplyURL(boardSlug, jobID string) string {
	return fmt.Sprintf("https://app.dover.com/apply/%s/%s", boardSlug, jobID)
}

// toJob maps a listing entry to a list-only Job: identity and structured facets, no
// description. Detail hydration fills the rest in FetchNew/detail.
func (p doverListingPosting) toJob(e CompanyEntry) Job {
	mode := workplaceTypeMode(p.WorkplaceType)
	return Job{
		ExternalID: p.ID,
		URL:        doverApplyURL(e.Board, p.ID),
		Title:      strings.TrimSpace(p.Title),
		Company:    e.Company,
		WorkMode:   mode,
		Remote:     mode == "remote",
		Countries:  doverCountriesFromLocations(p.Locations),
	}
}

// doverCountriesFromLocations extracts the ISO2 country codes from a posting's structured
// locations, ignoring region-typed entries (which carry no specific country) per
// countriesFromCodes' own "unresolved code, dropped" contract.
func doverCountriesFromLocations(locs []doverLocation) []string {
	codes := make([]string, 0, len(locs))
	for _, l := range locs {
		if l.LocationOption.LocationType == "COUNTRY" {
			codes = append(codes, l.LocationOption.Country)
		}
	}
	return countriesFromCodes(codes)
}

// doverCompensation is a posting's compensation block. OpenToSharingComp gates whether
// SalaryMin/Max are ever published (see toJob); EquityLowerBound/EquityUpperBound/OffersEquity
// have no home in Job and are folded into the description text instead.
type doverCompensation struct {
	LowerBound        *int    `json:"lower_bound"`
	UpperBound        *int    `json:"upper_bound"`
	CurrencyCode      string  `json:"currency_code"`
	OpenToSharingComp bool    `json:"open_to_sharing_comp"`
	EquityLowerBound  float64 `json:"equity_lower_bound"`
	EquityUpperBound  float64 `json:"equity_upper_bound"`
	OffersEquity      bool    `json:"offers_equity"`
	EmploymentType    string  `json:"employment_type"`
}

// doverDetailResponse is the per-posting detail response: everything the listing omits.
type doverDetailResponse struct {
	ID                      string            `json:"id"`
	Title                   string            `json:"title"`
	UserProvidedDescription string            `json:"user_provided_description"`
	Locations               []doverLocation   `json:"locations"`
	WorkplaceType           string            `json:"workplace_type"`
	Compensation            doverCompensation `json:"compensation"`
	VisaSupport             bool              `json:"visa_support"`
	Active                  bool              `json:"active"`
	IsPrivate               bool              `json:"is_private"`
}

// detail fetches one posting's full detail, returning ok=false when the request fails (in which
// case the caller falls back to the list-only job — see FetchNew).
func (s dover) detail(ctx context.Context, jobID string) (doverDetailResponse, bool) {
	var resp doverDetailResponse
	url := fmt.Sprintf("%s/inbound/application-portal-job/%s", doverBaseURL, jobID)
	if err := s.http.GetJSON(ctx, url, &resp); err != nil {
		return doverDetailResponse{}, false
	}
	return resp, true
}

// toJob maps a hydrated detail onto a full Job. url is the already-built apply URL (the detail
// response carries no board slug to rebuild it from).
func (d doverDetailResponse) toJob(e CompanyEntry, url string) Job {
	mode := workplaceTypeMode(d.WorkplaceType)
	description := sanitizeHTML(d.UserProvidedDescription) + doverDescriptionExtras(d.Compensation, d.VisaSupport)
	job := Job{
		ExternalID:     d.ID,
		URL:            url,
		Title:          strings.TrimSpace(d.Title),
		Company:        e.Company,
		Description:    description,
		WorkMode:       mode,
		Remote:         mode == "remote",
		Countries:      doverCountriesFromLocations(d.Locations),
		EmploymentType: doverEmploymentType(d.Compensation.EmploymentType),
	}
	if d.Compensation.OpenToSharingComp && (d.Compensation.LowerBound != nil || d.Compensation.UpperBound != nil) {
		job.SalaryMin = d.Compensation.LowerBound
		job.SalaryMax = d.Compensation.UpperBound
		job.SalaryCurrency = d.Compensation.CurrencyCode
		job.SalaryPeriod = "year"
	}
	return job
}

// doverEmploymentType maps Dover's compensation.employment_type enum onto freehire's
// employment-type vocabulary, returning "" for an unset/unrecognized value so the description
// parser decides — structured signal only, never a guess.
func doverEmploymentType(t string) string {
	switch t {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "CONTRACT", "TEMPORARY":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}

// doverDescriptionExtras renders the posting facts Job has no dedicated field for — equity
// compensation and visa sponsorship — as sanitized text the caller appends to the (already-HTML)
// description, so a platform-stated fact is never silently dropped for lack of a column. Mirrors
// joppy.go's joppyDescriptionExtras.
func doverDescriptionExtras(c doverCompensation, visaSupport bool) string {
	md := doverDescriptionExtrasMarkdown(c, visaSupport)
	if md == "" {
		return ""
	}
	return sanitizeHTML(markdownToHTML(md))
}

func doverDescriptionExtrasMarkdown(c doverCompensation, visaSupport bool) string {
	var b strings.Builder
	if c.OffersEquity {
		b.WriteString("\n\nThis role offers equity compensation.")
	}
	if visaSupport {
		b.WriteString("\n\nThe employer sponsors a work visa for this role.")
	}
	return b.String()
}
