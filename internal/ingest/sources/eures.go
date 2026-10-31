package sources

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// eures adapts the EURES job search API (europa.eu/eures), the EU's own cross-border public
// employment service aggregating vacancies from every EU/EFTA country's national employment
// service (PES) and partner job boards. Keyless. Board-based (one board per EURES-covered
// country); multi-company (each posting's employer comes from the posting itself).
//
// It is registered as an aggregator: verified live that a posting's `source` field and
// `applicationInstructions` trace back to a national PES feed already reachable through another
// freehire adapter (Germany's `DE001` links to the exact reference number `arbeitsagentur`
// already ingests directly) or a private partner board, never a first-party listing of EURES's
// own — the same re-listing shape `whatjobs`/`adzuna` carry the marker for. The cross-source
// dedup pass suppresses an EURES copy wherever a more authoritative source already carries it,
// while EURES postings for a country/employer nothing else reaches still stand on their own.
//
// The search endpoint already returns each posting's full HTML description, translated to
// requestLanguage when a translation exists — no detail fetch needed for title/company/
// description, unlike arbeitsagentur. A per-posting detail fetch resolves the application
// deadline and a human-readable Location: the search result's own locationMap carries opaque NUTS
// region codes, while the detail response's structured place data carries a real city name.
// So it is a HydratingSource: the detail is fetched only for a posting the catalogue does not
// hold, and a stored one is re-listed as a SeenRefresh.
// The IT scope (ICT professionals + ICT technicians) is fixed in code as an ESCO/ISCO occupation
// filter, never board-selectable, so every yielded Job carries IsTechHint.
//
// Every crawl lists a country's WHOLE catalogue, never a publication window. Until 2026-10-28
// the search was narrowed to LAST_THREE_DAYS, so a posting older than three days was never
// listed again by any crawl: measured that day, 40,170 of eures' 42,914 open postings were
// posted before the window, and 22,356 Netherlands postings stood open against the 9,283 the
// country actually carried. The unseen sweep could not tell those apart from removed postings,
// so it closed the live ones of any company that happened to post again inside the window and
// left the rest open forever. A full listing is ~52k postings across all countries, ~1,050
// search pages answering in ~0.3 s each from the crawl host.
//
// The API pages no deeper than euresDepthCap results, and Germany alone carries ~21k. A
// country past the cap is therefore listed through the sub-regions its own POSITION_LOCATION
// facet names — NUTS-1 codes plus "<cc>-NS" for a posting that names no region, each queryable
// as a location code (measured: every child's count matches its own query's total) — and the
// deduplicated union must cover the country's total. That check, and the per-walk check
// against the total the search states, are what earn the fullBoardListing marker.
type eures struct {
	http euresHTTP
	// depthCap is the deepest result the search pages to (euresDepthCap; a field so tests can
	// drive the partitioning path without ten thousand postings).
	depthCap int
}

// euresHTTP is the transport eures needs: a JSON POST for the paginated search, a JSON GET for
// the per-posting detail (location and application deadline).
type euresHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	euresSearchURL = "https://europa.eu/eures/api/jv-searchengine/public/jv-search/search"
	// euresDetailURL is the per-posting detail endpoint, keyed by the search result's own
	// (already unique) id. Fetched for structured location and application deadline.
	euresDetailURL = "https://europa.eu/eures/api/jv-searchengine/public/jv/id/"
	// euresPortalURL is the public EURES portal page for a posting, used as Job.URL. Not
	// documented by the (unofficial) API docs; confirmed live against the current portal.
	euresPortalURL = "https://europa.eu/eures/portal/jv-se/jv-details/"
	// euresPageSize is the API's own maximum resultsPerPage.
	euresPageSize = 50
	// euresDepthCap is the API's measured result-depth cap (confirmed live: page*resultsPerPage
	// beyond 10,000 answers "Too many results were requested" instead of a page of results).
	// A query stating more than this is split by region, never walked part-way.
	euresDepthCap        = 10000
	euresSortSearch      = "MOST_RECENT"
	euresRequestLanguage = "en"
	// euresSessionID is a fixed constant: the field is required by the request schema but
	// no auth/personalization semantics were observed against it.
	euresSessionID = "freehire-ingest"
)

