import "@typespec/http";

using Http;

namespace Freehire;

// The public company catalogue. The detail, hiring and closed-jobs reads stay hand-written
// because a retired slug answers with a redirect to the canonical one.

/** A row of the catalogue: the company, its open-job count and a few company-info facts for the card. */
model CompanyListItem {
  slug: string;
  name: string;
  job_count: int64;
  tagline: string | null;
  industries: string[];
  hq_country: string | null;

  /** Curated company tags; the catalogue card renders the backer marks from these. */
  collections: string[];

  feedback_count: int64;

  /** Null until a review counts. */
  feedback_rating_avg: float32 | null;

  /** How many people reported an AI interviewer. Absent at zero, so a badge never shows a bare 0. */
  ai_interview_reports?: int64;
}

/** One option of the subindustry vocabulary. */
model CompanySubindustry {
  value: string;
  count: int64;
}

// Ranked companies. `q` and the facet params (collections, regions, countries, domains,
// industries, company_type, company_size, remote_regions, yc_batch, yc_status,
// yc_stage, yc_flags, maturity, subindustries) are read from the raw query,
// comma-joined or repeated; a param the listing does not read is reported in
// `meta.ignored_params`.
/**
 * Most active first. Facet params are repeatable and filter by array overlap (OR within a
 * facet, AND across facets), composably with `q`. `meta.total` reports the count matching
 * the full filter.
 */
@summary("List companies with job counts and denormalized facets; optional filters.")
@useAuth(NoAuth)
@tag("Companies")
@route("/companies")
@get
op listCompanies(
  /** Case-insensitive name substring filter. (e.g. `acme`) */
  @query q?: string,
  @query sort?: string,
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): List<CompanyListItem> | Problem;

// The subindustry vocabulary with company counts, most common first.
/**
 * Backs the searchable “Industry” facet’s option list, most common first. Counts are
 * unconditional (they do not reflect other active list filters).
 */
@summary("Distinct company subindustry vocabulary with company counts.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        value: "payments",
        count: 42,
      },
      #{
        value: "developer-tools",
        count: 31,
      },
    ],
  },
})
@tag("Companies")
@route("/companies/subindustries")
@get
op companySubindustries(): Data<CompanySubindustry[]> | Problem;
