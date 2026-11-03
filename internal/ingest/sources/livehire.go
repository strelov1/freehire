// Package sources: livehire adapts LiveHire-powered career sites (www.livehire.com/careers/<segment>),
// the talent platform behind employers such as Flybuys. The board is the tenant's segment — the
// path segment in the career site URL and the `segmentCode` every API call carries.
//
// The public JSON API lives under /careers-api on the same host and is reachable WITHOUT the
// Cloudflare challenge that fronts the HTML career pages: the job-search listing is a POST to
// /careers-api/talent-job-search ({"segmentCodes":[board], ...}) and each posting's detail is a
// GET to /careers-api/job/<board>/<urlCode>/<advertUrlCode>. Both verified live 2026-11-03
// (flybuys: 6 postings listed, detail returning the full HTML description). The anonymous token
// endpoints the SPA fetches before searching are not needed for these two calls.
//
// The listing carries structured facets (work type, flexible-work option, location, country,
// salary range text, posted/close dates) but no body; the description lives only on the detail
// call (descriptionHtml). LiveHire is therefore a HydratingSource, modelled on getro.go: the
// listing runs every board, detail hydrates only unseen postings, and seen ones yield a
// SeenRefresh carrying just identity.
package sources

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// livehireAPIBase is the base of LiveHire's public careers API on the same host that serves the
// branded career sites.
const livehireAPIBase = "https://www.livehire.com/careers-api"

const (
	// livehireSearchURL is the public job-search listing endpoint. It is POST-only; the request
	// body names the tenant by segmentCodes and pages by page/pageSize.
	livehireSearchURL = livehireAPIBase + "/talent-job-search"
	// livehireDetailURL is the per-posting detail: board, urlCode and advertUrlCode — the same
	// three path segments a posting's own URL carries
	// (/careers/<board>/job/<urlCode>/<advertUrlCode>/<seo-slug>).
	livehireDetailURL = livehireAPIBase + "/job/%s/%s/%s"
	// livehirePageSize is the listing page size requested per call.
	livehirePageSize = 100
	// livehireMaxPages bounds pagination so a server that keeps returning pages can never loop
	// the walk forever; the empty-page and hasMoreResults checks end it sooner.
	livehireMaxPages = 50
)

// livehireHTTP is the transport the adapter needs: the POST search listing and the GET detail.
type livehireHTTP interface {
	JSONPoster
	JSONGetter
}

// NewLiveHire builds the LiveHire adapter over the given HTTP client.
func NewLiveHire(c livehireHTTP) Source { return livehire{http: c} }

type livehire struct {
	http livehireHTTP
}

func (livehire) Provider() string { return "livehire" }

// livehireSearchRequest is the listing request body. The zero values the SPA itself sends are
// spelled out so the endpoint answers a tenant-scoped search rather than a platform-wide one
// (a body without segmentCodes answers a 500).
type livehireSearchRequest struct {
	Keywords            *string  `json:"keywords"`
	Categories          []string `json:"categories"`
	SegmentCodes        []string `json:"segmentCodes"`
	WorkTypes           []string `json:"workTypes"`
	FlexibleWorkOptions []string `json:"flexibleWorkOptions"`
	Page                int      `json:"page"`
	PageSize            int      `json:"pageSize"`
}

// livehireSearchResponse is one listing page. hasMoreResults and totalCount both end the walk:
// a page that returns fewer postings than its size is not itself proof of the end (the endpoint
// has been observed re-serving the full set regardless of the requested page), so the walk
// stops when hasMoreResults is false or nothing new was gathered.
type livehireSearchResponse struct {
	Jobs           []livehirePosting `json:"jobs"`
	TotalCount     int               `json:"totalCount"`
	HasMoreResults bool              `json:"hasMoreResults"`
}

// livehirePosting is one listing entry. urlCode and advertUrlCode address the detail call and
// the canonical posting URL; the rest are the structured facets the listing carries inline.
type livehirePosting struct {
	SegmentCode        string     `json:"segmentCode"`
	URLCode            string     `json:"urlCode"`
	AdvertURLCode      string     `json:"advertUrlCode"`
	SEOSlug            string     `json:"seoSlug"`
	RoleName           string     `json:"roleName"`
	Summary            string     `json:"summary"`
	WorkType           string     `json:"workType"`
	FlexibleWorkOption string     `json:"flexibleWorkOption"`
	PhysicalLocation   string     `json:"physicalLocation"`
	CountryISO         string     `json:"countryIso"`
	CountryName        string     `json:"countryName"`
	SalaryRange        string     `json:"salaryRange"`
	PostedFrom         *time.Time `json:"postedFrom"`
	ApplicationsClose  *time.Time `json:"applicationsCloseAt"`
	InternalOnly       bool       `json:"internalOnly"`
}