// euresOccupationURIs fixes the adapter's IT scope: ESCO/ISCO occupation-group URIs for "ICT
// professionals" and "ICT technicians". Verified live against the real API that this correctly
// narrows results to technical roles (confirmed titles like "Information Security Analyst",
// dropped titles outside the group). Not board-selectable — EURES has no dedicated IT board of
// its own, unlike profession.hu, so this plays that role as a fixed request filter instead.
var euresOccupationURIs = []string{
	"http://data.europa.eu/esco/isco/C25",
	"http://data.europa.eu/esco/isco/C35",
}

// euresCountryNames maps a board's country code to a display name, for the ~31 EURES-covered
// countries (EU member states plus Iceland, Liechtenstein, Norway, Switzerland). Used to build
// Location when no city/address text is available (or as the whole Location when the detail
// fetch fails). "el" is EURES's own code for Greece, not the ISO 3166-1 "gr" — the EU convention
// for the Greek language code, carried over into EURES's country list.
var euresCountryNames = map[string]string{
	"at": "Austria", "be": "Belgium", "bg": "Bulgaria", "hr": "Croatia", "cy": "Cyprus",
	"cz": "Czechia", "dk": "Denmark", "ee": "Estonia", "fi": "Finland", "fr": "France",
	"de": "Germany", "el": "Greece", "hu": "Hungary", "is": "Iceland", "ie": "Ireland",
	"it": "Italy", "lv": "Latvia", "li": "Liechtenstein", "lt": "Lithuania", "lu": "Luxembourg",
	"mt": "Malta", "nl": "Netherlands", "no": "Norway", "pl": "Poland", "pt": "Portugal",
	"ro": "Romania", "sk": "Slovakia", "si": "Slovenia", "es": "Spain", "se": "Sweden",
	"ch": "Switzerland",
}

// NewEures builds the EURES adapter over the given client. Keyless: registers unconditionally.
func NewEures(c euresHTTP) Source { return eures{http: c, depthCap: euresDepthCap} }

func (eures) Provider() string { return "eures" }

// aggregator marks eures for cross-source dedup suppression — see the eures doc comment.
func (eures) aggregator() {}

// fullBoardListing: every walk is checked against the total the search states, and a country
// past the depth cap is split into regions whose union is checked against the country's total;
// any shortfall fails the crawl. See the eures doc comment and the fullBoardListing interface.
func (eures) fullBoardListing() {}

// euresSearchRequest is one search request. Every filter array the adapter does not use is sent
// as an explicit empty slice, matching the request schema's requirement that these fields be
// present (a nil slice marshals to `null`, which the API has not been tested against and other
// EU-portal-style APIs in this codebase — see emagine.go — are known to 400/500 on for a
// required array field).
type euresSearchRequest struct {
	ResultsPerPage                      int      `json:"resultsPerPage"`
	Page                                int      `json:"page"`
	SortSearch                          string   `json:"sortSearch"`
	Keywords                            []any    `json:"keywords"`
	OccupationUris                      []string `json:"occupationUris"`
	SkillUris                           []string `json:"skillUris"`
	RequiredExperienceCodes             []string `json:"requiredExperienceCodes"`
	PositionScheduleCodes               []string `json:"positionScheduleCodes"`
	SectorCodes                         []string `json:"sectorCodes"`
	EducationAndQualificationLevelCodes []string `json:"educationAndQualificationLevelCodes"`
	PositionOfferingCodes               []string `json:"positionOfferingCodes"`
	LocationCodes                       []string `json:"locationCodes"`
	EuresFlagCodes                      []string `json:"euresFlagCodes"`
	OtherBenefitsCodes                  []string `json:"otherBenefitsCodes"`
	RequiredLanguages                   []string `json:"requiredLanguages"`
	SessionID                           string   `json:"sessionId"`
	RequestLanguage                     string   `json:"requestLanguage"`
}

