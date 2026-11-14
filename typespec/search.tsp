import "@typespec/http";

using Http;

namespace Freehire;

// The job search surfaces over the Meilisearch index. A job is the jobview wire shape
// (internal/job/jobview), shared with the MCP tools, so it is passed through as `unknown`.
//
// The filter vocabulary (`countries`, `seniority`, `work_mode`, ... and their `_exclude`
// variants) is read from the raw query and is deliberately not declared here: a typed
// param would turn a misspelled or unknown value into a 400, where today the answer drops
// it and says so in `meta.ignored_params` / `meta.ignored_values`. Only the transport
// params are named, all as strings for the same reason.

model JobSearchMeta {
  ...ListMeta;
}

model JobSearchPage {
  data: unknown[];
  meta: JobSearchMeta;
}

model AgentJobSearchMeta {
  ...JobSearchMeta;

  /** "unavailable" when `semantic=true` was asked and the request was answered by keywords. */
  semantic?: string;
}

model AgentJobSearchPage {
  data: unknown[];
  meta: AgentJobSearchMeta;
}

// Full-text and faceted search over open postings. Public; a signed-in caller can also
// use `hide_seen`, `hide_applied_companies`, `former_employers`, `watchlist`,
// `min_skill_match` and `sort=match`, which make the answer private. A filter the
// endpoint does not read is named in `meta.ignored_params`, a value outside a closed
// vocabulary in `meta.ignored_values`.
/**
 * Free-text `q` plus any filter (the "Filtering jobs" section). Repeated values OR;
 * `<param>_mode=and` requires all, `<param>_exclude` excludes. Newest first without `q`,
 * by relevance with it. An unread param is listed in `meta.ignored_params`: check it, a
 * dropped filter looks like a broad result.
 */
@summary("Full-text + faceted search over open jobs.")
@useAuth(NoAuth)
@tag("Search")
@route("/jobs/search")
@get
op searchJobs(
  /**
   * Full-text query over `title`, `company`, `description`, and `location`. The first
   * words are required and trailing ones are dropped when too few postings match, so word
   * order can change the result. Operators: quotes require every quoted word in any order
   * (not a contiguous phrase); `-word` excludes; uppercase `OR` between words matches any
   * alternative (at most 5); `title:` at the start of an alternative matches it in the
   * title only, e.g. `title:"product owner" -junior OR title:produktchef`. `q_fields`
   * narrows the other alternatives. No exact-phrase matching. (e.g. `golang`)
   */
  @query q?: string,
  /**
   * Comma-separated subset of `title,company,description,location` to restrict `q` to,
   * e.g. `title`. A name outside that set drops the whole parameter (reported in
   * `meta.ignored_params`) rather than partially applying. (e.g. `title`)
   */
  @query q_fields?: string,
  /**
   * `true` lets the last word of `q` match as the start of a longer word (`data eng`
   * finds `engineer`), for suggestions while someone is still typing. Off by default:
   * the last word is read as complete, which is much faster on broad queries.
   * (e.g. `true`)
   */
  @query q_prefix?: string,
  /**
   * One of `created_at`, `posted_at`, `view_count`, `salary_min`, `salary_max`. Omit for
   * relevance/newest. (e.g. `posted_at`)
   */
  @query sort?: string,
  /** `asc` or `desc` (default `desc`). (e.g. `desc`) */
  @query order?: string,
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip; `offset + limit` ≤ 10000. (e.g. `0`) */
  @query offset?: string,
  /**
   * `true` leaves out the postings the signed-in caller has already opened (the 1000 most
   * recently viewed, saved, applied to or hidden), inside the search, so `meta.total` and
   * paging stay accurate. A per-person preference, not a filter: a saved search ignores
   * it. Without a session or API key it is not applied and is listed in
   * `meta.ignored_params`. (e.g. `true`)
   */
  @query hide_seen?: string,
  /**
   * `true` excludes every posting of employers represented in the newest 1,000
   * applications made by the signed-in caller. Employers represented only by older
   * applications can remain in results. Applied inside the search, so `meta.total` and
   * paging describe what is left. A per-person preference, not a filter: a saved search
   * carrying it narrows nothing. Without a session or API key, or with a value that is
   * not a boolean, the search runs unhidden and the param is reported in
   * `meta.ignored_params`. (e.g. `true`)
   */
  @query hide_applied_companies?: string,
  /**
   * Keeps only postings whose skills your profile covers by at least this percent
   * (1–100), across the whole ranking. Applies only with `sort=match`, to a signed-in
   * caller with profile skills; the read stops once no later posting can qualify or at a
   * depth bound, and then `meta.total` is a minimum with
   * `meta.total_is_lower_bound: true`. A per-person preference: a saved search ignores
   * it. Otherwise it is not applied and is listed in `meta.ignored_params`. (e.g. `70`)
   */
  @query min_skill_match?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: JobSearchPage;
} | Problem;

