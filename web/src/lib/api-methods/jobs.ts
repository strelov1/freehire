import type { Http } from '../api-core';
import type {
  ActivityGranularity,
  ActivityPoint,
  ApiSuggestion,
  CatalogScale,
  DirectApplication,
  DirectApplyForm,
  EmbedProgress,
  EngagementStats,
  FacetCounts,
  IngestStatus,
  Interpretation,
  Job,
  JobMatchResult,
  MatchAnalysisResponse,
  SourceEntry,
  UserGrowthPoint,
} from '../types';
import type { Display } from '#lib/generated/contracts.js';
import type { JobCopy, Page, Slice } from '../api-types';
import type { components } from '#lib/generated/api.js';
import { ApiError, jsonBody, query, toSlice } from '../api-core';

export function jobsApi({ call, request, requestData, baseUrl }: Http) {
  async function listJobs(limit: number, offset: number): Promise<Slice<Job>> {
    return toSlice(await request<Page<Job>>(`/api/v1/jobs${query(limit, offset)}`), offset);
  }

  /** A job by slug. Four words can never be one: `/jobs/{search,find,facets,sitemap}`
   *  are static API routes declared ahead of `/jobs/:slug`, so they answer 200 with
   *  their own shape instead of the 404 a missing job gives. Reaching /jobs/search in
   *  a browser handed the page a list where it expected a posting, and the render died
   *  on the first field it read — a 500 where the honest answer is "no such job". The
   *  shape is the check, not a list of the four: any future static sibling is covered. */
  async function getJob(slug: string): Promise<Job> {
    const data = await requestData<Job>(`/api/v1/jobs/${encodeURIComponent(slug)}`);
    if (!data || typeof data !== 'object' || Array.isArray(data) || !('public_slug' in data)) {
      throw new ApiError(404, `not a job: /jobs/${slug}`);
    }
    return data;
  }

  /** Jobs semantically nearest to the one addressed by `slug` — the "Similar jobs"
   *  section on the detail page. Same Job wire shape as the list, so the same card
   *  renders them; the source job is excluded by the backend. */
  async function getSimilarJobs(slug: string): Promise<Job[]> {
    return requestData<Job[]>(`/api/v1/jobs/${slug}/similar`);
  }

  /** The questions this job's application will ask, as the ATS published them. 404s for
   *  the majority of postings — a form can only be read from a few platforms — so callers
   *  treat a failure as "not known" rather than as an error worth surfacing. */
  async function getApplyForm(slug: string): Promise<Display> {
    return requestData<Display>(`/api/v1/jobs/${slug}/apply-form`);
  }

  /** The application form of a vacancy that takes applications on freehire, with the
   *  caller's prefill and whether they already applied. 404 for any other posting. */
  async function getDirectApplyForm(slug: string): Promise<DirectApplyForm> {
    return requestData<DirectApplyForm>(`/api/v1/jobs/${encodeURIComponent(slug)}/direct-apply`);
  }

  /** Apply on freehire: the employer receives the CV and answers by email. */
  async function sendDirectApplication(slug: string, input: DirectApplication): Promise<void> {
    await call(
      `/api/v1/jobs/${encodeURIComponent(slug)}/direct-apply`,
      jsonBody('POST', input satisfies components['schemas']['DirectApplyRequest']),
    );
  }

  /** The open postings sharing this job's role cluster — the "openings across cities"
   *  list under a collapsed role. `total` is the whole cluster's open size (pre-limit),
   *  so the header stays accurate when `copies` is a capped page. */
  async function getJobCopies(
    slug: string,
    limit?: number,
    offset?: number,
  ): Promise<{ copies: JobCopy[]; total: number }> {
    const qs = new URLSearchParams();
    if (limit != null) qs.set('limit', String(limit));
    if (offset != null) qs.set('offset', String(offset));
    const suffix = qs.toString() ? `?${qs}` : '';
    const res = await request<{ data: JobCopy[]; meta: { total: number } }>(`/api/v1/jobs/${slug}/copies${suffix}`);
    // Every list endpoint returns a `meta` envelope (see the backend contract), so
    // access it directly — same convention as toSlice/listMyJobs/getInbox.
    return { copies: res.data, total: res.meta.total };
  }

  /** How well the job addressed by `slug` is covered by the caller's profile skills:
   *  each job skill classified exact/adjacent/missing plus a coverage percent.
   *  Requires a signed-in caller with a profile (404 otherwise); the sidebar only
   *  calls it in that state. */
  async function getJobMatch(slug: string): Promise<JobMatchResult> {
    return requestData<JobMatchResult>(`/api/v1/jobs/${slug}/match`);
  }

  /** The cached LLM match analysis for a job (never runs the model). `has_cv` is false
   *  when no CV is stored; `analysis` is null when none is cached yet; `stale` marks a
   *  cached analysis whose CV or job changed since. Safe to call on expand. */
  async function getMatchAnalysis(slug: string): Promise<MatchAnalysisResponse> {
    return requestData<MatchAnalysisResponse>(`/api/v1/jobs/${slug}/match-analysis`);
  }

  /** Run the three-stage match prompt-chain over the caller's CV and this job, cache it
   *  per (user, job), and return it fresh. Bound to the explicit compute/recompute
   *  action. With no LLM configured this returns `has_cv` with a null analysis. */
  async function runMatchAnalysis(slug: string): Promise<MatchAnalysisResponse> {
    return requestData<MatchAnalysisResponse>(`/api/v1/jobs/${slug}/match-analysis`, { method: 'POST' });
  }

  /** The same-origin URL for the match SSE stream — the tailoring workspace opens an EventSource on
   *  it (EventSource takes a URL, not our fetch wrapper; the session cookie rides along). */
  function matchAnalysisStreamUrl(slug: string): string {
    return `${baseUrl}/api/v1/jobs/${slug}/match-analysis/stream`;
  }

  /** The same-origin URL for the homepage's live "recently added jobs" SSE feed
   *  (public, no cookie needed — see openspec/changes/add-homepage-recent-jobs-feed). */
  function recentJobsFeedUrl(): string {
    return `${baseUrl}/api/v1/feed/recent`;
  }

  /** Full-text search over jobs. `facets` carries the query text and any facet
   *  filters (built by the caller); pagination is appended here. Results are the
   *  same Job wire shape as listJobs, so views render them with the same
   *  components. `meta.total` is an estimate from the search engine.
   *
   *  Keyword-only: the API has no semantic/hybrid ranking mode (the Meilisearch
   *  semantic index it used to offer via `semantic_ratio` was removed — see
   *  openspec/changes/drop-hybrid-search-pgvector-similar). A blended ranking mode
   *  would score every job by similarity, so a query like "devops" would return the
   *  whole catalogue reordered rather than the handful that match — which reads as
   *  "search is broken". */
  async function searchJobs(
    facets: URLSearchParams,
    limit: number,
    offset: number,
    signal?: AbortSignal,
  ): Promise<Slice<Job>> {
    const params = new URLSearchParams(facets);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<Job>>(`/api/v1/jobs/search?${params}`, { signal }), offset);
  }

  /** Complete a partly-typed query against the catalogue's own vocabulary: the
   *  posting titles it actually carries, plus roles, skills, specializations and
   *  companies.
   *
   *  It completes a PHRASE, so a row can name more than one thing — "Senior Software
   *  Engineer Google" carries the role AND the company, and applying only one of them
   *  discards what was typed. An empty query returns nothing: what an empty box offers
   *  is a curated starting point the client owns (see starterSuggestions). */
  async function suggest(q: string, limit: number, signal?: AbortSignal): Promise<ApiSuggestion[]> {
    const params = new URLSearchParams({ q, limit: String(limit) });
    return requestData<ApiSuggestion[]>(`/api/v1/suggest?${params}`, { signal });
  }

  /** Ask for a specialization, skill, or industry the dictionary does not hold yet. Signed-in
   *  only; 409 when the term already resolves, 429 past the daily cap. */
  function recordDictionaryGap(
    kind: 'specialization' | 'skill' | 'industry',
    term: string,
  ): Promise<{ request_count: number; status: string }> {
    return requestData('/api/v1/dictionary/gaps', jsonBody('POST', { kind, term }));
  }

  /** Turn a written description of a job search into filter values (the AI filter).
   *  Signed-in only. `previous` refines an interpretation already on screen: the
   *  server returns a complete replacement search, not a diff, so the caller always
   *  has one coherent result to show. Nothing is stored on either side. */
  async function interpretSearch(text: string, previous?: Interpretation): Promise<Interpretation> {
    return requestData<Interpretation>(
      '/api/v1/search/interpret',
      jsonBody('POST', { text, previous: previous ?? null }),
    );
  }

  /** The swipe-deck batch: open jobs matching the same facets/query as
   *  `searchJobs`, minus every job the caller has already interacted with
   *  (viewed/saved/applied/dismissed), excluded server-side. The deck records a
   *  view the moment a card is shown, so exclusion drives pagination: each fetch
   *  returns the head of the un-seen deck (offset 0) and the caller dedups held
   *  cards client-side. Authenticated. */
  async function swipeDeck(facets: URLSearchParams, limit: number): Promise<Slice<Job>> {
    const params = new URLSearchParams(facets);
    params.set('limit', String(limit));
    params.set('offset', '0');
    return toSlice(await request<Page<Job>>(`/api/v1/me/tracking/swipe?${params}`), 0);
  }

  /** Facet-distribution counts for the analytics page. `params` carries the same
   *  query text and facet filters as `searchJobs` (built by the caller, e.g. via
   *  `filtersToParams`); the endpoint returns counts instead of a page of jobs.
   *  Empty `facets`/`stats` are normalized to `{}` so callers never see null. */
  // `disjunctive` asks the endpoint to count each facet under the filter minus its
  // own selection (so a selected facet still shows its siblings) — the live-modal mode.
  /** `facets` narrows the count to the named params. Counting is paid per facet
   *  and the wide-valued ones (cities, skills) dominate, so a caller that reads a
   *  specific key should name it. Cannot be combined with `disjunctive`, which
   *  derives its queries from the selection rather than from this list. */
  async function facetCounts(
    params: URLSearchParams,
    opts?: { disjunctive?: boolean; facets?: string[] },
  ): Promise<FacetCounts> {
    const p = new URLSearchParams(params);
    if (opts?.disjunctive) p.set('disjunctive', '1');
    if (opts?.facets?.length) p.set('facets', opts.facets.join(','));
    const res = await request<{ data: FacetCounts }>(`/api/v1/jobs/facets?${p}`);
    return {
      total: res.data.total,
      facets: res.data.facets ?? {},
      stats: res.data.stats ?? {},
      companies_total: res.data.companies_total,
      top_companies: res.data.top_companies,
    };
  }

  /** The public catalogue-activity time series: vacancies added vs. removed per
   *  period, aggregated to `granularity` (day/week/month) over an optional date
   *  range (`from`/`to`, ISO YYYY-MM-DD; the server defaults a sensible recent
   *  window per granularity). The series is dense — empty periods carry zeros —
   *  so the chart draws without gap-filling. Unauthenticated. */
  async function jobsActivity(granularity: ActivityGranularity, from?: string, to?: string): Promise<ActivityPoint[]> {
    const params = new URLSearchParams({ granularity });
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    return requestData<ActivityPoint[]>(`/api/v1/stats/jobs-activity?${params}`);
  }

  /** The public member-growth series: the cumulative count of registered members
   *  per UTC day, from the first registration through today. Dense and
   *  monotonically non-decreasing (gap days repeat the running total).
   *  Aggregate-only — no user field is exposed. Unauthenticated. */
  async function userGrowth(): Promise<UserGrowthPoint[]> {
    return requestData<UserGrowthPoint[]>(`/api/v1/stats/user-growth`);
  }

  /** Aggregate engagement counts across all users (jobs saved / applied / viewed, CVs
   *  uploaded and tailored, matches analyzed, inboxes connected, searches saved).
   *  Aggregate-only, unauthenticated. */
  async function engagementStats(): Promise<EngagementStats> {
    return requestData<EngagementStats>(`/api/v1/stats/engagement`);
  }

  /** The precomputed facet-distribution snapshot the /open page renders: value→count
   *  per facet for countries, skills, seniority, and work_mode. Served from the daily
   *  insights_facet_stats rollup, so this stays off the live Meilisearch facet count.
   *  Returns the same `facets` map shape as `facetCounts`. Aggregate-only,
   *  unauthenticated. An unpopulated snapshot yields empty facet maps. */
  async function statsFacets(): Promise<FacetCounts['facets']> {
    const res = await request<{ data: { facets: FacetCounts['facets'] } }>(`/api/v1/stats/facets`);
    return res.data.facets ?? {};
  }

  /** The skills most often listed beside one skill, as skill → postings listing both.
   *  Served from the daily insights_skill_neighbours rollup, so the /skills/<slug> page
   *  never asks for a live skills facet count — 100-700 ms of engine time each, which a
   *  crawler burst over cold glossary URLs turned into 500s. Empty until the rollup has
   *  run, or for a skill it never saw. Unauthenticated. */
  async function skillNeighbours(slug: string): Promise<Record<string, number>> {
    const res = await request<{ data: { skill: string; count: number }[] }>(
      `/api/v1/stats/skills/${encodeURIComponent(slug)}/neighbours`,
    );
    return Object.fromEntries((res.data ?? []).map((n) => [n.skill, n.count]));
  }

  /** How big the catalogue is, as one snapshot: open postings, companies, sources,
   *  ATS platforms and Telegram channels. Every surface quoting catalogue scale reads
   *  this one call, so /about and /open cannot show different numbers — which they
   *  could when each took its own list total at its own moment.
   *
   *  Never fails: with no published snapshot the backend answers with an approximate
   *  open-job count and `exact: false`. Aggregate-only, unauthenticated. */
  async function catalogScale(): Promise<CatalogScale> {
    return requestData<CatalogScale>(`/api/v1/stats/catalog`);
  }

  /** Semantic-embedding coverage of the catalogue for the public /semantic-search page.
   *  Unmeasured figures are `null`. Aggregate-only, unauthenticated. */
  async function embedProgress(): Promise<EmbedProgress> {
    return requestData<EmbedProgress>(`/api/v1/stats/embeddings`);
  }

  /** The same-origin URL for the live stream of `embedProgress` documents (public). */
  function embedProgressStreamUrl(): string {
    return `${baseUrl}/api/v1/stats/embeddings/stream`;
  }

  /** The public status read: the site/API's own live status (`site`), plus a
   *  per-provider ingest-fleet health rollup with a derived
   *  operational/degraded/down verdict and an overall status. Sanitized
   *  (no error text or board identifiers), aggregate-only, unauthenticated. */
  async function ingestStatus(): Promise<IngestStatus> {
    return requestData<IngestStatus>(`/api/v1/status`);
  }

  /** The public source catalogue: every source the catalogue is built from — each
   *  registered crawl adapter, plus any source carrying postings without one — with what
   *  it currently holds, how its crawl is doing, and the host a logo resolves from.
   *  Sanitized (no error text, board identifiers or posting URLs), aggregate-only,
   *  unauthenticated. One request serves the whole /sources page. */
  async function listSources(): Promise<SourceEntry[]> {
    return requestData<SourceEntry[]>(`/api/v1/sources`);
  }

  return {
    listJobs,
    getJob,
    getSimilarJobs,
    getApplyForm,
    getDirectApplyForm,
    sendDirectApplication,
    getJobCopies,
    getJobMatch,
    getMatchAnalysis,
    runMatchAnalysis,
    matchAnalysisStreamUrl,
    recentJobsFeedUrl,
    searchJobs,
    suggest,
    recordDictionaryGap,
    interpretSearch,
    swipeDeck,
    facetCounts,
    jobsActivity,
    userGrowth,
    engagementStats,
    statsFacets,
    skillNeighbours,
    catalogScale,
    embedProgress,
    embedProgressStreamUrl,
    ingestStatus,
    listSources,
  };
}
