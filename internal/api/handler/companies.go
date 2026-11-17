package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/singleflight"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/search"
)

// companiesHandlers serves the public company catalogue reads: the ranked,
// filterable list, the subindustry vocabulary, and the single-company detail.
type companiesHandlers struct {
	queries *db.Queries
	// companySearch backs GET /api/v1/companies, separate from the jobs search. Nil when Meilisearch is unconfigured;
	// on nil or any query error the list falls back to the Postgres substring path.
	companySearch companySearcher
	// subindustries holds the vocabulary CompanySubindustries serves.
	subindustries subindustryCache
}

func newCompaniesHandlers(queries *db.Queries, companySearch companySearcher) *companiesHandlers {
	return &companiesHandlers{
		queries:       queries,
		companySearch: companySearch,
		subindustries: subindustryCache{load: queries.CompanySubindustries, now: time.Now},
	}
}

// The detail, hiring and closed-jobs reads stay Fiber routes: a slug a merge retired answers with a 301 the contract does not model.
func (h *companiesHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/companies", contract)
	api.Get("/companies/subindustries", contract)
	api.Get("/companies/by-domain", contract)
	api.Get("/companies/:slug", mw.optional, h.GetCompany)
	api.Get("/companies/:slug/hiring", h.CompanyHiring)
	api.Get("/companies/:slug/closed-jobs", h.CompanyClosedJobs)
}

// companySearcher is the company-search backend the list handler depends on (*search.Client; tests inject a fake).
// Nil, or any search error, routes to the Postgres path (see ListCompanies).
type companySearcher interface {
	SearchCompanies(ctx context.Context, p search.CompanySearchParams) (search.CompanyResult, error)
}

// companyDetailResponse is a company with a page of its jobs. Jobs is []jobview.Job and Company a companyView, so the
// internal job id and bookkeeping columns cannot leak.
type companyDetailResponse struct {
	Company companyView   `json:"company"`
	Jobs    []jobview.Job `json:"jobs"`
	// ReferralAvailable is true when the company has an approved employee referrer.
	ReferralAvailable bool `json:"referral_available"`
	// Response is how many of the applications whose outcome we can OBSERVE were answered. Absent below the sample gate.
	Response *companyResponse `json:"response,omitempty"`
	// Parent owns this company and Subsidiaries are the companies it owns (company_parents). They stay separate
	// companies with their own jobs. Both are omitted when there are none.
	Parent       *companyRef  `json:"parent,omitempty"`
	Subsidiaries []companyRef `json:"subsidiaries,omitempty"`
}

// companyRef names a related company for a link: enough to render it, nothing more.
type companyRef struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	JobCount int32  `json:"job_count"`
}

// companyRelatives reads a company's parent and subsidiaries in one query. Best-effort like the referral flag:
// a failed read leaves the links out and never fails the company page.
func companyRelatives(ctx context.Context, q *db.Queries, slug string) (parent *companyRef, subs []companyRef) {
	rows, err := q.ListCompanyRelatives(ctx, slug)
	if err != nil {
		log.Printf("companies: relatives of %q: %v", slug, err)
		return nil, nil
	}
	for _, r := range rows {
		ref := companyRef{Slug: r.Slug, Name: r.Name, JobCount: r.JobCount}
		if r.Role == "parent" {
			parent = &ref
			continue
		}
		subs = append(subs, ref)
	}
	return parent, subs
}