// euresSearchBody builds the request for one page of a location: a board's country code, or one
// of its sub-region codes when the country is listed region by region.
func euresSearchBody(location string, page int) euresSearchRequest {
	return euresSearchRequest{
		ResultsPerPage:                      euresPageSize,
		Page:                                page,
		SortSearch:                          euresSortSearch,
		Keywords:                            []any{},
		OccupationUris:                      euresOccupationURIs,
		SkillUris:                           []string{},
		RequiredExperienceCodes:             []string{},
		PositionScheduleCodes:               []string{},
		SectorCodes:                         []string{},
		EducationAndQualificationLevelCodes: []string{},
		PositionOfferingCodes:               []string{},
		LocationCodes:                       []string{location},
		EuresFlagCodes:                      []string{},
		OtherBenefitsCodes:                  []string{},
		RequiredLanguages:                   []string{},
		SessionID:                           euresSessionID,
		RequestLanguage:                     euresRequestLanguage,
	}
}

// euresSearchResponse is one search page.
type euresSearchResponse struct {
	NumberRecords int            `json:"numberRecords"`
	Jvs           []euresVacancy `json:"jvs"`
	Facets        struct {
		PositionLocation struct {
			Entries []euresFacetEntry `json:"facetEntriesList"`
		} `json:"POSITION_LOCATION"`
	} `json:"facets"`
}

// euresFacetEntry is one POSITION_LOCATION facet entry: a country and the sub-region codes the
// postings matching the query carry under it.
type euresFacetEntry struct {
	Code     string `json:"code"`
	Children []struct {
		Code string `json:"code"`
	} `json:"childrenList"`
}

// regions returns the sub-region codes the facet names under country.
func (r euresSearchResponse) regions(country string) []string {
	var out []string
	for _, e := range r.Facets.PositionLocation.Entries {
		if !strings.EqualFold(e.Code, country) {
			continue
		}
		for _, c := range e.Children {
			if c.Code != "" {
				out = append(out, c.Code)
			}
		}
	}
	return out
}

// euresVacancy is one search result — already carrying the translated title/description, so no
// detail fetch is needed for these fields.
type euresVacancy struct {
	ID                    string              `json:"id"`
	Title                 string              `json:"title"`
	Description           string              `json:"description"`
	CreationDate          int64               `json:"creationDate"`
	LocationMap           map[string][]string `json:"locationMap"`
	PositionOfferingCode  string              `json:"positionOfferingCode"`
	PositionScheduleCodes []string            `json:"positionScheduleCodes"`
	Employer              euresEmployer       `json:"employer"`
}

// euresEmployer is null for most postings of some national feeds, and then Company stays empty
// on purpose. Measured 2026-10-30: 10,166 of the Netherlands' 10,242 open postings (the UWV
// werk.nl feed, documentId WERKNL…) and 269 of Spain's 529 carry no employer, and the detail
// response has no employer block for them either — it carries one exactly when the search does.
// What the Dutch detail does carry is a contact person whose familyName is sometimes an agency
// ("Spilberg"), plus an email and a webProfiles link that mostly point at a recruiter or a job
// board (joof.nl, actiever.nl); none of them names the employer, so none is read as one.
type euresEmployer struct {
	Name string `json:"name"`
}

// euresDetailResponse carries location and application deadline. requestLang does NOT select
// a translated profile the way the search endpoint's
// requestLanguage does (confirmed live: jvProfiles only ever carried preferredLanguage's own
// key), so the profile to read is always the one keyed by PreferredLanguage.
type euresDetailResponse struct {
	PreferredLanguage string                  `json:"preferredLanguage"`
	JvProfiles        map[string]euresProfile `json:"jvProfiles"`
}

type euresProfile struct {
	Locations           []euresLocation `json:"locations"`
	LastApplicationDate int64           `json:"lastApplicationDate"`
}

