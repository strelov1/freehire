package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"path"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/industrytag"
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
	// companySearch is the company-search backend backing GET /api/v1/companies,
	// kept separate from the jobs search so the companies index stays fully decoupled
	// from it. Nil when Meilisearch is unconfigured, or on any query error, the list
	// falls back to the Postgres substring path so /companies never depends on
	// Meilisearch being up.
	companySearch companySearcher
}

func newCompaniesHandlers(queries *db.Queries, companySearch companySearcher) *companiesHandlers {
	return &companiesHandlers{queries: queries, companySearch: companySearch}
}

func (h *companiesHandlers) register(api fiber.Router, mw middleware) {
	readLimit := publicReadLimiter(mw.throttler)
	api.Get("/companies", readLimit, h.ListCompanies)
	api.Get("/companies/subindustries", readLimit, h.CompanySubindustries)
	api.Get("/companies/:slug", readLimit, mw.optional, h.GetCompany)
}

// companySearcher is the company-search backend the list handler depends on.
// *search.Client satisfies it; the unit tests inject a fake. A nil companySearcher
// (Meilisearch unconfigured) routes every request to the Postgres path, as does any
// search error — see ListCompanies.
type companySearcher interface {
	SearchCompanies(ctx context.Context, p search.CompanySearchParams) (search.CompanyResult, error)
}

// companyDetailResponse is the public shape of a company together with a page of
// its jobs. Its Jobs field is []jobview.Job, not []db.Job, so the internal job
// id cannot leak through this endpoint; its Company is a companyView, not a raw
// db.Company, so the internal bookkeeping columns cannot leak either — both types
// enforce the DTO mapping.
type companyDetailResponse struct {
	Company companyView   `json:"company"`
	Jobs    []jobview.Job `json:"jobs"`
	// ReferralAvailable is true when the company has at least one approved employee
	// referrer, so the company page can show the "ask for a referral" affordance.
	ReferralAvailable bool `json:"referral_available"`
	// Response is how many of the applications we can OBSERVE the outcome of were
	// answered. Absent below the sample gate — which is nearly every company today,
	// and the correct answer rather than an unfinished one.
	Response *companyResponse `json:"response,omitempty"`
}

// companyView is the public projection of a company for the detail endpoint. It
// mirrors db.Company minus the purely-internal bookkeeping columns (created_at,
// updated_at, is_reference, company_info_at), so those never leak onto
// GET /api/v1/companies/:slug. Every field the company page renders is kept.
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
	// The nullable columns are pointers, not pgtype: this file's own companyListItem says
	// why ("without putting a persistence vocabulary on a public contract"), and the two
	// company endpoints answered the same logical fields through two different encodings
	// until they agreed. The wire is unchanged — pgtype's MarshalJSON already emitted a bare
	// number/string or null — but the shape of GET /companies/:slug is now decided here
	// rather than by a pgx minor version.
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
	// UpvoteCount/DownvoteCount are the company's materialized public thumbs counters,
	// served straight from the companies columns. MyVote is the caller's own vote
	// (-1, 0, 1), caller-scoped — set only on the auth-aware detail read, 0 otherwise.
	UpvoteCount   int32 `json:"upvote_count"`
	DownvoteCount int32 `json:"downvote_count"`
	MyVote        int32 `json:"my_vote"`
	// FeedbackCount/FeedbackRatingAvg are the company's materialized feedback
	// counters (internal/engage/companyfeedback), the same read-straight-from-the-row
	// shape as UpvoteCount/DownvoteCount above.
	FeedbackCount     int32    `json:"feedback_count"`
	FeedbackRatingAvg *float32 `json:"feedback_rating_avg"`
}

// companyViewFrom projects a stored company onto its public view, dropping only the
// internal bookkeeping columns.
func companyViewFrom(c db.Company) companyView {
	return companyView{
		Slug:              c.Slug,
		Name:              c.Name,
		Collections:       c.Collections,
		JobCount:          c.JobCount,
		Regions:           c.Regions,
		Countries:         c.Countries,
		Domains:           c.Domains,
		CompanyTypes:      c.CompanyTypes,
		CompanySizes:      c.CompanySizes,
		Industries:        c.Industries,
		YearFounded:       pgconv.IntPtr(c.YearFounded),
		EmployeeCount:     pgconv.IntPtr(c.EmployeeCount),
		HqCountry:         pgconv.TextPtr(c.HqCountry),
		OrganizationType:  pgconv.TextPtr(c.OrganizationType),
		Tagline:           pgconv.TextPtr(c.Tagline),
		CompanyInfo:       c.CompanyInfo,
		RemoteRegions:     c.RemoteRegions,
		YcBatch:           c.YcBatch,
		YcStatus:          c.YcStatus,
		YcStage:           c.YcStage,
		YcFlags:           c.YcFlags,
		Maturity:          pgconv.TextPtr(c.Maturity),
		UpvoteCount:       c.UpvoteCount,
		DownvoteCount:     c.DownvoteCount,
		FeedbackCount:     c.FeedbackCount,
		FeedbackRatingAvg: pgconv.Float4Ptr(c.FeedbackRatingAvg),
	}
}