// companyView is the public projection of a company for the detail endpoint: db.Company minus the internal
// bookkeeping columns (created_at, updated_at, is_reference, company_info_at).
type companyView struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Collections  []string `json:"collections"`
	JobCount     int32    `json:"job_count"`
	Regions      []string `json:"regions"`
	Countries    []string `json:"countries"`
	Domains      []string `json:"domains"`
	CompanyTypes []string `json:"company_types"`
	CompanySizes []string `json:"company_sizes"`
	Industries   []string `json:"industries"`
	// Nullable columns are pointers, not pgtype, so the wire shape is decided here rather than by a pgx version
	// (companyListItem explains why). The wire output is the same as pgtype's.
	YearFounded      *int            `json:"year_founded"`
	EmployeeCount    *int            `json:"employee_count"`
	HqCountry        *string         `json:"hq_country"`
	OrganizationType *string         `json:"organization_type"`
	Tagline          *string         `json:"tagline"`
	CompanyInfo      json.RawMessage `json:"company_info"`
	RemoteRegions    []string        `json:"remote_regions"`
	YcBatch          []string        `json:"yc_batch"`
	YcStatus         []string        `json:"yc_status"`
	YcStage          []string        `json:"yc_stage"`
	YcFlags          []string        `json:"yc_flags"`
	Maturity         *string         `json:"maturity"`
	// UpvoteCount/DownvoteCount are the materialized thumbs counters. MyVote is the caller's own vote (-1, 0, 1),
	// set only on the auth-aware detail read, 0 otherwise.
	UpvoteCount   int32 `json:"upvote_count"`
	DownvoteCount int32 `json:"downvote_count"`
	MyVote        int32 `json:"my_vote"`
	// FeedbackCount/FeedbackRatingAvg are the materialized feedback counters (internal/engage/companyfeedback).
	FeedbackCount     int32    `json:"feedback_count"`
	FeedbackRatingAvg *float32 `json:"feedback_rating_avg"`
	// AIInterviewReports is how many people reported an AI interviewer (internal/engage/processreport). Omitted at zero:
	// the count must always be shown with the label, so no reports is an absent field, not a zero.
	AIInterviewReports int32 `json:"ai_interview_reports,omitempty"`
}

// companyViewFrom projects a stored company onto its public view, dropping only the
// internal bookkeeping columns.
func companyViewFrom(c db.Company) companyView {
	return companyView{
		Slug:               c.Slug,
		Name:               c.Name,
		Collections:        c.Collections,
		JobCount:           c.JobCount,
		Regions:            c.Regions,
		Countries:          c.Countries,
		Domains:            c.Domains,
		CompanyTypes:       c.CompanyTypes,
		CompanySizes:       c.CompanySizes,
		Industries:         c.Industries,
		YearFounded:        pgconv.IntPtr(c.YearFounded),
		EmployeeCount:      pgconv.IntPtr(c.EmployeeCount),
		HqCountry:          pgconv.TextPtr(c.HqCountry),
		OrganizationType:   pgconv.TextPtr(c.OrganizationType),
		Tagline:            pgconv.TextPtr(c.Tagline),
		CompanyInfo:        c.CompanyInfo,
		RemoteRegions:      c.RemoteRegions,
		YcBatch:            c.YcBatch,
		YcStatus:           c.YcStatus,
		YcStage:            c.YcStage,
		YcFlags:            c.YcFlags,
		Maturity:           pgconv.TextPtr(c.Maturity),
		UpvoteCount:        c.UpvoteCount,
		DownvoteCount:      c.DownvoteCount,
		FeedbackCount:      c.FeedbackCount,
		FeedbackRatingAvg:  pgconv.Float4Ptr(c.FeedbackRatingAvg),
		AIInterviewReports: c.AiInterviewReports,
	}
}

