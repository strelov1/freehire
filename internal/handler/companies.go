package handler

import (
	"encoding/json"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
)

// companyDetailResponse is the public shape of a company together with a page of
// its jobs. Its Jobs field is []jobview.Job, not []db.Job, so the internal job
// id cannot leak through this endpoint; its Company is a companyView, not a raw
// db.Company, so the internal bookkeeping columns cannot leak either — both types
// enforce the DTO mapping.
type companyDetailResponse struct {
	Company companyView   `json:"company"`
	Jobs    []jobview.Job `json:"jobs"`
}

// companyView is the public projection of a company for the detail endpoint. It
// mirrors db.Company minus the purely-internal bookkeeping columns (created_at,
// updated_at, is_reference, company_info_at), so those never leak onto
// GET /api/v1/companies/:slug. Every field the company page renders is kept.
type companyView struct {
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Collections      []string        `json:"collections"`
	JobCount         int32           `json:"job_count"`
	Regions          []string        `json:"regions"`
	Countries        []string        `json:"countries"`
	Domains          []string        `json:"domains"`
	CompanyTypes     []string        `json:"company_types"`
	CompanySizes     []string        `json:"company_sizes"`
	Industries       []string        `json:"industries"`
	YearFounded      pgtype.Int4     `json:"year_founded"`
	EmployeeCount    pgtype.Int4     `json:"employee_count"`
	HqCountry        pgtype.Text     `json:"hq_country"`
	OrganizationType pgtype.Text     `json:"organization_type"`
	Tagline          pgtype.Text     `json:"tagline"`
	CompanyInfo      json.RawMessage `json:"company_info"`
	RemoteRegions    []string        `json:"remote_regions"`
	YcBatch          []string        `json:"yc_batch"`
	YcStatus         []string        `json:"yc_status"`
	YcStage          []string        `json:"yc_stage"`
	YcFlags          []string        `json:"yc_flags"`
	Maturity         pgtype.Text     `json:"maturity"`
}

// companyViewFrom projects a stored company onto its public view, dropping only the
// internal bookkeeping columns.
func companyViewFrom(c db.Company) companyView {
	return companyView{
		Slug:             c.Slug,
		Name:             c.Name,
		Collections:      c.Collections,
		JobCount:         c.JobCount,
		Regions:          c.Regions,
		Countries:        c.Countries,
		Domains:          c.Domains,
		CompanyTypes:     c.CompanyTypes,
		CompanySizes:     c.CompanySizes,
		Industries:       c.Industries,
		YearFounded:      c.YearFounded,
		EmployeeCount:    c.EmployeeCount,
		HqCountry:        c.HqCountry,
		OrganizationType: c.OrganizationType,
		Tagline:          c.Tagline,
		CompanyInfo:      c.CompanyInfo,
		RemoteRegions:    c.RemoteRegions,
		YcBatch:          c.YcBatch,
		YcStatus:         c.YcStatus,
		YcStage:          c.YcStage,
		YcFlags:          c.YcFlags,
		Maturity:         c.Maturity,
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
func (a *API) ListCompanies(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	search := c.Query("q")
	vals := queryValues(c)

	// Parse each facet once and feed both queries, so their WHERE clauses can't
	// drift. The query param names (company_type/company_size singular) differ from
	// the plural db columns; every facet is a non-nil slice so pgx sends '{}', not
	// NULL — NULL would defeat the cardinality() short-circuit.
	collections := facetValues(vals, "collections")
	regions := facetValues(vals, "regions")
	countries := facetValues(vals, "countries")
	domains := facetValues(vals, "domains")
	companyTypes := facetValues(vals, "company_type")
	companySizes := facetValues(vals, "company_size")
	remoteRegions := facetValues(vals, "remote_regions")
	ycBatch := facetValues(vals, "yc_batch")
	ycStatus := facetValues(vals, "yc_status")
	ycStage := facetValues(vals, "yc_stage")
	ycFlags := facetValues(vals, "yc_flags")
	maturity := facetValues(vals, "maturity")
	subindustries := facetValues(vals, "subindustries")

	companies, err := a.queries.ListCompanies(c.Context(), db.ListCompaniesParams{
		Search:        search,
		Collections:   collections,
		Regions:       regions,
		Countries:     countries,
		Domains:       domains,
		CompanyTypes:  companyTypes,
		CompanySizes:  companySizes,
		RemoteRegions: remoteRegions,
		YcBatch:       ycBatch,
		YcStatus:      ycStatus,
		YcStage:       ycStage,
		YcFlags:       ycFlags,
		Maturity:      maturity,
		Subindustries: subindustries,
		Limit:         int32(limit),
		Offset:        int32(offset),
	})
	if err != nil {
		return err
	}

	// The unfiltered catalogue count(*) is a cold-cache heap scan (~17s on prod — see
	// EstimateHiringCompanies); every facet/search filter narrows to an index and keeps
	// it cheap. So a filtered request gets the exact count (accurate pagination total),
	// and the pathological unfiltered case gets the O(1) planner estimate, as /jobs does.
	var total int64
	if isCompanyFilter(search, collections, regions, countries, domains, companyTypes,
		companySizes, remoteRegions, ycBatch, ycStatus, ycStage, ycFlags, maturity, subindustries) {
		total, err = a.queries.CountCompanies(c.Context(), db.CountCompaniesParams{
			Search:        search,
			Collections:   collections,
			Regions:       regions,
			Countries:     countries,
			Domains:       domains,
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
		total, err = a.queries.EstimateHiringCompanies(c.Context())
	}
	if err != nil {
		return err
	}

	return listResponse(c, companies, total, limit, offset)
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
func (a *API) CompanySubindustries(c *fiber.Ctx) error {
	rows, err := a.queries.CompanySubindustries(c.Context())
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
func (a *API) GetCompany(c *fiber.Ctx) error {
	slug := c.Params("slug")

	company, err := a.queries.GetCompany(c.Context(), slug)
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit, offset := pageParams(c)

	jobs, err := a.queries.ListJobsByCompany(c.Context(), db.ListJobsByCompanyParams{
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

	return c.JSON(fiber.Map{"data": companyDetailResponse{Company: companyViewFrom(company), Jobs: views}})
}
