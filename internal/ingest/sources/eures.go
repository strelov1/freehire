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
// description, unlike arbeitsagentur. A per-posting detail fetch is still made, but ONLY to
// resolve a human-readable Location: the search result's own locationMap carries opaque NUTS
// region codes, while the detail response's structured place data carries a real city name.
// The IT scope (ICT professionals + ICT technicians) is fixed in code as an ESCO/ISCO occupation
// filter, never board-selectable, so every yielded Job carries IsTechHint.
type eures struct {
	http euresHTTP
}

// euresHTTP is the transport eures needs: a JSON POST for the paginated search, a JSON GET for
// the per-posting detail (location enrichment only).
type euresHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	euresSearchURL = "https://europa.eu/eures/api/jv-searchengine/public/jv-search/search"
	// euresDetailURL is the per-posting detail endpoint, keyed by the search result's own
	// (already unique) id. Fetched only for its structured location data — see the eures
	// doc comment.
	euresDetailURL = "https://europa.eu/eures/api/jv-searchengine/public/jv/id/"
	// euresPortalURL is the public EURES portal page for a posting, used as Job.URL. Not
	// documented by the (unofficial) API docs; confirmed live against the current portal.
	euresPortalURL = "https://europa.eu/eures/portal/jv-se/jv-details/"
	// euresPageSize is the API's own maximum resultsPerPage.
	euresPageSize = 50
	// euresMaxPages backstops pagination at the API's measured ~10,000-result depth cap
	// (confirmed live: page*resultsPerPage beyond it answers "Too many results were
	// requested" instead of a page of results). Combined with euresPublicationPeriod, a
	// crawl should exhaust its results well before this backstop; it exists so the adapter
	// never issues the failing request even if a board's volume grows unexpectedly.
	euresMaxPages = 10000 / euresPageSize
	// euresPublicationPeriod bounds every search to a fresh window, the same role
	// arbeitsagenturWithinDays plays there. LAST_THREE_DAYS was chosen because it keeps the
	// largest observed country/occupation-group combination (Germany + ICT professionals,
	// measured at ~6,100 records) comfortably under the depth cap, where LAST_WEEK
	// (~10,500) does not fit. Sorting MOST_RECENT means that if some board still exceeds
	// the cap, only the oldest tail of the window is missed for that run — it is picked up
	// on the next run, well inside the window, since ingest runs far more often than every
	// three days.
	euresPublicationPeriod = "LAST_THREE_DAYS"
	euresSortSearch        = "MOST_RECENT"
	euresRequestLanguage   = "en"
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
func NewEures(c euresHTTP) Source { return eures{http: c} }

func (eures) Provider() string { return "eures" }

// aggregator marks eures for cross-source dedup suppression — see the eures doc comment.
func (eures) aggregator() {}

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
	PublicationPeriod                   string   `json:"publicationPeriod"`
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

// euresSearchBody builds the request for one board's page.
func euresSearchBody(board string, page int) euresSearchRequest {
	return euresSearchRequest{
		ResultsPerPage:                      euresPageSize,
		Page:                                page,
		SortSearch:                          euresSortSearch,
		Keywords:                            []any{},
		PublicationPeriod:                   euresPublicationPeriod,
		OccupationUris:                      euresOccupationURIs,
		SkillUris:                           []string{},
		RequiredExperienceCodes:             []string{},
		PositionScheduleCodes:               []string{},
		SectorCodes:                         []string{},
		EducationAndQualificationLevelCodes: []string{},
		PositionOfferingCodes:               []string{},
		LocationCodes:                       []string{strings.ToLower(strings.TrimSpace(board))},
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

type euresEmployer struct {
	Name string `json:"name"`
}

// euresDetailResponse is the per-posting detail payload, read only for its structured location
// data. requestLang does NOT select a translated profile the way the search endpoint's
// requestLanguage does (confirmed live: jvProfiles only ever carried preferredLanguage's own
// key), so the profile to read is always the one keyed by PreferredLanguage.
type euresDetailResponse struct {
	PreferredLanguage string                  `json:"preferredLanguage"`
	JvProfiles        map[string]euresProfile `json:"jvProfiles"`
}

type euresProfile struct {
	Locations []euresLocation `json:"locations"`
}

// euresLocation is one place entry. CityName and AddressLines[0] are both nullable/absent on
// live data (confirmed for France and Poland respectively) — see euresLocationText.
type euresLocation struct {
	CountryCode  string   `json:"countryCode"`
	CityName     string   `json:"cityName"`
	AddressLines []string `json:"addressLines"`
}

func (e eures) Fetch(ctx context.Context, entry CompanyEntry) ([]Job, error) {
	var vacancies []euresVacancy
	for page := 1; page <= euresMaxPages; page++ {
		var resp euresSearchResponse
		if err := e.http.PostJSON(ctx, euresSearchURL, euresSearchBody(entry.Board, page), &resp); err != nil {
			return nil, fmt.Errorf("eures: search board %q page %d: %w", entry.Board, page, err)
		}
		vacancies = append(vacancies, resp.Jvs...)
		if len(resp.Jvs) < euresPageSize || len(vacancies) >= resp.NumberRecords {
			break
		}
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v euresVacancy) (Job, bool) {
		return e.toJob(ctx, entry.Board, v), true
	}), nil
}

func (e eures) toJob(ctx context.Context, board string, v euresVacancy) Job {
	return Job{
		ExternalID:     v.ID,
		URL:            euresPortalURL + v.ID + "?lang=en",
		Title:          strings.TrimSpace(v.Title),
		Company:        strings.TrimSpace(v.Employer.Name),
		Description:    sanitizeHTML(v.Description),
		Location:       e.resolveLocation(ctx, v.ID, board),
		PostedAt:       euresPostedAt(v.CreationDate),
		Countries:      euresCountries(v.LocationMap),
		EmploymentType: euresEmploymentType(v.PositionOfferingCode, v.PositionScheduleCodes),
		IsTechHint:     true,
	}
}

// resolveLocation resolves a human-readable Location by fetching the posting's detail. A failed
// fetch or a profile with no usable place data falls back to the board's own country name rather
// than dropping the posting.
func (e eures) resolveLocation(ctx context.Context, id, board string) string {
	fallback := euresCountryNames[strings.ToLower(board)]
	var d euresDetailResponse
	if err := e.http.GetJSON(ctx, euresDetailURL+id+"?requestLang=en", &d); err != nil {
		return fallback
	}
	profile, ok := d.JvProfiles[d.PreferredLanguage]
	if !ok || len(profile.Locations) == 0 {
		return fallback
	}
	loc := profile.Locations[0]
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

// euresPostedAt converts the API's unix-millisecond creationDate. Zero/negative is treated as
// absent rather than the 1970 epoch.
func euresPostedAt(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}