// livehireDetail is the slice of the detail response the adapter reads. The detail carries the
// full description both as text and as HTML; descriptionHtml is the canonical one and goes
// through the shared sanitizer. Salary fields mirror the listing's salaryRange string when the
// employer stated bounds.
type livehireDetail struct {
	RoleName            string     `json:"roleName"`
	DescriptionHTML     string     `json:"descriptionHtml"`
	PhysicalLocation    string     `json:"physicalLocation"`
	CountryISOCode      string     `json:"locationCountryIsoCode"`
	WorkType            string     `json:"workType"`
	FlexibleWorkOption  string     `json:"flexibleWorkOption"`
	SalaryType          string     `json:"salaryType"`
	MinSalary           *int       `json:"minSalary"`
	MaxSalary           *int       `json:"maxSalary"`
	SalaryRange         string     `json:"salaryRange"`
	CurrencyISOCode     string     `json:"currencyIsoCode"`
	ApplicationsClosed  bool       `json:"applicationsClosed"`
	ClosedAt            *time.Time `json:"closedAt"`
	Category            string     `json:"category"`
	ApplicationsCloseAt *time.Time `json:"applicationsCloseAt"`
	InternalOnly        bool       `json:"internalOnly"`
}

// Fetch is the list-only crawl (no description): kept as the fallback for callers that do not
// drive hydration. FetchNew is the hydrating path used by ingest.
func (s livehire) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it lists the board, then fetches a posting's detail (the
// description the listing omits) only for a posting the catalogue does not already have. A seen
// posting yields the list-only job as a liveness refresh; an unseen posting whose detail fails
// still ingests list-only, so one dead detail never drops a live posting.
func (s livehire) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p livehirePosting) (Job, bool) {
		base, ok := p.toJob()
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			// Already ingested: refresh liveness only, no detail request — a content-less
			// re-upsert would re-derive its facets from an empty description.
			base.SeenRefresh = true
			return base, true
		}
		d, ok := s.detail(ctx, e.Board, p)
		if !ok {
			log.Printf("livehire: detail %s failed; ingesting list-only", base.ExternalID)
			return base, true
		}
		hydrateLiveHire(&base, d)
		return base, true
	}), nil
}

// list pages the search endpoint until hasMoreResults is false. The endpoint re-serves the full
// set on later pages (verified live: page 2 at pageSize 2 returned all 6 postings again), so
// cross-page dedup by external id keeps the set honest and an unchanged set ends the walk.
func (s livehire) list(ctx context.Context, board string) ([]livehirePosting, error) {
	var all []livehirePosting
	seen := map[string]bool{}
	for page := 1; page <= livehireMaxPages; page++ {
		var resp livehireSearchResponse
		body := livehireSearchRequest{
			Keywords:            nil,
			Categories:          []string{},
			SegmentCodes:        []string{board},
			WorkTypes:           []string{},
			FlexibleWorkOptions: []string{},
			Page:                page,
			PageSize:            livehirePageSize,
		}
		if err := s.http.PostJSON(ctx, livehireSearchURL, body, &resp); err != nil {
			return nil, fmt.Errorf("livehire: search page %d: %w", page, err)
		}
		before := len(seen)
		for _, p := range resp.Jobs {
			id := p.externalID()
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			all = append(all, p)
		}
		if !resp.HasMoreResults || len(seen) == before {
			break
		}
	}
	return all, nil
}

// detail fetches one posting's full detail. ok=false means the detail could not be read — the
// caller falls back to the list-only job rather than dropping the posting.
func (s livehire) detail(ctx context.Context, board string, p livehirePosting) (livehireDetail, bool) {
	var d livehireDetail
	if p.URLCode == "" || p.AdvertURLCode == "" {
		return d, false
	}
	url := fmt.Sprintf(livehireDetailURL, board, p.URLCode, p.AdvertURLCode)
	if err := s.http.GetJSON(ctx, url, &d); err != nil {
		return livehireDetail{}, false
	}
	return d, true
}