// euresLocation is one place entry. CityName and AddressLines[0] are both nullable/absent on
// live data (confirmed for France and Poland respectively) — see euresLocationText.
type euresLocation struct {
	CountryCode  string   `json:"countryCode"`
	CityName     string   `json:"cityName"`
	AddressLines []string `json:"addressLines"`
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (e eures) Fetch(ctx context.Context, entry CompanyEntry) ([]Job, error) {
	return e.FetchNew(ctx, entry, func(string) bool { return false })
}

// FetchNew lists the board's whole country and fetches a posting's detail only when the
// catalogue does not hold it; a seen posting is re-listed as a SeenRefresh.
func (e eures) FetchNew(ctx context.Context, entry CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	country := strings.ToLower(strings.TrimSpace(entry.Board))
	vacancies, err := e.list(ctx, country)
	if err != nil {
		return nil, fmt.Errorf("eures: board %q: %w", entry.Board, err)
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v euresVacancy) (Job, bool) {
		if seen(v.ID) {
			j := v.job()
			j.SeenRefresh = true
			return j, true
		}
		return e.toJob(ctx, entry.Board, v), true
	}), nil
}

// list reads every posting of country: in one walk when the country fits under the depth cap,
// otherwise region by region. A region walk proves only its own region, so the union must also
// account for the country's total — the lower of the totals stated before and after the regions
// are walked, so a posting withdrawn meanwhile does not fail the board. The union is counted in
// distinct postings plus the repeats the region walks read: the search counts a posting it
// indexes twice twice (see walk), so distinct postings alone can never reach such a total.
func (e eures) list(ctx context.Context, country string) ([]euresVacancy, error) {
	first, err := e.search(ctx, country, 1)
	if err != nil {
		return nil, err
	}
	if first.NumberRecords <= e.depthCap {
		vs, _, err := e.walk(ctx, country, first)
		return vs, err
	}
	regions := first.regions(country)
	if len(regions) == 0 {
		return nil, fmt.Errorf("%d postings exceed the %d-result depth cap and the search names no region to split them by", first.NumberRecords, e.depthCap)
	}
	var out []euresVacancy
	listed := map[string]bool{}
	repeats := 0
	for _, region := range regions {
		rfirst, err := e.search(ctx, region, 1)
		if err != nil {
			return nil, err
		}
		if rfirst.NumberRecords > e.depthCap {
			return nil, fmt.Errorf("region %q: %d postings exceed the %d-result depth cap", region, rfirst.NumberRecords, e.depthCap)
		}
		vs, r, err := e.walk(ctx, region, rfirst)
		if err != nil {
			return nil, err
		}
		repeats += r
		for _, v := range vs {
			if !listed[v.ID] {
				listed[v.ID] = true
				out = append(out, v)
			}
		}
	}
	recount, err := e.search(ctx, country, 1)
	if err != nil {
		return nil, err
	}
	if recount.NumberRecords == 0 {
		return nil, fmt.Errorf("the country's recount states no results after %d", first.NumberRecords)
	}
	if want := min(first.NumberRecords, recount.NumberRecords); len(out)+repeats < want {
		return nil, fmt.Errorf("regions %v listed %d postings (%d repeated) of the country's %d", regions, len(out), repeats, want)
	}
	return out, nil
}

// walk pages one location from its already-fetched first page to the search's natural end — a
// short page — and fails unless the positions read reach the total the search states. It counts
// POSITIONS, not distinct postings, because the search counts a posting it indexes twice twice:
// measured live 2026-10-28, region "dea" stated 4,731 results and served 4,730 distinct ids on
// every walk. The lowest total any page stated is the bar: a posting withdrawn mid-walk shifts
// the pages and lowers the total together, while a truncated walk falls short of either. It
// returns the distinct postings and how many positions repeated one already read.
func (e eures) walk(ctx context.Context, location string, first euresSearchResponse) ([]euresVacancy, int, error) {
	var out []euresVacancy
	listed := map[string]bool{}
	read := 0
	want := first.NumberRecords
	resp := first
	for page := 1; ; page++ {
		if page > 1 {
			var err error
			if resp, err = e.search(ctx, location, page); err != nil {
				return nil, 0, err
			}
			// A withdrawal lowers the total by a posting or two; a page stating none at all
			// after earlier ones stated some is a broken answer, and taking it as the bar
			// would pass whatever the walk had read so far.
			if resp.NumberRecords == 0 && want > 0 {
				return nil, 0, fmt.Errorf("location %q: page %d states no results after earlier pages stated %d", location, page, want)
			}
			want = min(want, resp.NumberRecords)
		}
		read += len(resp.Jvs)
		for _, v := range resp.Jvs {
			if !listed[v.ID] {
				listed[v.ID] = true
				out = append(out, v)
			}
		}
		if len(resp.Jvs) < euresPageSize || page*euresPageSize >= e.depthCap {
			break
		}
	}
	if read < want {
		return nil, 0, fmt.Errorf("location %q: read %d of the %d results the search states", location, read, want)
	}
	return out, read - len(out), nil
}

