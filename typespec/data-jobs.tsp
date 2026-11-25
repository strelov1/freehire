import "@typespec/http";

using Http;

namespace Freehire;

// The Data plan's job reads: the same answers as `/jobs/search` and `/jobs/{slug}`, plus the
// employer's own links (`url`, `source_url`) that every other surface leaves out.
/**
 * The same text query and filters as `GET /jobs/search` (the "Filtering jobs" section), with
 * each job's `url` (the employer's own posting) and `source_url` where it differs. The public
 * job reads leave both out and link only through `apply_url`. Needs the Data plan: any other
 * account gets `403`. Never cached: the answer is private to the plan holder.
 */
@summary("Search open jobs with their original links (Data plan).")
@useAuth(SessionOrKey)
@tag("Data plan")
@route("/data/jobs/search")
@get
op searchDataJobs(
  /** Full-text query, as on `/jobs/search`. (e.g. `golang`) */
  @query q?: string,
  /** As on `/jobs/search`. (e.g. `title`) */
  @query q_fields?: string,
  /** As on `/jobs/search`. (e.g. `true`) */
  @query q_prefix?: string,
  /** As on `/jobs/search`. (e.g. `posted_at`) */
  @query sort?: string,
  /** `asc` or `desc` (default `desc`). (e.g. `desc`) */
  @query order?: string,
  /** Page size, 1–100. (e.g. `100`) */
  @query limit?: string,
  /** Rows to skip; `offset + limit` ≤ 10000. (e.g. `0`) */
  @query offset?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: JobSearchPage;
} | Problem;

/**
 * The same answer as `GET /jobs/{slug}`, with the job's `url` and `source_url`. Needs the
 * Data plan: any other account gets `403`.
 */
@summary("A single job with its original links (Data plan).")
@useAuth(SessionOrKey)
@tag("Data plan")
@route("/data/jobs/{slug}")
@get
op getDataJob(/** The job `public_slug`. (e.g. `senior-go-engineer-acme-1a2b`) */ @path slug: string): Data<Job> | Problem;
