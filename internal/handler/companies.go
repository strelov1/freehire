package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
)

// companyDetailResponse is the public shape of a company together with a page of
// its jobs. Its Jobs field is []jobview.Job, not []db.Job, so the internal job
// id cannot leak through this endpoint — the type enforces the DTO mapping.
type companyDetailResponse struct {
	Company db.Company    `json:"company"`
	Jobs    []jobview.Job `json:"jobs"`
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
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))

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
		Limit:         int32(limit),
		Offset:        int32(offset),
	})
	if err != nil {
		return err
	}

	total, err := a.queries.CountCompanies(c.Context(), db.CountCompaniesParams{
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
	})
	if err != nil {
		return err
	}

	return listResponse(c, companies, total, limit, offset)
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

	return c.JSON(fiber.Map{"data": companyDetailResponse{Company: company, Jobs: views}})
}
