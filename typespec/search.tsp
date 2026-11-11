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

  /** True when `total` counts only what was read, not the whole match. */
  total_is_lower_bound?: boolean;
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

/**
 * Full-text and faceted search over open postings. Public; a signed-in caller can also use
 * `hide_seen`, `hide_applied_companies`, `former_employers`, `watchlist`, `min_skill_match`
 * and `sort=match`, which make the answer private. A filter the endpoint does not read is
 * named in `meta.ignored_params`, a value outside a closed vocabulary in `meta.ignored_values`.
 */
@tag("Search")
@route("/jobs/search")
@get
op searchJobs(
  @query q?: string,
  @query q_fields?: string,
  @query sort?: string,
  @query order?: string,
  @query limit?: string,
  @query offset?: string,
  @query hide_seen?: string,
  @query hide_applied_companies?: string,
  @query min_skill_match?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: JobSearchPage;
} | Problem;

/** The same query as `/jobs/search`, always with the full description in the chosen format. */
@tag("Search")
@route("/agent/jobs/search")
@get
op agentSearchJobs(
  @query q?: string,
  @query q_fields?: string,
  @query sort?: string,
  @query order?: string,
  @query limit?: string,
  @query offset?: string,

  /** `text` or `markdown`; anything else leaves the stored HTML. */
  @query description_format?: string,

  @query semantic?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: AgentJobSearchPage;
} | Problem;

/** The count of postings per facet value under the same filters, keyed by public facet param. */
@tag("Search")
@route("/jobs/facets")
@get
op jobFacets(
  @query q?: string,

  /** Comma-separated facet params to count instead of all of them. */
  @query facets?: string,

  @query disjunctive?: string,
): DataWithIgnored<unknown> | Problem;

/** Open postings in the same specialization and place as the job. */
@tag("Search")
@route("/jobs/{slug}/similar")
@get
op similarJobs(
  @path slug: string,
  @query limit?: string,
): Data<unknown[]> | Problem;
