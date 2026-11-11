import "@typespec/http";

using Http;

namespace Freehire;

// The public company catalogue. A company is passed through as `unknown`: the list item
// is shared with the search backends. The detail, hiring and closed-jobs reads stay
// hand-written because a retired slug answers with a redirect to the canonical one.

/** One option of the subindustry vocabulary. */
model CompanySubindustry {
  value: string;
  count: int64;
}

/**
 * Ranked companies. `q` and the facet params (collections, regions, countries, domains,
 * industries, company_type, company_size, remote_regions, yc_batch, yc_status, yc_stage,
 * yc_flags, maturity, subindustries) are read from the raw query, comma-joined or repeated;
 * a param the listing does not read is reported in `meta.ignored_params`.
 */
@tag("Companies")
@route("/companies")
@get
op listCompanies(
  @query q?: string,
  @query sort?: string,
  @query limit?: string,
  @query offset?: string,
): List<unknown> | Problem;

/** The subindustry vocabulary with company counts, most common first. */
@tag("Companies")
@route("/companies/subindustries")
@get
op companySubindustries(): Data<CompanySubindustry[]> | Problem;