// ListCompanies returns a page of companies with denormalized job counts, most active first. `q` filters by a
// case-insensitive name substring; the repeatable facet params (collections, regions, countries, domains, company_type,
// company_size, remote_regions, yc_batch, yc_status) overlap the denormalized facet arrays (OR within a facet, AND across,
// composable with `q`). meta.total counts the full filter.
func (h *companiesHandlers) ListCompanies(ctx context.Context, params apigen.ListCompaniesParams) (*apigen.ListCompaniesOK, error) {
	limit, offset, err := contractPageWindowed(ctx, defaultLimit, maxLimit)
	if err != nil {
		return nil, err
	}
	search := params.Q.Or("")
	// sort=rating orders by feedback_rating_avg; any other value is job_count DESC, name. Forces the Postgres path
	// because rating isn't a Meili-sortable attribute yet (see companySettings).
	sort := params.Sort.Or("")
	vals := contractQuery(ctx)

	// Parse each facet once and feed both queries so their WHERE clauses can't drift. Every facet is a non-nil slice so pgx
	// sends '{}', not NULL, which would defeat the cardinality() short-circuit.
	collections := facetValues(vals, "collections")
	regions := facetValues(vals, "regions")
	countries := facetValues(vals, "countries")
	domains := facetValues(vals, "domains")
	industries := facetValues(vals, "industries")
	companyTypes := facetValues(vals, "company_type")
	companySizes := facetValues(vals, "company_size")
	remoteRegions := facetValues(vals, "remote_regions")
	ycBatch := facetValues(vals, "yc_batch")
	ycStatus := facetValues(vals, "yc_status")
	ycStage := facetValues(vals, "yc_stage")
	ycFlags := facetValues(vals, "yc_flags")
	maturity := facetValues(vals, "maturity")
	subindustries := facetValues(vals, "subindustries")

	// One list feeds both isCompanyFilter calls: a second hand-written list once forgot `industries` and miscounted the
	// request as unfiltered, silently since the argument is variadic.
	facets := [][]string{collections, regions, countries, domains, industries,
		companyTypes, companySizes, remoteRegions, ycBatch, ycStatus, ycStage,
		ycFlags, maturity, subindustries}

	// Meilisearch serves the list when configured and a filter (q or any facet) is present: relevance-first, typo-tolerant,
	// job_count as tiebreaker. The unfiltered catalogue stays on Postgres (job_count DESC, name; O(1) total estimate). Any Meili
	// error falls through to Postgres. sort=rating also stays off Meili, which would silently ignore that order.
	if h.companySearch != nil && sort != "rating" && isCompanyFilter(search, facets...) {
		items, total, err := h.companyHitsViaMeili(ctx, search, vals, limit, offset)
		if err == nil {
			return companyListPage(items, total, limit, offset, vals)
		}
		log.Printf("companies: meili search fell back to postgres (q=%q): %v", search, err)
	}

	companies, err := h.queries.ListCompanies(ctx, db.ListCompaniesParams{
		Search:        search,
		Collections:   collections,
		Regions:       regions,
		Countries:     countries,
		Domains:       domains,
		Industries:    industries,
		CompanyTypes:  companyTypes,
		CompanySizes:  companySizes,
		RemoteRegions: remoteRegions,
		YcBatch:       ycBatch,
		YcStatus:      ycStatus,
		YcStage:       ycStage,
		YcFlags:       ycFlags,
		Maturity:      maturity,
		Subindustries: subindustries,
		Sort:          sort,
		Limit:         int32(limit),
		Offset:        int32(offset),
	})
	if err != nil {
		return nil, err
	}

	// The unfiltered count(*) is a cold-cache heap scan (tens of seconds on prod, see EstimateHiringCompanies); filters narrow
	// to an index. So a filtered request gets the exact count and the unfiltered case the O(1) planner estimate, as /jobs does.
	var total int64
	if isCompanyFilter(search, facets...) {
		total, err = h.queries.CountCompanies(ctx, db.CountCompaniesParams{
			Search:        search,
			Collections:   collections,
			Regions:       regions,
			Countries:     countries,
			Domains:       domains,
			Industries:    industries,
			CompanyTypes:  companyTypes,
			CompanySizes:  companySizes,
			RemoteRegions: remoteRegions,
			YcBatch:       ycBatch,
			YcStatus:      ycStatus,
			YcStage:       ycStage,
			YcFlags:       ycFlags,
			Maturity:      maturity,
			Subindustries: subindustries,
		})
	} else {
		total, err = h.queries.EstimateHiringCompanies(ctx)
	}
	if err != nil {
		return nil, err
	}

	items := make([]apigen.CompanyListItem, len(companies))
	for i, row := range companies {
		items[i] = companyListItemFromRow(row)
	}
	return companyListPage(items, total, limit, offset, vals)
}

// isCompanyFilter reports whether a /companies request carries any name search or facet constraint: the exact-vs-estimate
// meta.total gate in ListCompanies. Every facet is a non-nil slice, so len == 0 means unset.
func isCompanyFilter(search string, facets ...[]string) bool {
	if search != "" {
		return true
	}
	for _, f := range facets {
		if len(f) > 0 {
			return true
		}
	}
	return false
}