// externalID is the posting's stable identity: segment + both URL codes, the same three parts
// the platform's own id string (id: "flybuys_EYFKM_LiveHirePublic") and canonical URL carry.
func (p livehirePosting) externalID() string {
	return p.SegmentCode + "_" + p.URLCode + "_" + p.AdvertURLCode
}

// toJob maps a listing posting to a Job (no description — detail hydration fills it). The
// structured facets LiveHire states — work type, flexible-work option, country, close date —
// are mapped into freehire's vocabularies; anything the platform does not state stays unset.
func (p livehirePosting) toJob() (Job, bool) {
	if p.URLCode == "" || p.AdvertURLCode == "" || p.SegmentCode == "" || p.RoleName == "" {
		return Job{}, false
	}
	if p.InternalOnly {
		return Job{}, false
	}
	slug := p.SEOSlug
	if slug == "" {
		slug = strings.ToLower(strings.Join(strings.Fields(strings.ToLower(p.RoleName)), "-"))
	}
	job := Job{
		ExternalID: p.externalID(),
		URL: fmt.Sprintf("https://www.livehire.com/careers/%s/job/%s/%s/%s",
			p.SegmentCode, p.URLCode, p.AdvertURLCode, slug),
		Title:     strings.TrimSpace(p.RoleName),
		Company:   strings.TrimSpace(p.SegmentCode),
		Location:  strings.TrimSpace(p.PhysicalLocation),
		WorkMode:  livehireWorkMode(p.FlexibleWorkOption),
		Countries: countryFromCode(p.CountryISO),
	}
	if p.PostedFrom != nil {
		t := p.PostedFrom.UTC()
		job.PostedAt = &t
	}
	if p.ApplicationsClose != nil {
		t := p.ApplicationsClose.UTC()
		job.ApplicationDeadline = &t
	}
	return job, true
}

// hydrateLiveHire folds the detail's content into the list-only job: the sanitized HTML
// description, the detail's own (fresher) structured fields, and the salary bounds the detail
// states. A posting whose applications are closed or that was closed outright is marked Removed.
func hydrateLiveHire(job *Job, d livehireDetail) {
	job.Description = sanitizeHTML(d.DescriptionHTML)
	if d.ApplicationsClosed || d.ClosedAt != nil {
		job.Removed = true
		return
	}
	if d.RoleName != "" {
		job.Title = strings.TrimSpace(d.RoleName)
	}
	if d.PhysicalLocation != "" {
		job.Location = strings.TrimSpace(d.PhysicalLocation)
	}
	if d.CountryISOCode != "" {
		job.Countries = countryFromCode(d.CountryISOCode)
	}
	if m := livehireWorkMode(d.FlexibleWorkOption); m != "" {
		job.WorkMode = m
	}
	if et := livehireEmploymentType(d.WorkType); et != "" {
		job.EmploymentType = et
	}
	if d.MinSalary != nil && d.MaxSalary != nil && *d.MinSalary > 0 && *d.MaxSalary >= *d.MinSalary {
		job.SalaryMin = d.MinSalary
		job.SalaryMax = d.MaxSalary
		job.SalaryCurrency = d.CurrencyISOCode
	}
	if d.ApplicationsCloseAt != nil && !d.ApplicationsCloseAt.IsZero() {
		t := d.ApplicationsCloseAt.UTC()
		job.ApplicationDeadline = &t
	}
}

// livehireWorkMode maps LiveHire's flexibleWorkOption ("Hybrid", "Remote", "Onsite"…) into
// freehire's work-mode vocabulary via the shared normalizer; unknown values map to "".
func livehireWorkMode(o string) string {
	return workplaceTypeMode(o)
}

// livehireEmploymentType maps LiveHire's workType enum onto vocab.EmploymentTypeValues. Only
// values the platform actually spells are mapped; anything else leaves the field unset for the
// pipeline's dictionaries rather than guessing.
func livehireEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "fulltimepermanent", "fulltimecontract", "fulltimecasual", "fulltime":
		return "full_time"
	case "parttimepermanent", "parttimecasual", "parttime":
		return "part_time"
	case "casual", "temporary", "fixedterm":
		return "contract"
	case "internship", "graduate":
		return "internship"
	default:
		return ""
	}
}

// livehireVocabOK is a compile-time guard that the employment-type mapping only emits values in
// the controlled vocabulary.
var _ = func() bool {
	for _, v := range []string{"full_time", "part_time", "contract", "internship"} {
		if !slices.Contains(vocab.EmploymentTypeValues, v) {
			panic("livehire: employment type outside vocabulary: " + v)
		}
	}
	return true
}()