// The same query as `/jobs/search`, always with the full description in the chosen
// format.
/**
 * Same text query and facet filters as `/jobs/search`, but each result carries the
 * `description` in full (verbatim from the database) instead of the truncated index
 * preview. Use `description_format` to choose how it is rendered.
 *
 * Plus every filter in the "Filtering jobs" section.
 */
@summary("Search with full descriptions, for programmatic/agent consumers.")
@useAuth(NoAuth)
@tag("Search")
@route("/agent/jobs/search")
@get
op agentSearchJobs(
  /**
   * Full-text query over `title`, `company`, `description`, and `location`. The first
   * words are required and trailing ones are dropped when too few postings match, so word
   * order can change the result. Operators: quotes require every quoted word in any order
   * (not a contiguous phrase); `-word` excludes; uppercase `OR` between words matches any
   * alternative (at most 5); `title:` at the start of an alternative matches it in the
   * title only, e.g. `title:"product owner" -junior OR title:produktchef`. `q_fields`
   * narrows the other alternatives. No exact-phrase matching. (e.g. `golang`)
   */
  @query q?: string,
  /**
   * Comma-separated subset of `title,company,description,location` to restrict `q` to,
   * e.g. `title`. A name outside that set drops the whole parameter (reported in
   * `meta.ignored_params`) rather than partially applying. (e.g. `title`)
   */
  @query q_fields?: string,
  /**
   * `true` lets the last word of `q` match as the start of a longer word (`data eng`
   * finds `engineer`), for suggestions while someone is still typing. Off by default:
   * the last word is read as complete, which is much faster on broad queries.
   * (e.g. `true`)
   */
  @query q_prefix?: string,
  /**
   * One of `created_at`, `posted_at`, `view_count`, `salary_min`, `salary_max`. Omit for
   * relevance/newest. (e.g. `posted_at`)
   */
  @query sort?: string,
  /** `asc` or `desc` (default `desc`). (e.g. `desc`) */
  @query order?: string,
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip; `offset + limit` ≤ 10000. (e.g. `0`) */
  @query offset?: string,

  /** `text` or `markdown`; anything else leaves the stored HTML. */
  @query description_format?: string,

  @query semantic?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: AgentJobSearchPage;
} | Problem;

// The count of postings per facet value under the same filters, keyed by public facet
// param.
/**
 * The same `q` and filters as search, answered with the count of postings per facet value
 * and `stats` (min/max) for the numeric facets. `former_employers` and `watchlist` are
 * ignored. Narrow with `facets=`; naming `company_slug` there adds `companies_total` and
 * `top_companies`.
 */
@summary("Count of matching jobs per facet value (and numeric stats).")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      total: 1820,
      facets: #{
        seniority: #{
          senior: 640,
          middle: 410,
          junior: 120,
        },
        category: #{
          backend: 700,
          frontend: 380,
        },
      },
      stats: #{
        salary_min: #{
          min: 20000,
          max: 400000,
        },
      },
    },
  },
})
@tag("Search")
@route("/jobs/facets")
@get
op jobFacets(
  /** Same full-text query as search. (e.g. `golang`) */
  @query q?: string,

  /**
   * Comma-separated facet params to count instead of all of them, e.g. `skills,seniority`.
   * `company_slug` is counted only when named here. An unknown name is a 400, and so is
   * combining it with `disjunctive`, or `company_slug` with an `OR` query.
   */
  @query facets?: string,

  /**
   * `true` counts each facet under the full filter MINUS its own selection, so a selected
   * facet still shows its siblings' counts — what a live filter sidebar wants. Costs one
   * query per facet.
   */
  @query disjunctive?: string,
): DataWithIgnored<unknown> | Problem;

// Open postings in the same specialization and place as the job.
/**
 * Searches the live Meilisearch index with the source job’s known location and
 * specialization as mandatory filters, ranked by keyword search on its title. Returns an
 * empty list when location or specialization is unknown or no visible matches remain.
 */
@summary("Similar jobs within the same location and specialization.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        public_slug: "senior-go-engineer-acme-1a2b",
        title: "Senior Go Engineer",
        company: "Acme",
      },
    ],
  },
})
@tag("Search")
@route("/jobs/{slug}/similar")
@get
op similarJobs(
  /** The job `public_slug`. (e.g. `senior-go-engineer-acme-1a2b`) */
  @path slug: string,
  /** Max similar jobs. (e.g. `10`) */
  @query limit?: string,
): Data<unknown[]> | Problem;