// subindustryFacet is one option in the company subindustry vocabulary: a clean YC
// subindustry leaf and the number of companies carrying it.
type subindustryFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// CompanySubindustries serves the distinct subindustry vocabulary with company counts, most common first, for the
// "Industry" facet's options. Counts ignore other active list filters.
func (h *companiesHandlers) CompanySubindustries(ctx context.Context) (*apigen.CompanySubindustriesOK, error) {
	facets, err := h.subindustries.get(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CompanySubindustry, len(facets))
	for i, f := range facets {
		out[i] = apigen.CompanySubindustry{Value: f.Value, Count: f.Count}
	}
	return &apigen.CompanySubindustriesOK{Data: out}, nil
}

// subindustryCacheTTL is how long one count of the vocabulary is served. The query is a full scan of companies, and uncached
// calls at a few req/s exhausted the connection pool. The vocabulary moves with ingest, so ten-minute-old counts are fine.
const subindustryCacheTTL = 10 * time.Minute

// subindustryCache keeps the vocabulary in process for subindustryCacheTTL. Concurrent misses share one query through
// flight. A failed query is not stored.
type subindustryCache struct {
	load func(context.Context) ([]db.CompanySubindustriesRow, error)
	// now is injectable so expiry is testable without sleeping.
	now func() time.Time

	flight  singleflight.Group
	mu      sync.Mutex
	facets  []subindustryFacet
	expires time.Time
}

