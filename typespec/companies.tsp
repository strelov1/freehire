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
@opExample(#{
  returnType: #{
      data: #[
        #{
          slug: "acme",
          name: "Acme",
          job_count: 12,
          tagline: "Payments for builders",
          industries: #[
            "payments",
          ],
          hq_country: "DE",
          collections: #[
            "yc",
          ],
          feedback_count: 1,
          feedback_rating_avg: null,
        },
      ],
      meta: #{
        total: 1,
        limit: 20,
        offset: 0,
      },
    },
})
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

/** One company whose site matches a requested domain. */
model CompanyByDomain {
  /** The normalized domain it matched. */
  domain: string;
  slug: string;
  name: string;
  /** Open postings; 0 when the company is not hiring now. */
  job_count: int64;
  /** The site as stored (`website`, else `homepage`). */
  website: string;
}

model CompanyByDomainMeta {
  /** Requested domains that matched no company, normalized. */
  unmatched: string[];
}

model CompanyByDomainPage {
  data: CompanyByDomain[];
  meta: CompanyByDomainMeta;
}

// `domain` is read from the raw query, repeated or comma-joined.
/**
 * Resolves company web domains to companies. Each `domain` is a bare domain or a full URL;
 * it is reduced to a lowercase host without `www.`, port or trailing dot, and compared
 * with the same reduction of the company's site. Every match is returned (two companies
 * can share a site), including companies with `job_count` 0. Up to 100 domains. Then read
 * the postings with `GET /jobs?company_slug=<slug>`.
 */
@summary("Find companies by web domain.")
@useAuth(NoAuth)
@tag("Companies")
@opExample(#{
  returnType: #{
    data: #[
      #{
        domain: "acme.io",
        slug: "acme",
        name: "Acme",
        job_count: 12,
        website: "https://www.acme.io/",
      },
    ],
    meta: #{ unmatched: #["nope.example"] },
  },
})
@route("/companies/by-domain")
@get
op companiesByDomain(
  /** Company domain or URL; repeatable or comma-joined, at most 100. (e.g. `acme.io`) */
  @query domain?: string,
): CompanyByDomainPage | Problem;