func (e eures) search(ctx context.Context, location string, page int) (euresSearchResponse, error) {
	var resp euresSearchResponse
	if err := e.http.PostJSON(ctx, euresSearchURL, euresSearchBody(location, page), &resp); err != nil {
		return resp, fmt.Errorf("search %q page %d: %w", location, page, err)
	}
	return resp, nil
}

// job is the identity every path shares — the shape a SeenRefresh carries.
func (v euresVacancy) job() Job {
	return Job{
		ExternalID: v.ID,
		URL:        euresPortalURL + v.ID + "?lang=en",
		Title:      strings.TrimSpace(v.Title),
		Company:    strings.TrimSpace(v.Employer.Name),
	}
}

func (e eures) toJob(ctx context.Context, board string, v euresVacancy) Job {
	j := v.job()
	j.Description = sanitizeHTML(v.Description)
	j.Location = euresCountryNames[strings.ToLower(board)]
	var detail euresDetailResponse
	if err := e.http.GetJSON(ctx, euresDetailURL+v.ID+"?requestLang=en", &detail); err == nil {
		profile := detail.JvProfiles[detail.PreferredLanguage]
		j.Location = profile.locationText(j.Location)
		j.ApplicationDeadline = euresTime(profile.LastApplicationDate)
	}
	j.PostedAt = euresTime(v.CreationDate)
	j.Countries = euresCountries(v.LocationMap)
	j.EmploymentType = euresEmploymentType(v.PositionOfferingCode, v.PositionScheduleCodes)
	j.IsTechHint = true
	return j
}

// locationText falls back to the board's country when the profile has no usable place.
func (p euresProfile) locationText(fallback string) string {
	if len(p.Locations) == 0 {
		return fallback
	}
	loc := p.Locations[0]
	countryName := euresCountryNames[strings.ToLower(loc.CountryCode)]
	if countryName == "" {
		countryName = fallback
	}
	place := loc.CityName
	if place == "" && len(loc.AddressLines) > 0 {
		place = loc.AddressLines[0]
	}
	return distinctJoin([]string{place, countryName}, ", ", func(s string) string { return s })
}

// euresCountries normalizes locationMap's country keys to freehire's canonical codes, sorted for
// a deterministic Job.Countries. "el" (EURES's code for Greece) is translated to the ISO "gr"
// NormalizeCountry expects.
func euresCountries(m map[string][]string) []string {
	var out []string
	for cc := range m {
		code := strings.ToLower(cc)
		if code == "el" {
			code = "gr"
		}
		if n := location.NormalizeCountry(code); n != "" {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// euresEmploymentType maps the search result's offering/schedule codes onto freehire's
// employment-type vocabulary where they map unambiguously, leaving it empty otherwise for the
// pipeline's own dictionaries to decide. The offering code wins when it names an unambiguous
// arrangement (internship/contract); otherwise the broader schedule code (full/part time) is
// used.
func euresEmploymentType(offering string, schedules []string) string {
	switch offering {
	case "internship":
		return "internship"
	case "contract", "temporary", "temporarytohire", "contracttohire":
		return "contract"
	}
	for _, s := range schedules {
		switch s {
		case "fulltime":
			return "full_time"
		case "parttime":
			return "part_time"
		}
	}
	return ""
}

// euresTime converts the API's Unix-millisecond dates. Zero/negative is treated as
// absent rather than the 1970 epoch.
func euresTime(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}
