package sources

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// emagine adapts portal.emagine.org, the consultant portal of emagine Consulting A/S — a
// European freelance-contract marketplace (~1k live assignments across DE/PL/FR/PT/SE/UK and
// India). Boardless: one public API, no per-tenant board. It is NOT an aggregator: the portal
// never names the end client, so every posting is contracted through emagine itself and the
// source facet would only duplicate the company filter.
//
// The listing is POST-only and omits the description, which lives behind a per-posting detail
// call, so the adapter hydrates (HydratingSource) — a detail request is spent only on a posting
// the catalogue does not already hold.
type emagine struct {
	list   JSONPoster // the search listing (three requests per crawl)
	detail JSONGetter // per-posting detail, bounded in-flight (see limitedEmagineGetter)
}

const (
	emagineSearchURL = "https://portal-api.emagine.org/api/JobAds/Search"
	// emagineDetailURL carries the description the listing omits. The trailing language
	// segment selects the UI language only — an ad is served in the language it was written
	// in (a German posting stays German under /EN), so it is fixed rather than per-posting.
	emagineDetailURL = "https://portal-api.emagine.org/api/JobAds/details/%d/EN"
	// emagineJobURL is the public posting page. The slug segment is decorative — the portal
	// routes on the id alone — but the canonical link carries it.
	emagineJobURL = "https://portal.emagine.org/jobs/%d/%s"
	// emagineCompany is the employer for every posting: emagine contracts the consultant and
	// keeps the end client anonymous.
	emagineCompany = "emagine"
	// emaginePageSize is the listing page size. The API honours it verbatim up to at least
	// 1000; 500 keeps a page's response comfortably inside the client's body cap.
	emaginePageSize = 500
	// emagineMaxPages bounds pagination as a backstop (totalCount is the primary stop).
	emagineMaxPages = 40
)

// NewEmagine builds the emagine adapter: list pages the search endpoint, detail hydrates each
// unseen posting's description under a bounded number of in-flight requests.
func NewEmagine(list JSONPoster, detail JSONGetter) Source {
	return emagine{list: list, detail: detail}
}

func (emagine) Provider() string { return "emagine" }

func (emagine) boardless() {}

// emagineSearchRequest is the listing request. Sorting is by ascending id: a stable order under
// concurrent publishing, unlike the portal's own recency sort, where an ad created mid-crawl
// shifts every later page down by one and pushes a posting across the page boundary unseen.
type emagineSearchRequest struct {
	SkipCount           int           `json:"skipCount"`
	MaxResultCount      int           `json:"maxResultCount"`
	Sorting             string        `json:"sorting"`
	Filter              emagineFilter `json:"filter"`
	SupportedLanguageID string        `json:"supportedLanguageId"`
}

// emagineFilter is the unfiltered filter block. Every collection on it is [Required] server-side,
// so each must serialize as [] — a nil slice marshals to null and the request 400s. The whole
// block and supportedLanguageId are mandatory too: omitting the language yields a 500.
type emagineFilter struct {
	TextFilters           []string `json:"textFilters"`
	WorkLocationTypes     []string `json:"workLocationTypes"`
	WorkLocations         []string `json:"workLocations"`
	ProfessionalRolesIDs  []int    `json:"professionalRolesIds"`
	ConsultantSeniorities []string `json:"consultantSeniorities"`
	LanguageProficiencies []string `json:"languageProficiencies"`
	IndustriesIDs         []int    `json:"industriesIds"`
	RecordIDsToExclude    []int    `json:"recordIdsToExclude"`
}

// emagineSearchBody builds the request for one page.
func emagineSearchBody(skip int) emagineSearchRequest {
	return emagineSearchRequest{
		SkipCount:      skip,
		MaxResultCount: emaginePageSize,
		Sorting:        "Id asc",
		Filter: emagineFilter{
			TextFilters:           []string{},
			WorkLocationTypes:     []string{},
			WorkLocations:         []string{},
			ProfessionalRolesIDs:  []int{},
			ConsultantSeniorities: []string{},
			LanguageProficiencies: []string{},
			IndustriesIDs:         []int{},
			RecordIDsToExclude:    []int{},
		},
		SupportedLanguageID: "EN",
	}
}

// emagineSearchResponse is one listing page.
type emagineSearchResponse struct {
	Items      []emagineAd `json:"items"`
	TotalCount int         `json:"totalCount"`
}

// emagineAd is one listed assignment. The listing carries no description and no publication
// date; startDate is when the consultant is expected on the project, not when the ad went up.
type emagineAd struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	IsPartTime bool   `json:"isPartTime"`
	Area       struct {
		Name string `json:"name"`
	} `json:"area"`
	WorkLocation *emagineWorkLocation `json:"jobAdWorkLocation"`
}

// emagineWorkLocation is the assignment's place of work; city and region are often null.
type emagineWorkLocation struct {
	WorkLocationType string `json:"workLocationType"`
	City             string `json:"city"`
	Region           string `json:"region"`
	Country          string `json:"country"`
}

// emagineDetail is the per-posting payload: the description, the grade, and the live status.
type emagineDetail struct {
	Status      string `json:"status"`
	Description string `json:"description"`
	Seniority   string `json:"seniority"`
}