// ListCompanies returns a page of companies with their denormalized job counts,
// most active first. An optional `q` query param filters by a case-insensitive
// name substring, and repeatable facet params — collections/regions/countries/
// domains/company_type/company_size/remote_regions/yc_batch/yc_status — filter
// against the company's denormalized facet arrays by array overlap (OR within a
// facet, AND across facets), composably with `q`. `remote_regions` is the
// job-derived remote-hiring facet; `yc_batch`/`yc_status` are the curated YC
// directory facets. meta.total reports the count matching the full filter so
// pagination is correct.
func (h *companiesHandlers) ListCompanies(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	search := c.Query("q")
	// sort=rating orders by feedback_rating_avg (see ListCompanies in
	// internal/platform/db/queries/companies.sql); any other value (including absent)
	// is the existing job_count DESC, name ordering. Forces the Postgres path
	// below regardless of any filter present — rating isn't a Meili-sortable
	// attribute yet (see companySettings' doc comment).
	sort := c.Query("sort")
	vals := queryValues(c)

	// Parse each facet once and feed both queries, so their WHERE clauses can't
	// drift. The query param names (company_type/company_size singular) differ from
	// the plural db columns; every facet is a non-nil slice so pgx sends '{}', not
	// NULL — NULL would defeat the cardinality() short-circuit.
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

	// The industry facet reads two columns, so it needs the requested industries
	// translated into the job-derived vocabulary as well. Meili's own translation
	// happens inside search.CompanyFilterFromValues; both call the same helper, and
	// an integration test compares the two backends' matched sets.
	industryDomains := industrytag.DomainsForIndustries(industries)

	// One list feeds both isCompanyFilter calls below. They used to be written out
	// separately and the second forgot `industries`, so a request filtered only by
	// industry was counted as unfiltered and answered with the catalogue-wide planner
	// estimate instead of its own total — silently, since the argument is variadic.
	facets := [][]string{collections, regions, countries, domains, industries,
		companyTypes, companySizes, remoteRegions, ycBatch, ycStatus, ycStage,
		ycFlags, maturity, subindustries}

	// Company search is served by the Meilisearch companies index when configured and
	// a filter (q or any facet) is present — it gives relevance-first, typo-tolerant
	// ranking with job_count as the tiebreaker. The unfiltered catalogue keeps the
	// Postgres ordering (job_count DESC, name) and its O(1) total estimate, so it is
	// deliberately not routed to Meili. On ANY Meili error the request falls through to
	// the Postgres substring path below, so /companies never depends on Meili being up.
	//
	// sort=rating also stays off Meili even when a filter is present: rating isn't a
	// Meili-sortable attribute yet (see companySettings), so routing there would
	// silently ignore the caller's requested order instead of honouring it.
	if h.companySearch != nil && sort != "rating" && isCompanyFilter(search, facets...) {
		items, total, err := h.companyHitsViaMeili(c.Context(), search, vals, limit, offset)
		if err == nil {
			return listResponseWithIgnored(c, items, total, limit, offset, ignoredCompanyParams(c))
		}
		log.Printf("companies: meili search fell back to postgres (q=%q): %v", search, err)
	}

	companies, err := h.queries.ListCompanies(c.Context(), db.ListCompaniesParams{
		Search:          search,
		Collections:     collections,
		Regions:         regions,
		Countries:       countries,
		Domains:         domains,
		Industries:      industries,
		IndustryDomains: industryDomains,
		CompanyTypes:    companyTypes,
		CompanySizes:    companySizes,
		RemoteRegions:   remoteRegions,
		YcBatch:         ycBatch,
		YcStatus:        ycStatus,
		YcStage:         ycStage,
		YcFlags:         ycFlags,
		Maturity:        maturity,
		Subindustries:   subindustries,
		Sort:            sort,
		Limit:           int32(limit),
		Offset:          int32(offset),
	})
	if err != nil {
		return err
	}

	// The unfiltered catalogue count(*) is a cold-cache heap scan (~17s on prod — see
	// EstimateHiringCompanies); every facet/search filter narrows to an index and keeps
	// it cheap. So a filtered request gets the exact count (accurate pagination total),
	// and the pathological unfiltered case gets the O(1) planner estimate, as /jobs does.
	var total int64
	if isCompanyFilter(search, facets...) {
		total, err = h.queries.CountCompanies(c.Context(), db.CountCompaniesParams{
			Search:          search,
			Collections:     collections,
			Regions:         regions,
			Countries:       countries,
			Domains:         domains,
			Industries:      industries,
			IndustryDomains: industryDomains,
			CompanyTypes:    companyTypes,
			CompanySizes:    companySizes,
			RemoteRegions:   remoteRegions,
			YcBatch:         ycBatch,
			YcStatus:        ycStatus,
			YcStage:         ycStage,
			YcFlags:         ycFlags,
			Maturity:        maturity,
			Subindustries:   subindustries,
		})
	} else {
		total, err = h.queries.EstimateHiringCompanies(c.Context())
	}
	if err != nil {
		return err
	}

	items := make([]companyListItem, len(companies))
	for i, row := range companies {
		items[i] = companyListItemFromRow(row)
	}
	return listResponseWithIgnored(c, items, total, limit, offset, ignoredCompanyParams(c))
}