// get returns the cached vocabulary, loading it when absent or expired. The slice is shared and must not be modified.
func (s *subindustryCache) get(ctx context.Context) ([]subindustryFacet, error) {
	if facets, ok := s.fresh(); ok {
		return facets, nil
	}
	v, err, _ := s.flight.Do("", func() (any, error) {
		// A flight that finished between our miss and this call has already stored it.
		if facets, ok := s.fresh(); ok {
			return facets, nil
		}
		rows, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		facets := make([]subindustryFacet, 0, len(rows))
		for _, r := range rows {
			facets = append(facets, subindustryFacet{Value: r.Value.String, Count: r.Count})
		}
		s.mu.Lock()
		s.facets, s.expires = facets, s.now().Add(subindustryCacheTTL)
		s.mu.Unlock()
		return facets, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subindustryFacet), nil
}

func (s *subindustryCache) fresh() ([]subindustryFacet, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.facets, s.facets != nil && s.now().Before(s.expires)
}

// facetValues reads the repeatable values of one facet query param, dropping empty entries and always returning a
// non-nil slice (pgx encodes '{}', keeping the SQL cardinality() short-circuit true).
func facetValues(vals url.Values, key string) []string {
	out := make([]string, 0, len(vals[key]))
	for _, v := range vals[key] {
		// Comma-joined and repeated must mean the same thing, as in the Meilisearch path (search.CompanyFilterFromValues)
		// and the job search. Treating `countries=DE,BR` as ONE literal value answered zero companies instead of an error.
		for _, part := range strings.Split(v, ",") {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// lookupCompany reads the company a :slug route names. A slug a merge retired keeps working: the 301 to the canonical
// slug is already written and redirected is true. The alias lookup runs only AFTER the miss, so a live company
// always wins over a stale alias. An unknown slug returns pgx.ErrNoRows, which RenderError maps to 404.
func (h *companiesHandlers) lookupCompany(c *fiber.Ctx, slug string) (company db.Company, redirected bool, err error) {
	company, canonical, err := companyBySlugOrAlias(c.Context(), h.queries, slug)
	if canonical != "" {
		return company, true, c.Redirect(companyPath(c, canonical), fiber.StatusMovedPermanently)
	}
	return company, false, err
}

// companySlugReader is what resolving a company slug reads. *db.Queries satisfies it.
type companySlugReader interface {
	GetCompany(ctx context.Context, slug string) (db.Company, error)
	GetCompanySlugAlias(ctx context.Context, aliasSlug string) (string, error)
}

// companyBySlugOrAlias reads a company by slug, falling back to the alias a merge left behind (then canonical is the
// live slug and the company is zero). The alias lookup runs only after the miss. An unknown slug returns pgx.ErrNoRows.
func companyBySlugOrAlias(ctx context.Context, q companySlugReader, slug string) (company db.Company, canonical string, err error) {
	company, err = q.GetCompany(ctx, slug)
	if !errors.Is(err, pgx.ErrNoRows) {
		return company, "", err
	}
	canonical, aliasErr := q.GetCompanySlugAlias(ctx, slug)
	switch {
	case aliasErr == nil:
		return company, canonical, err
	case !errors.Is(aliasErr, pgx.ErrNoRows):
		// A database failure is not "no such company": a 404 would tell a crawler to drop a live page and hide the outage.
		return company, "", aliasErr
	}
	return company, "", err
}

// GetCompany returns a single company with a page of its jobs: companies, then a single-table filter on company_slug, no join.
func (h *companiesHandlers) GetCompany(c *fiber.Ctx) error {
	slug := c.Params("slug")

	// The pagination window is checked before the company lookup so a refused request costs no database work and the
	// refusal stays uniform across the list endpoints (their shared guard asserts it).
	limit, offset, err := pageParams(c)
	if err != nil {
		return err
	}

	company, redirected, err := h.lookupCompany(c, slug)
	if err != nil || redirected {
		return err
	}

	jobs, err := h.queries.ListJobsByCompany(c.Context(), db.ListJobsByCompanyParams{
		CompanySlug: slug,
		Limit:       int32(limit),
		Offset:      int32(offset),
	})
	if err != nil {
		return err
	}

	views, err := jobview.FromRows(jobs)
	if err != nil {
		return err
	}

	// Referral availability is best-effort: a lookup error degrades to false (block hidden).
	referralAvailable, _ := h.queries.CompanyHasApprovedReferrer(c.Context(), slug)

	view := companyViewFrom(company)
	// The caller's own thumbs vote, overlaid only when signed in (OptionalAuth). Best-effort: an error leaves my_vote 0.
	if userID, ok := auth.UserID(c); ok {
		if mv, err := h.queries.GetCompanyVote(c.Context(), db.GetCompanyVoteParams{UserID: userID, CompanySlug: slug}); err == nil {
			view.MyVote = int32(mv)
		}
	}

	parent, subsidiaries := companyRelatives(c.Context(), h.queries, slug)

	return c.JSON(fiber.Map{"data": companyDetailResponse{
		Company:           view,
		Jobs:              views,
		ReferralAvailable: referralAvailable,
		Response:          companyResponseRate(c.Context(), h.queries, slug),
		Parent:            parent,
		Subsidiaries:      subsidiaries,
	}})
}

// closedJobsWindow bounds how deep the Closed tab may page (offset+limit), far tighter than maxPageWindow: each skipped
// offset is a random heap fetch of a closed row. 2000 is the site's FEED_WINDOW (web/src/lib/pagination.ts), so the pager never asks for more.
const closedJobsWindow = 2000

// CompanyClosedJobs serves the Closed tab: a page of the postings this company has closed, most recent first, from Postgres
// (closed rows are never in the search index). It answers has_more instead of a total: an exact count over a large
// employer's closed rows is the cost to avoid, and one extra fetched row says the same.
func (h *companiesHandlers) CompanyClosedJobs(c *fiber.Ctx) error {
	slug := c.Params("slug")

	// Before the lookup, so a refused page costs no database work (see GetCompany).
	limit, offset, err := pageParamsWithin(c, defaultLimit, maxLimit, closedJobsWindow)
	if err != nil {
		return err
	}

	if _, redirected, err := h.lookupCompany(c, slug); err != nil || redirected {
		return err
	}

	rows, err := h.queries.ListClosedJobsByCompany(c.Context(), db.ListClosedJobsByCompanyParams{
		CompanySlug: slug,
		Limit:       int32(limit + 1),
		Offset:      int32(offset),
	})
	if err != nil {
		return err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	views, err := jobview.FromRows(rows)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"data": views,
		"meta": fiber.Map{"limit": limit, "offset": offset, "has_more": hasMore},
	})
}

// companyHitsViaMeili runs the ranked company search and projects each hit onto the list wire shape, the same type the
// Postgres path serves. meta.total is Meilisearch's estimated filtered total.
func (h *companiesHandlers) companyHitsViaMeili(ctx context.Context, query string, vals url.Values, limit, offset int) ([]apigen.CompanyListItem, int64, error) {
	res, err := h.companySearch.SearchCompanies(ctx, search.CompanySearchParams{
		Query:  query,
		Filter: search.CompanyFilterFromValues(vals),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]apigen.CompanyListItem, len(res.Hits))
	for i, h := range res.Hits {
		items[i] = companyListItemFromDoc(h)
	}
	return items, res.Total, nil
}

// companyListItemFromRow projects the Postgres read onto the wire shape. The row already carries
// null-ness, so the two nullable columns pass through as-is.
func companyListItemFromRow(r db.ListCompaniesRow) apigen.CompanyListItem {
	return apigen.CompanyListItem{
		Slug:               r.Slug,
		Name:               r.Name,
		JobCount:           int64(r.JobCount),
		Tagline:            nilString(pgconv.TextPtr(r.Tagline)),
		Industries:         r.Industries,
		HqCountry:          nilString(pgconv.TextPtr(r.HqCountry)),
		Collections:        r.Collections,
		FeedbackCount:      int64(r.FeedbackCount),
		FeedbackRatingAvg:  nilFloat32Ptr(pgconv.Float4Ptr(r.FeedbackRatingAvg)),
		AiInterviewReports: optInt64(int64(r.AiInterviewReports)),
	}
}

// companyListItemFromDoc projects a search document onto the same shape. An absent scalar is stored as the empty string and
// must serialize as null like Postgres NULL; an absent array arrives nil and must serialize as [], or the backer marks
// (driven by collections) disappear on search.
func companyListItemFromDoc(d search.CompanyDocument) apigen.CompanyListItem {
	return apigen.CompanyListItem{
		Slug:              d.Slug,
		Name:              d.Name,
		JobCount:          int64(d.JobCount),
		Tagline:           nilString(presentOrNil(d.Tagline)),
		Industries:        orEmpty(d.Industries),
		HqCountry:         nilString(presentOrNil(d.HqCountry)),
		Collections:       orEmpty(d.Collections),
		FeedbackCount:     int64(d.FeedbackCount),
		FeedbackRatingAvg: nilFloat32Ptr(presentOrNilFloat(d.FeedbackRatingAvg)),
	}
}

func nilFloat32Ptr(p *float32) apigen.NilFloat32 {
	if p == nil {
		return apigen.NilFloat32{Null: true}
	}
	return apigen.NewNilFloat32(*p)
}

// presentOrNilFloat treats 0 as absent — CompanyDocument's "no rating" sentinel
// (see its doc comment): a real average is never 0, since ratings are 1-5.
func presentOrNilFloat(f float32) *float32 {
	if f == 0 {
		return nil
	}
	return &f
}

// presentOrNil treats the empty string as absent — the search document's way of spelling NULL.
func presentOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orEmpty normalizes a nil array so it serializes as [] rather than null.
func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// companiesParams are the listing's own params: the name query, the sort
// selector and the pagination window.
var companiesParams = []string{"q", "sort", "limit", "offset"}

// ignoredCompanyParams reports the query params this listing did not filter on.
// Separate from the jobs endpoints' ignoredParams because the vocabularies differ: a company search reads its own facet list
// and none of the `_exclude` / `_mode` conventions, so a jobs facet sent here is ignored and named as such.
func ignoredCompanyParams(query url.Values) []search.UnknownParam {
	return search.UnknownCompanyParams(query, companiesParams)
}

// companyListPage is the list envelope: the items and the params the filter did not read.
func companyListPage(items []apigen.CompanyListItem, total int64, limit, offset int, query url.Values) (*apigen.ListCompaniesOK, error) {
	return &apigen.ListCompaniesOK{Data: items, Meta: apigen.ListMeta{
		Total: total, Limit: int64(limit), Offset: int64(offset),
		IgnoredParams: contractIgnoredParams(ignoredCompanyParams(query)),
	}}, nil
}

// companyPath is the request's own path with its :slug segment swapped for the canonical slug, keeping any sub-resource
// (such as /hiring) and the query string. The segment is located through the matched route's pattern; built from the live
// path so a mounted-elsewhere API redirects within itself; escaped because the slug reaches the Location header.
func companyPath(c *fiber.Ctx, canonical string) string {
	segments := strings.Split(c.Path(), "/")
	for i, part := range strings.Split(c.Route().Path, "/") {
		if part == ":slug" && i < len(segments) {
			segments[i] = url.PathEscape(canonical)
		}
	}
	target := strings.Join(segments, "/")
	if raw := string(c.Request().URI().QueryString()); raw != "" {
		return target + "?" + raw
	}
	return target
}