// Fetch is the list-only crawl (no description) — the fallback for callers that do not drive
// hydration. FetchNew is the hydrating path used by ingest.
func (s emagine) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	ads, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, a := range ads {
		if job, ok := a.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it pages the same listing but fetches a posting's detail only
// when the catalogue does not already hold it. A seen posting yields a liveness-only refresh (no
// detail request, content preserved); an unseen one is hydrated; a failed detail is isolated
// (logged, ingested list-only) so a posting is never lost to it.
func (s emagine) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	ads, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(ads, defaultDetailWorkers, func(a emagineAd) (Job, bool) {
		base, ok := a.toJob()
		if !ok {
			return Job{}, false // unusable ad — dropped, as in Fetch
		}
		if seen(base.ExternalID) {
			// Already ingested: refresh liveness only. Re-upserting content-less would
			// wipe the description and the facets derived from it when this ad was new.
			base.SeenRefresh = true
			return base, true
		}
		d, err := s.fetchDetail(ctx, a.ID)
		if err != nil {
			log.Printf("emagine: detail %d failed; ingesting list-only: %v", a.ID, err)
			return base, true
		}
		return d.apply(base)
	}), nil
}

// crawl pages the listing by skipCount until totalCount is covered — the shared list walk behind
// Fetch and FetchNew. An empty page ends the crawl regardless of the reported total, so a
// shrinking or overstated count can never spin the pager.
func (s emagine) crawl(ctx context.Context) ([]emagineAd, error) {
	var ads []emagineAd
	for page := 0; page < emagineMaxPages; page++ {
		var resp emagineSearchResponse
		if err := s.list.PostJSON(ctx, emagineSearchURL, emagineSearchBody(len(ads)), &resp); err != nil {
			return nil, fmt.Errorf("emagine: search at %d: %w", len(ads), err)
		}
		if len(resp.Items) == 0 {
			break
		}
		ads = append(ads, resp.Items...)
		if len(ads) >= resp.TotalCount {
			break
		}
	}
	return ads, nil
}

// fetchDetail fetches a posting's detail. It returns the error rather than a bare miss so the
// caller can name the cause when it falls back to the list-only job — a silently dropped
// description is permanent, because the next crawl sees the posting as already ingested and never
// re-fetches it.
func (s emagine) fetchDetail(ctx context.Context, id int) (emagineDetail, error) {
	var d emagineDetail
	if err := s.detail.GetJSON(ctx, fmt.Sprintf(emagineDetailURL, id), &d); err != nil {
		return emagineDetail{}, err
	}
	return d, nil
}

// apply enriches a list-derived job with its detail. It returns ok=false for an ad the detail
// reports closed: the listing serves open ads only, but one can be taken down between the two
// calls, and ingesting it would create a job that is already dead.
func (d emagineDetail) apply(base Job) (Job, bool) {
	if strings.EqualFold(strings.TrimSpace(d.Status), "closed") {
		return Job{}, false
	}
	base.Description = sanitizeHTML(d.Description)
	base.Seniority = emagineSeniority(d.Seniority)
	return base, true
}

// toJob maps an ad to a Job, returning ok=false for an unusable one (no id to key on, or no
// title to name it).
func (a emagineAd) toJob() (Job, bool) {
	if a.ID == 0 || strings.TrimSpace(a.Title) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: strconv.Itoa(a.ID),
		URL:        fmt.Sprintf(emagineJobURL, a.ID, normalize.Slug(a.Title)),
		Title:      a.Title,
		Company:    emagineCompany,
		Category:   emagineCategory(a.Area.Name),
		// The portal brokers freelance assignments, so a contract is what the source
		// states rather than something inferred from the ad's wording.
		EmploymentType: emagineEmploymentType(a.IsPartTime),
	}
	if l := a.WorkLocation; l != nil {
		job.Location = distinctJoin([]string{l.City, l.Region, l.Country}, ", ", func(s string) string { return s })
		job.WorkMode = workplaceTypeMode(l.WorkLocationType)
	}
	return job, true
}

// emagineEmploymentType maps the one employment fact the listing states. Part-time wins when set:
// it is the narrower statement about the engagement, and every assignment here is a contract.
func emagineEmploymentType(partTime bool) string {
	if partTime {
		return "part_time"
	}
	return "contract"
}

// emagineSeniority maps emagine's three grades onto freehire's vocabulary. The portal offers
// nothing below entry and nothing above senior, so the tails (intern, lead and up) never appear,
// and the grade is optional — most ads state none, which maps to "" and leaves it to the title
// dictionary. Both spellings of a grade are accepted: the detail payload states the DISPLAY name
// ("Mid level"), while the portal's own filter vocabulary keys the same grade by id ("Medium").
func emagineSeniority(grade string) string {
	switch strings.ToLower(strings.TrimSpace(grade)) {
	case "entry", "entry level":
		return "junior"
	case "medium", "mid level":
		return "middle"
	case "senior":
		return "senior"
	default:
		return ""
	}
}

// emagineCategories maps the practice areas that pin exactly ONE freehire category. The rest are
// deliberately absent: "Software Development" spans backend, frontend, mobile and fullstack, and
// "Data & Analytics" spans data engineering, data science and analytics — mapping either would
// state a category the source never claimed. An unmapped area leaves Category empty so the title
// dictionary decides.
var emagineCategories = map[string]string{
	"test & qa":              "qa",
	"ux, ui & visual design": "design",
	"project delivery":       "project_management",
}

// emagineCategory resolves a practice area to a freehire category, or "" when it is broader than
// one category.
func emagineCategory(area string) string {
	return emagineCategories[strings.ToLower(strings.TrimSpace(area))]
}