// isCompanyFilter reports whether a /companies request carries any name search or facet
// constraint — the exact-vs-estimate meta.total gate in ListCompanies. Every facet arrives
// as a non-nil slice, so len == 0 means unset.
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

// CompanySubindustries serves the distinct subindustry vocabulary with company counts,
// most common first, backing the searchable "Industry" facet's option list. Counts are
// unconditional (they do not reflect other active list filters).
func (h *companiesHandlers) CompanySubindustries(c *fiber.Ctx) error {
	rows, err := h.queries.CompanySubindustries(c.Context())
	if err != nil {
		return err
	}
	out := make([]subindustryFacet, 0, len(rows))
	for _, r := range rows {
		out = append(out, subindustryFacet{Value: r.Value.String, Count: r.Count})
	}
	return c.JSON(fiber.Map{"data": out})
}

// facetValues reads the repeatable values of one facet query param, dropping empty
// entries and always returning a non-nil slice (so pgx encodes '{}' rather than
// NULL for an absent facet, keeping the SQL cardinality() short-circuit true).
func facetValues(vals url.Values, key string) []string {
	out := make([]string, 0, len(vals[key]))
	for _, v := range vals[key] {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// GetCompany returns a single company together with a page of its jobs. The
// company is read from companies and its jobs from a single-table filter on
// company_slug — no join between the two tables.
func (h *companiesHandlers) GetCompany(c *fiber.Ctx) error {
	slug := c.Params("slug")

	company, err := h.queries.GetCompany(c.Context(), slug)
	if errors.Is(err, pgx.ErrNoRows) {
		// A slug a merge retired keeps working, rather than becoming a dead end that also
		// drops whatever the page had earned in search. The lookup runs only AFTER the miss,
		// so a live company always wins over a stale alias — a company that came back is
		// never shadowed by the row that once retired it.
		canonical, aliasErr := h.queries.GetCompanySlugAlias(c.Context(), slug)
		switch {
		case aliasErr == nil:
			return c.Redirect(companyPath(c, canonical), fiber.StatusMovedPermanently)
		case !errors.Is(aliasErr, pgx.ErrNoRows):
			// A database failure is not "no such company". Reporting it as 404 would tell a
			// crawler to drop a page that exists, and hide the outage from the error rate.
			return aliasErr
		}
		return err // RenderError maps pgx.ErrNoRows to 404.
	}
	if err != nil {
		return err
	}

	limit, offset := pageParams(c)

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

	// Referral availability: best-effort — a lookup error degrades to false (block hidden),
	// never failing the company read.
	referralAvailable, _ := h.queries.CompanyHasApprovedReferrer(c.Context(), slug)

	view := companyViewFrom(company)
	// Caller's own thumbs vote, overlaid only when signed in (OptionalAuth attaches
	// the id on this public read). Best-effort: a lookup error leaves my_vote 0.
	if userID, ok := auth.UserID(c); ok {
		if mv, err := h.queries.GetCompanyVote(c.Context(), db.GetCompanyVoteParams{UserID: userID, CompanySlug: slug}); err == nil {
			view.MyVote = int32(mv)
		}
	}

	return c.JSON(fiber.Map{"data": companyDetailResponse{
		Company:           view,
		Jobs:              views,
		ReferralAvailable: referralAvailable,
		Response:          companyResponseRate(c.Context(), h.queries, slug),
	}})
}

// companyHitsViaMeili runs the ranked company search and projects each hit onto the list wire
// shape — the same type the Postgres path serves, so the two cannot disagree. meta.total is
// Meilisearch's estimated filtered total, which backs list pagination exactly as CountCompanies
// does on the Postgres path.
func (h *companiesHandlers) companyHitsViaMeili(ctx context.Context, query string, vals url.Values, limit, offset int) ([]companyListItem, int64, error) {
	res, err := h.companySearch.SearchCompanies(ctx, search.CompanySearchParams{
		Query:  query,
		Filter: search.CompanyFilterFromValues(vals),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]companyListItem, len(res.Hits))
	for i, h := range res.Hits {
		items[i] = companyListItemFromDoc(h)
	}
	return items, res.Total, nil
}

// companyListItem is the public projection of a company for the list endpoint. It exists so the
// response shape is owned here rather than by sqlc: served straight, a generated row makes
// `make sqlc` an API-changing operation — a renamed column or a changed SELECT alias would
// rewrite the public JSON with nothing to compile against. Both backends project onto this one
// type, so a field added for one cannot be silently missing from the other.
//
// The nullable columns are *string rather than pgtype.Text: identical on the wire in all three
// states ("x", "", null — pinned by the golden bodies in companies_test.go), without putting a
// persistence vocabulary on a public contract. A plain string would collapse null into "" and
// change the response for every company with no tagline.
type companyListItem struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	JobCount    int32    `json:"job_count"`
	Tagline     *string  `json:"tagline"`
	Industries  []string `json:"industries"`
	HqCountry   *string  `json:"hq_country"`
	Collections []string `json:"collections"`
	// FeedbackCount/FeedbackRatingAvg are the company's materialized feedback
	// counters (internal/engage/companyfeedback) — the same fields the single-company
	// detail view (companyView) already serves.
	FeedbackCount     int32    `json:"feedback_count"`
	FeedbackRatingAvg *float32 `json:"feedback_rating_avg"`
}

// companyListItemFromRow projects the Postgres read onto the wire shape. The row already carries
// null-ness, so the two nullable columns pass through as-is.
func companyListItemFromRow(r db.ListCompaniesRow) companyListItem {
	return companyListItem{
		Slug:              r.Slug,
		Name:              r.Name,
		JobCount:          r.JobCount,
		Tagline:           pgconv.TextPtr(r.Tagline),
		Industries:        r.Industries,
		HqCountry:         pgconv.TextPtr(r.HqCountry),
		Collections:       r.Collections,
		FeedbackCount:     r.FeedbackCount,
		FeedbackRatingAvg: pgconv.Float4Ptr(r.FeedbackRatingAvg),
	}
}

// companyListItemFromDoc projects a company search document onto the same shape, carrying the
// two rules the search path has always needed. A document stores an absent scalar as the empty
// string, which must serialize as null like the Postgres NULL; and an absent array arrives nil,
// which must serialize as [] like the Postgres '{}'. Collections drive the backer marks, so an
// array that came back null instead would make those marks disappear the moment a user searched.
func companyListItemFromDoc(d search.CompanyDocument) companyListItem {
	return companyListItem{
		Slug:              d.Slug,
		Name:              d.Name,
		JobCount:          d.JobCount,
		Tagline:           presentOrNil(d.Tagline),
		Industries:        orEmpty(d.Industries),
		HqCountry:         presentOrNil(d.HqCountry),
		Collections:       orEmpty(d.Collections),
		FeedbackCount:     d.FeedbackCount,
		FeedbackRatingAvg: presentOrNilFloat(d.FeedbackRatingAvg),
	}
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
//
// Separate from the jobs endpoints' ignoredParams because the vocabularies are
// separate: a company search reads its own facet list and none of the
// `_exclude` / `_mode` conventions, so a jobs facet sent here is ignored and has
// to be named as such.
func ignoredCompanyParams(c *fiber.Ctx) []search.UnknownParam {
	return search.UnknownCompanyParams(queryValues(c), companiesParams)
}

// companyPath is the request's own path with the final segment swapped for the canonical
// slug, query string preserved. Built from the live path rather than a hardcoded prefix so a
// mounted-elsewhere API redirects within itself; escaped because the slug reaches the
// Location header.
func companyPath(c *fiber.Ctx, canonical string) string {
	base := path.Dir(c.Path())
	target := path.Join(base, url.PathEscape(canonical))
	if raw := string(c.Request().URI().QueryString()); raw != "" {
		return target + "?" + raw
	}
	return target
}
