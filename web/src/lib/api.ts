// The only module that knows the API base URL and wire shapes. Views call the
// typed functions below; they never touch fetch or URLs directly. List endpoints
// return a `Slice` so callers (and the Paginator) stay ignorant of how each one
// signals more pages.
//
// The client is built by `createApi(fetch)` so the same code runs on the server
// and in the browser: a SvelteKit server `load` passes `event.fetch` (which
// forwards the request's auth cookie and resolves relative /api URLs), while the
// browser uses the default `api` instance (global fetch, same-origin). Binding
// fetch per call site — not a module-level variable — keeps concurrent SSR
// requests from sharing (and racing on) a session.

import type { CvMeta, CvRecord, CreateCvInput, UpdateCvInput } from './cv';
import type {
  Job,
  EmailLinking,
  TrackedApplication,
  Company,
  CompanyListItem,
  FacetCounts,
  ListMeta,
  MyJob,
  MyJobCounts,
  PipelineStats,
  User,
  UserJob,
  ApiKey,
  CreatedApiKey,
  SavedSearch,
  Board,
  UserProfile,
  Subscription,
  TelegramStatus,
  Submission,
  SubmissionInput,
  Contribution,
  Report,
  ReportInput,
  Verdict,
  ATSResponse,
  JobMatch,
  JobFitResponse,
  JobFitQuota,
  MyAnalysisItem,
  ResumeProfile,
  ResumeMeta,
  ActivityGranularity,
  ActivityPoint,
  UserGrowthPoint,
  EngagementStats,
  IngestStatus,
  LocationPreferences,
} from './types';

/** A page of list items, optionally the total matching the query (endpoints that
 *  report one), and whether more remain. */
export interface Slice<T> {
  items: T[];
  total?: number;
  hasMore: boolean;
}

interface Page<T> {
  data: T[];
  meta: ListMeta;
}

/** One entry in a sitemap sub-file: the public slug and its lastmod. Kept slim
 *  on purpose — the sitemap never needs the full job/company row. */
export interface SitemapEntry {
  slug: string;
  updated_at: string;
}

/** Aggregate market-insight wire shapes (the /api/v1/insights/* reads). */
export interface InsightRole {
  category: string;
  seniority: string;
  open_count: number;
  growth: number;
}
export interface InsightSkill {
  skill: string;
  open_count: number;
  growth: number;
}
export interface InsightSalaryBand {
  seniority: string;
  currency: string;
  period: string;
  sample_size: number;
  p25: number;
  p50: number;
  p75: number;
}

/** One posting in a role cluster — a single city's opening under a collapsed role
 *  (see the /jobs/:slug/copies endpoint). Each keeps its own location and apply URL. */
export interface JobCopy {
  public_slug: string;
  location: string;
  apply_url: string;
  posted_at: string | null;
}

/** A non-2xx API response. Carries the HTTP status so callers can branch on it
 *  (e.g. 401 invalid credentials, 409 email taken) instead of parsing strings.
 *  `message` is the backend's `{ "error": msg }` text when present, so logs and
 *  any raw-error surface read the real reason rather than a bare status line. */
export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

/** Extract a human-readable message from a failed response. The backend's
 *  standard error envelope is `{ "error": msg }`; surface that when present,
 *  falling back to the status line for a non-JSON error (e.g. a proxy 502). */
async function errorMessage(res: Response): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === 'string') return body.error;
  } catch {
    // Body was not JSON (proxy/HTML error page) — fall through to the status line.
  }
  return `${res.status} ${res.statusText}`;
}

function query(limit: number, offset: number): string {
  return `?limit=${limit}&offset=${offset}`;
}

/** Turn a count-bearing page into a Slice; more remain unless we've reached total. */
function toSlice<T>(page: Page<T>, offset: number): Slice<T> {
  return {
    items: page.data,
    total: page.meta.total,
    hasMore: offset + page.data.length < page.meta.total,
  };
}

/** Build an API client bound to a specific fetch and base URL.
 *
 *  - Browser: the default `api` uses global fetch and an empty base, so requests
 *    are relative and same-origin (the auth cookie rides along; see SPA-era note).
 *  - SvelteKit server `load`: pass `event.fetch` and the internal API origin
 *    (`serverApi`), because a server-side relative `/api` would hit the Node app
 *    itself, not nginx→Go. `baseUrl` resolves that to a real server-to-server call. */
// --- Gmail inbox wire shapes ---------------------------------------------

export interface GmailStatus {
  connected: boolean;
  available?: boolean; // whether the connect flow is configured server-side
  email?: string;
  status?: string;
}

/** The hosted-mailbox option: the caller's address (null when none) + whether
 *  the feature is configured server-side. */
export interface MailboxStatus {
  available: boolean;
  address: string | null;
}

/** The account switcher value: '' = all sources. */
export type InboxSource = '' | 'gmail' | 'hosted';

/** One row in the flat inbox listing. */
export interface InboxMessage extends EmailLinking {
  id: number;
  source: string;
  external_id: string;
  from_addr: string;
  from_name: string;
  subject: string;
  snippet: string;
  received_at: string;
  read: boolean;
}

/** One message in full, for the reading pane. */
export interface EmailBody extends EmailLinking {
  id: number;
  source: string;
  external_id: string;
  from_addr: string;
  from_name: string;
  subject: string;
  body_text: string;
  body_html: string;
  received_at: string;
  read: boolean;
}

export function createApi(
  fetchImpl: typeof fetch = fetch,
  baseUrl = '',
  defaultHeaders: Record<string, string> = {},
) {
  /** The single place this module touches fetch. Always sends credentials so the
   *  auth cookie rides along, and turns a non-2xx into an ApiError. `defaultHeaders`
   *  lets a server caller forward the request's Cookie to an absolute API URL
   *  (where `event.fetch` would not). */
  async function call(path: string, init?: RequestInit): Promise<Response> {
    const res = await fetchImpl(`${baseUrl}${path}`, {
      credentials: 'include',
      ...init,
      headers: { ...defaultHeaders, ...init?.headers },
    });
    if (!res.ok) {
      throw new ApiError(res.status, await errorMessage(res));
    }
    return res;
  }

  /** Call `path` and return the decoded JSON body. A bare call (no init) is a GET. */
  async function request<T>(path: string, init?: RequestInit): Promise<T> {
    return (await call(path, init)).json() as Promise<T>;
  }

  /** Request a `{ data: T }` envelope and unwrap it. Nearly every endpoint wraps
   *  its payload this way, so this collapses the request+`.data` unwrap into one call. */
  async function requestData<T>(path: string, init?: RequestInit): Promise<T> {
    return (await request<{ data: T }>(path, init)).data;
  }

  /** Build the request init for a JSON body (POST/PATCH/PUT). Single-sources the
   *  Content-Type header and JSON.stringify so no call site repeats them. */
  function jsonBody(method: string, body: unknown): RequestInit {
    return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
  }

  async function listJobs(limit: number, offset: number): Promise<Slice<Job>> {
    return toSlice(await request<Page<Job>>(`/api/v1/jobs${query(limit, offset)}`), offset);
  }

  async function getJob(slug: string): Promise<Job> {
    return requestData<Job>(`/api/v1/jobs/${slug}`);
  }

  /** Jobs semantically nearest to the one addressed by `slug` — the "Similar jobs"
   *  section on the detail page. Same Job wire shape as the list, so the same card
   *  renders them; the source job is excluded by the backend. */
  async function getSimilarJobs(slug: string): Promise<Job[]> {
    return requestData<Job[]>(`/api/v1/jobs/${slug}/similar`);
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
    const res = await request<{ data: JobCopy[]; meta: { total: number } }>(
      `/api/v1/jobs/${slug}/copies${suffix}`,
    );
    return { copies: res.data, total: res.meta?.total ?? res.data.length };
  }

  /** How well the job addressed by `slug` is covered by the caller's profile skills:
   *  each job skill classified exact/adjacent/missing plus a coverage percent.
   *  Requires a signed-in caller with a profile (404 otherwise); the sidebar only
   *  calls it in that state. */
  async function getJobMatch(slug: string): Promise<JobMatch> {
    return requestData<JobMatch>(`/api/v1/jobs/${slug}/match`);
  }

  /** The cached LLM fit analysis for a job (never runs the model). `has_cv` is false
   *  when no CV is stored; `analysis` is null when none is cached yet; `stale` marks a
   *  cached analysis whose CV or job changed since. Safe to call on expand. */
  async function getJobFit(slug: string): Promise<JobFitResponse> {
    return requestData<JobFitResponse>(`/api/v1/jobs/${slug}/fit`);
  }

  /** Run the three-stage fit prompt-chain over the caller's CV and this job, cache it
   *  per (user, job), and return it fresh. Bound to the explicit compute/recompute
   *  action. With no LLM configured this returns `has_cv` with a null analysis. */
  async function runJobFit(slug: string): Promise<JobFitResponse> {
    return requestData<JobFitResponse>(`/api/v1/jobs/${slug}/fit`, { method: 'POST' });
  }

  /** The same-origin URL for the fit SSE stream — the fit page opens an EventSource on
   *  it (EventSource takes a URL, not our fetch wrapper; the session cookie rides along). */
  function jobFitStreamUrl(slug: string): string {
    return `${baseUrl}/api/v1/jobs/${slug}/fit/stream`;
  }

  /** Full-text search over jobs. `facets` carries the query text and any facet
   *  filters (built by the caller); pagination is appended here. Results are the
   *  same Job wire shape as listJobs, so views render them with the same
   *  components. `meta.total` is an estimate from the search engine.
   *
   *  Keyword-only by default (semantic_ratio=0): hybrid/semantic ranking scores
   *  every job by similarity, so a query like "devops" returns the whole catalogue
   *  reordered rather than the handful that match — which reads as "search is
   *  broken". Semantic stays available on the API for an explicit opt-in later. */
  async function searchJobs(facets: URLSearchParams, limit: number, offset: number): Promise<Slice<Job>> {
    const params = new URLSearchParams(facets);
    params.set('semantic_ratio', '0');
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<Job>>(`/api/v1/jobs/search?${params}`), offset);
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

  /** Personalized job recommendations for the signed-in user: open jobs ranked by
   *  semantic similarity to their uploaded CV, constrained to `facets` (the same
   *  facet filter params `searchJobs` accepts, built by the caller). An empty slice
   *  means either no usable CV vector yet (no CV uploaded, or the embedder was
   *  superseded) or that the active filter matched nothing — the page tells them
   *  apart by whether a filter is set. Authenticated. */
  async function recommendations(facets: URLSearchParams, limit: number, offset: number): Promise<Slice<Job>> {
    const params = new URLSearchParams(facets);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<Job>>(`/api/v1/me/recommendations?${params}`), offset);
  }

  /** Facet-distribution counts for the analytics page. `params` carries the same
   *  query text and facet filters as `searchJobs` (built by the caller, e.g. via
   *  `filtersToParams`); the endpoint returns counts instead of a page of jobs.
   *  Empty `facets`/`stats` are normalized to `{}` so callers never see null. */
  // `disjunctive` asks the endpoint to count each facet under the filter minus its
  // own selection (so a selected facet still shows its siblings) — the live-modal mode.
  async function facetCounts(params: URLSearchParams, opts?: { disjunctive?: boolean }): Promise<FacetCounts> {
    const p = new URLSearchParams(params);
    if (opts?.disjunctive) p.set('disjunctive', '1');
    const res = await request<{ data: FacetCounts }>(`/api/v1/jobs/facets?${p}`);
    return { total: res.data.total, facets: res.data.facets ?? {}, stats: res.data.stats ?? {} };
  }

  /** The public catalogue-activity time series: vacancies added vs. removed per
   *  period, aggregated to `granularity` (day/week/month) over an optional date
   *  range (`from`/`to`, ISO YYYY-MM-DD; the server defaults a sensible recent
   *  window per granularity). The series is dense — empty periods carry zeros —
   *  so the chart draws without gap-filling. Unauthenticated. */
  async function jobsActivity(
    granularity: ActivityGranularity,
    from?: string,
    to?: string,
  ): Promise<ActivityPoint[]> {
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

  /** Aggregate engagement counts (jobs saved / applied / viewed across all users).
   *  Aggregate-only, unauthenticated. */
  async function engagementStats(): Promise<EngagementStats> {
    return requestData<EngagementStats>(`/api/v1/stats/engagement`);
  }

  /** The public ingest-fleet status: a per-provider health rollup with a derived
   *  operational/degraded/down verdict and an overall status. Sanitized
   *  (no error text or board identifiers), aggregate-only, unauthenticated. */
  async function ingestStatus(): Promise<IngestStatus> {
    return requestData<IngestStatus>(`/api/v1/status`);
  }

  /** List companies, optionally filtered by a name query `q` (a case-insensitive
   *  substring match; an empty `q` lists everything). `meta.total` reflects the
   *  filtered count, so the Paginator pages over the matches. */
  // `facets` carries the sidebar's filter params (regions/collections/…, and `q`
  // when it lives in the filter model); the count-ordered typeahead just passes a
  // bare `q`. Both funnel through here so the endpoint stays one call site.
  async function listCompanies(
    q: string,
    limit: number,
    offset: number,
    facets?: URLSearchParams,
  ): Promise<Slice<CompanyListItem>> {
    const params = new URLSearchParams(facets);
    if (q) params.set('q', q);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<CompanyListItem>>(`/api/v1/companies?${params}`), offset);
  }

  async function getCompany(
    slug: string,
    limit: number,
    offset: number,
  ): Promise<{ company: Company; jobs: Job[] }> {
    return requestData<{ company: Company; jobs: Job[] }>(
      `/api/v1/companies/${slug}${query(limit, offset)}`,
    );
  }

  /** The subindustry facet vocabulary (each clean YC leaf + its company count),
   *  count-ordered, backing the company "Industry" filter's searchable options. */
  async function listCompanySubindustries(): Promise<{ value: string; count: number }[]> {
    return requestData<{ value: string; count: number }[]>('/api/v1/companies/subindustries');
  }

  // --- Insights (aggregate market data behind the /insights SEO pages) --------

  function insightsQuery(opts: {
    category?: string;
    country?: string;
    sort?: 'open' | 'growth';
    limit?: number;
  }): string {
    const q = new URLSearchParams();
    if (opts.category) q.set('category', opts.category);
    if (opts.country) q.set('country', opts.country);
    if (opts.sort) q.set('sort', opts.sort);
    if (opts.limit != null) q.set('limit', String(opts.limit));
    return q.toString();
  }

  /** Ranked roles (category × seniority). Pass `category` to restrict to one
   *  category's seniorities (the per-category roles page); omit for all. */
  async function insightsRoles(
    opts: { category?: string; country?: string; sort?: 'open' | 'growth'; limit?: number } = {},
  ): Promise<InsightRole[]> {
    return requestData<InsightRole[]>(`/api/v1/insights/roles?${insightsQuery(opts)}`);
  }

  /** Ranked skills, optionally scoped by category or country (not both). */
  async function insightsSkills(
    opts: { category?: string; country?: string; sort?: 'open' | 'growth'; limit?: number } = {},
  ): Promise<InsightSkill[]> {
    return requestData<InsightSkill[]>(`/api/v1/insights/skills?${insightsQuery(opts)}`);
  }

  /** Every seniority's salary bands for one category, in a single call (the
   *  per-category salary page). Rows carry their own `seniority` ('' = the
   *  category-wide band). */
  async function insightsSalaryByCategory(category: string): Promise<InsightSalaryBand[]> {
    return requestData<InsightSalaryBand[]>(
      `/api/v1/insights/salary?category=${encodeURIComponent(category)}`,
    );
  }

  // --- Sitemap --------------------------------------------------------------
  //
  // Feeds behind the sitemap index (server routes only). Jobs ship their freshest
  // slice (one file); companies are keyset-paginated across chunks, with a boundary
  // endpoint returning the cursor ending each chunk so the index can enumerate them.

  /** The freshest open-job sitemap entries (newest first), one file. */
  async function sitemapJobs(): Promise<SitemapEntry[]> {
    return requestData<SitemapEntry[]>('/api/v1/jobs/sitemap');
  }

  /** One chunk of company sitemap entries with slug > `after` ('' for the first). */
  async function sitemapCompanies(after: string, limit: number): Promise<SitemapEntry[]> {
    return requestData<SitemapEntry[]>(
      `/api/v1/companies/sitemap?after=${encodeURIComponent(after)}&limit=${limit}`,
    );
  }

  /** The slug cursor ending each `chunk`-sized page of companies. */
  async function sitemapCompanyBoundaries(chunk: number): Promise<string[]> {
    return requestData<string[]>(`/api/v1/companies/sitemap/boundaries?chunk=${chunk}`);
  }

  // --- Auth -----------------------------------------------------------------
  //
  // register/login set the httpOnly auth cookie server-side and return the user;
  // the token never reaches JS. Subsequent calls (me) are authenticated by the
  // cookie the browser attaches automatically.

  /** POST credentials and return the created/authenticated user. */
  async function postAuth(path: string, body: unknown): Promise<User> {
    return requestData<User>(path, jsonBody('POST', body));
  }

  function register(email: string, password: string): Promise<User> {
    return postAuth('/api/v1/auth/register', { email, password });
  }

  function login(email: string, password: string): Promise<User> {
    return postAuth('/api/v1/auth/login', { email, password });
  }

  /** Names of OAuth providers enabled on the server (google/github/linkedin).
   *  The dialog renders one "Continue with …" button per name; sign-in itself is
   *  a full-page redirect through /api/v1/auth/oauth/:provider/start. */
  async function oauthProviders(): Promise<string[]> {
    return requestData<string[]>('/api/v1/auth/oauth/providers');
  }

  /** Clear the session cookie server-side. */
  async function logout(): Promise<void> {
    await call('/api/v1/auth/logout', { method: 'POST' });
  }

  /** Fetch the current user using the auth cookie. Throws ApiError(401) if it is
   *  missing or rejected. */
  async function me(): Promise<User> {
    return requestData<User>('/api/v1/auth/me');
  }

  // --- Per-user job interactions --------------------------------------------
  //
  // Both require a session (the auth cookie). Callers gate on auth state before
  // invoking — the SPA never sends these for a signed-out visitor.

  /** Call a job-interaction endpoint and return the resulting record. */
  async function jobInteraction(
    slug: string,
    action: 'view' | 'apply' | 'save' | 'stage' | 'track' | 'dismiss',
    method: 'POST' | 'DELETE' = 'POST',
  ): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${slug}/${action}`, { method });
  }

  /** Record that the current user viewed a job; returns their interaction
   *  (including whether they have already applied). */
  function recordJobView(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'view');
  }

  /** Mark a job as applied for the current user. */
  function markJobApplied(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'apply');
  }

  /** Save (bookmark) a job for the current user. */
  function saveJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'save');
  }

  /** Set a job's application stage and/or notes (partial update — omit a field to
   *  leave it unchanged). Returns the updated interaction. */
  async function trackJob(slug: string, patch: { stage?: string; notes?: string }): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${slug}/track`, jsonBody('PATCH', patch));
  }

  /** Clear a job's saved mark. Idempotent: "already not saved" is success. */
  function unsaveJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'save', 'DELETE');
  }

  /** Dismiss (swipe away) a job in the swipe deck. Keeps it out of the deck only;
   *  the job stays visible in the normal list and search. */
  function dismissJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'dismiss');
  }

  /** Clear a job's dismissed mark (swipe-mode undo). Idempotent: "already not
   *  dismissed" is success. */
  function undismissJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'dismiss', 'DELETE');
  }

  /** Drop a job's pipeline progress (stage + applied), keeping its saved mark —
   *  the board's backward "move to Saved" drag. */
  function clearJobStage(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'stage', 'DELETE');
  }

  /** Remove a job from the board entirely (it stays in view history). */
  function untrackJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'track', 'DELETE');
  }

  /** The current user's job interactions, newest activity first. Alongside the
   *  page, the response carries the per-tab counts for the my-jobs tab badges. */
  async function listMyJobs(
    filter: MyJobsFilter,
    limit: number,
    offset: number,
  ): Promise<Slice<MyJob> & { counts: MyJobCounts }> {
    const res = await request<{ data: MyJob[]; meta: ListMeta & { counts: MyJobCounts } }>(
      `/api/v1/me/tracking${query(limit, offset)}&filter=${filter}`,
    );
    return { ...toSlice(res, offset), counts: res.meta.counts };
  }

  /** The current user's application-pipeline snapshot: per-bucket application
   *  counts aggregated server-side, for the Pipeline tab's Sankey and rate cards. */
  async function getMyPipeline(): Promise<PipelineStats> {
    return requestData<PipelineStats>('/api/v1/me/tracking/pipeline');
  }

  /** The jobs the current user has run the AI fit analysis on (newest first), plus their
   *  monthly quota — powers the Tracking → AI fit tab. Never triggers the LLM. */
  async function myAnalyses(): Promise<{ items: MyAnalysisItem[]; quota: JobFitQuota }> {
    const res = await request<{ data: MyAnalysisItem[]; meta: { quota: JobFitQuota } }>(
      '/api/v1/me/tracking/analyses',
    );
    return { items: res.data, quota: res.meta.quota };
  }

  /** The public slugs of every job the current user has interacted with. The
   *  browse UI cross-references this set to dim already-viewed cards without
   *  authenticating the public job list. */
  async function listViewedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/viewed');
  }

  /** The public slugs of every job the current user has saved (bookmarked). The
   *  browse UI cross-references this set to render the save toggle as filled on
   *  already-saved cards without authenticating the public job list. */
  async function listSavedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/saved');
  }

  // --- API keys -------------------------------------------------------------
  //
  // Personal API keys for non-browser access. Management is cookie-only (these
  // calls ride the session cookie); the plaintext token is returned once, by
  // createApiKey, and never again.

  /** The current user's API keys (metadata only — no secret). */
  async function listApiKeys(): Promise<ApiKey[]> {
    return requestData<ApiKey[]>('/api/v1/me/api-keys');
  }

  /** Create a key and return it with its one-time plaintext `token`. `expiresAt` is
   *  an RFC3339 string, or omitted for a key that never expires. */
  async function createApiKey(name: string, expiresAt?: string): Promise<CreatedApiKey> {
    return requestData<CreatedApiKey>('/api/v1/me/api-keys', jsonBody('POST', { name, expires_at: expiresAt ?? null }));
  }

  /** Revoke a key by id; it stops authenticating immediately. */
  async function revokeApiKey(id: number): Promise<void> {
    await call(`/api/v1/me/api-keys/${id}`, { method: 'DELETE' });
  }

  // Saved searches: named snapshots of the filter state (cookie-only on the server).

  /** The current user's saved searches, most recently updated first. */
  async function listSavedSearches(): Promise<SavedSearch[]> {
    return requestData<SavedSearch[]>('/api/v1/me/searches');
  }

  /** Save the current filter state under a name. `query` is the canonical search
   *  query string (may be empty). A duplicate name or the per-user cap is a 409. */
  async function createSavedSearch(name: string, query: string): Promise<SavedSearch> {
    return requestData<SavedSearch>('/api/v1/me/searches', jsonBody('POST', { name, query }));
  }

  /** Overwrite a saved search's name and/or query; an omitted field is unchanged. */
  async function updateSavedSearch(
    id: number,
    patch: { name?: string; query?: string },
  ): Promise<SavedSearch> {
    return requestData<SavedSearch>(`/api/v1/me/searches/${id}`, jsonBody('PATCH', patch));
  }

  /** Delete a saved search by id. */
  async function deleteSavedSearch(id: number): Promise<void> {
    await call(`/api/v1/me/searches/${id}`, { method: 'DELETE' });
  }

  /** Publish a saved search as a public board (cookie-only). Returns the updated set,
   *  now carrying `public_slug`. An optional `authorLabel` is shown on the board; blank
   *  renders it anonymously. Re-sharing keeps the existing slug. */
  async function shareSavedSearch(id: number, authorLabel = ''): Promise<SavedSearch> {
    return requestData<SavedSearch>(`/api/v1/me/searches/${id}/share`, jsonBody('POST', { author_label: authorLabel }));
  }

  /** Make a shared board private again (cookie-only). Idempotent. */
  async function unshareSavedSearch(id: number): Promise<void> {
    await call(`/api/v1/me/searches/${id}/share`, { method: 'DELETE' });
  }

  /** Public read of a shared board by slug — unauthenticated. Returns only display
   *  fields (name, query, author_label). An unknown/unshared slug throws (404). */
  async function getBoard(slug: string): Promise<Board> {
    return requestData<Board>(`/api/v1/boards/${encodeURIComponent(slug)}`);
  }

  // The single per-user profile: a specialization + skills set (cookie-only on the server).

  /** The current user's profile, or null when they have not saved one yet. */
  async function getProfile(): Promise<UserProfile | null> {
    return requestData<UserProfile | null>('/api/v1/me/profile');
  }

  /** Create-or-replace the user's profile from a non-empty set of specializations (job
   *  categories), a non-empty set of skills, and an optional location-preferences block
   *  (null clears it). A bad specialization, empty skills, or an out-of-vocabulary location
   *  value is a 400. */
  async function saveProfile(
    specializations: string[],
    skills: string[],
    location: LocationPreferences | null,
  ): Promise<UserProfile> {
    return requestData<UserProfile>(
      '/api/v1/me/profile',
      jsonBody('PUT', { specializations, skills, location_preferences: location }),
    );
  }

  /** Clear the user's profile. Idempotent. */
  async function deleteProfile(): Promise<void> {
    await call('/api/v1/me/profile', { method: 'DELETE' });
  }

  /** Build the request init for a résumé payload: pasted text goes as JSON, a `File`
   *  goes as multipart. */
  function resumeInit(method: string, input: File | string): RequestInit {
    if (typeof input === 'string') {
      return {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: input }),
      };
    }
    const form = new FormData();
    form.append('file', input);
    return { method, body: form };
  }

  /** Derive a résumé's profile — a PDF `File` (sent as multipart) or pasted text (sent
   *  as JSON) — via the deterministic dictionaries: canonical skills plus the
   *  specializations and seniority resolved, ready to pre-fill a profile or the onboarding
   *  wizard. */
  async function extractResumeProfile(input: File | string): Promise<ResumeProfile> {
    return requestData<ResumeProfile>(
      '/api/v1/me/resume/extract',
      resumeInit('POST', input),
    );
  }

  /** The caller's résumé status, including the read-only structured résumé parsed from
   *  the CV (null when none is current). Session-scoped; always 200 (an absent/disabled
   *  résumé is a normal state the profile renders). */
  async function getResume(): Promise<ResumeMeta> {
    return requestData<ResumeMeta>('/api/v1/me/resume');
  }

  /** The market-coverage verdict for the caller's profile: how many open vacancies the
   *  profile's skills reach for the selected role, and which missing skill unlocks the
   *  most new ones. `params` carries the same facet filters as job search, so the caller
   *  can recompute for an ad-hoc role; absent a `category` the server defaults to the
   *  profile's specializations. Session-scoped (404 when no profile). */
  async function getProfileVerdict(params?: URLSearchParams): Promise<Verdict> {
    const qs = params?.toString();
    return requestData<Verdict>(
      `/api/v1/me/profile/verdict${qs ? `?${qs}` : ''}`,
    );
  }

  /** The CV ATS-readiness report for the caller's profile: structural checks over the
   *  stored CV plus a keyword-match against the selected role's top skills. `params`
   *  carries the same facet filters as the verdict. `has_cv` is false (report null)
   *  when no CV is stored — the page then prompts an upload. Session-scoped. */
  async function getATSReport(params?: URLSearchParams): Promise<ATSResponse> {
    const qs = params?.toString();
    return requestData<ATSResponse>(
      `/api/v1/me/profile/ats-report${qs ? `?${qs}` : ''}`,
    );
  }

  /** Run the optional LLM qualitative review over the caller's stored CV; returns the
   *  ATS report with content-quality + findings folded in (cached server-side). With no
   *  LLM configured this is just the deterministic report. */
  async function runATSReview(params?: URLSearchParams): Promise<ATSResponse> {
    const qs = params?.toString();
    return requestData<ATSResponse>(
      `/api/v1/me/profile/ats-report${qs ? `?${qs}` : ''}`,
      { method: 'POST' },
    );
  }

  /** The caller's notification subscriptions (one per saved search + channel). */
  async function listSubscriptions(): Promise<Subscription[]> {
    return requestData<Subscription[]>('/api/v1/me/subscriptions');
  }

  /** Subscribe a saved search to a channel (telegram by default). A duplicate is a 409. */
  async function createSubscription(savedSearchId: number, channel = 'telegram'): Promise<Subscription> {
    return requestData<Subscription>('/api/v1/me/subscriptions', jsonBody('POST', { saved_search_id: savedSearchId, channel }));
  }

  /** Pause or resume a subscription. */
  async function setSubscriptionActive(id: number, active: boolean): Promise<Subscription> {
    return requestData<Subscription>(`/api/v1/me/subscriptions/${id}`, jsonBody('PATCH', { active }));
  }

  /** Unsubscribe by subscription id. */
  async function deleteSubscription(id: number): Promise<void> {
    await call(`/api/v1/me/subscriptions/${id}`, { method: 'DELETE' });
  }

  /** Whether Telegram notifications are configured and whether this user is linked. */
  async function telegramStatus(): Promise<TelegramStatus> {
    return requestData<TelegramStatus>('/api/v1/me/telegram');
  }

  /** Mint a one-time deep link the user opens to connect their Telegram chat. */
  async function telegramLink(): Promise<string> {
    const res = await request<{ data: { url: string } }>('/api/v1/me/telegram/link', { method: 'POST' });
    return res.data.url;
  }

  /** Disconnect the user's Telegram chat. */
  async function telegramUnlink(): Promise<void> {
    await call('/api/v1/me/telegram', { method: 'DELETE' });
  }

  /** Submit a vacancy for moderation. Returns the pending submission. */
  async function submitJob(input: SubmissionInput): Promise<Submission> {
    return requestData<Submission>('/api/v1/submissions', jsonBody('POST', input));
  }

  /** The caller's own submissions with their review status. */
  async function listMySubmissions(): Promise<Submission[]> {
    return requestData<Submission[]>('/api/v1/me/submissions');
  }

  /** Contribute a job link. The server detects the ATS, dedups by derived identity, and
   *  records + rewards a novel link (422 unsupported ATS, 409 already held / contributed). */
  async function submitContribution(url: string): Promise<Contribution> {
    return requestData<Contribution>('/api/v1/me/contributions', jsonBody('POST', { url }));
  }

  /** The caller's own link contributions, newest first. */
  async function listMyContributions(): Promise<Contribution[]> {
    return requestData<Contribution[]>('/api/v1/me/contributions');
  }

  /** The moderator review queue: pending submissions, with submitter emails. */
  async function listPendingSubmissions(): Promise<Submission[]> {
    return requestData<Submission[]>('/api/v1/submissions');
  }

  /** Approve a pending submission; the server mints a live job from it. */
  async function approveSubmission(id: number): Promise<Submission> {
    return requestData<Submission>(`/api/v1/submissions/${id}/approve`, {
      method: 'POST',
    });
  }

  /** Reject a pending submission with an optional reason. */
  async function rejectSubmission(id: number, reason?: string): Promise<Submission> {
    return requestData<Submission>(`/api/v1/submissions/${id}/reject`, jsonBody('POST', { reason: reason ?? '' }));
  }

  /** Report a problem with a live vacancy (by slug). Returns the pending report. */
  async function reportJob(slug: string, input: ReportInput): Promise<Report> {
    return requestData<Report>(`/api/v1/jobs/${slug}/reports`, jsonBody('POST', input));
  }

  /** The moderator review queue: pending reports, with reporter email and job fields. */
  async function listPendingReports(): Promise<Report[]> {
    return requestData<Report[]>('/api/v1/reports');
  }

  /** Resolve a pending report; optionally soft-close the reported job. */
  async function resolveReport(id: number, closeJob: boolean): Promise<Report> {
    return requestData<Report>(`/api/v1/reports/${id}/resolve`, jsonBody('POST', { close_job: closeJob }));
  }

  /** Dismiss a pending report with an optional reason; the job is unchanged. */
  async function dismissReport(id: number, reason?: string): Promise<Report> {
    return requestData<Report>(`/api/v1/reports/${id}/dismiss`, jsonBody('POST', { reason: reason ?? '' }));
  }

  // --- Gmail inbox ---------------------------------------------------------

  /** Whether the caller has connected Gmail for the ATS inbox. */
  async function gmailStatus(): Promise<GmailStatus> {
    return requestData<GmailStatus>('/api/v1/me/gmail');
  }

  /** Disconnect Gmail: revoke the grant and purge synced mail. */
  async function disconnectGmail(): Promise<void> {
    await requestData<unknown>('/api/v1/me/gmail', { method: 'DELETE' });
  }

  /** Trigger an on-demand sync of the caller's ATS mail (runs in the background). */
  async function syncGmail(): Promise<void> {
    await requestData<unknown>('/api/v1/me/gmail/sync', { method: 'POST' });
  }

  /** A page of the flat inbox listing, newest first, with the total message count.
   *  Optional search term filters by subject, sender, or body; optional source
   *  narrows to one account (the switcher). */
  async function getInbox(
    q = '',
    limit = 20,
    offset = 0,
    source: InboxSource = '',
    unread = false,
    status = '',
  ): Promise<{ messages: InboxMessage[]; total: number }> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (q) params.set('q', q);
    if (source) params.set('source', source);
    if (unread) params.set('unread', '1');
    if (status) params.set('status', status);
    const res = await request<{ data: InboxMessage[]; meta: { total: number } }>(
      `/api/v1/me/inbox?${params.toString()}`,
    );
    return { messages: res.data, total: res.meta.total };
  }

  /** Mark every unread message matching the active filters as read; returns the
   *  count marked. The unread flag is implicit server-side, so it is not sent. */
  async function markAllRead(source: InboxSource = '', status = '', q = ''): Promise<number> {
    const params = new URLSearchParams();
    if (source) params.set('source', source);
    if (status) params.set('status', status);
    if (q) params.set('q', q);
    const res = await requestData<{ marked: number }>(
      `/api/v1/me/inbox/read-all?${params.toString()}`,
      { method: 'POST' },
    );
    return res.marked;
  }

  /** Soft-delete a message (hidden from the inbox, restorable via restoreEmail). */
  async function deleteEmail(id: number): Promise<void> {
    await call(`/api/v1/me/emails/${id}/delete`, { method: 'POST' });
  }

  /** Undo a soft-delete, bringing the message back into the inbox. */
  async function restoreEmail(id: number): Promise<void> {
    await call(`/api/v1/me/emails/${id}/restore`, { method: 'POST' });
  }

  /** The caller's hosted-mailbox address (or null) + feature availability. */
  async function mailboxStatus(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox');
  }

  /** Claim (or return) the caller's hosted mailbox address. */
  async function claimMailbox(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox', { method: 'POST' });
  }

  /** Release the hosted mailbox: drop the address and purge its received mail. */
  async function releaseMailbox(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox', { method: 'DELETE' });
  }

  /** One message's full body. */
  async function getEmail(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}`);
  }

  /** The caller's application for a job slug, with its linked emails. */
  async function getTrackedApplication(slug: string): Promise<TrackedApplication> {
    return requestData<TrackedApplication>(`/api/v1/me/tracking/${encodeURIComponent(slug)}`);
  }

  /** Promote an email's pending suggestion to a confirmed link. */
  async function confirmEmailLink(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/confirm`, { method: 'POST' });
  }

  /** Dismiss an email's pending suggestion without linking. */
  async function rejectEmailLink(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/reject`, { method: 'POST' });
  }

  /** Manually link an email to the application named by slug. */
  async function linkEmail(id: number, slug: string): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/link`, jsonBody('POST', { slug }));
  }

  /** Clear an email's application link. */
  async function unlinkEmail(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/unlink`, { method: 'POST' });
  }

  // --- CV builder (beta-gated on the server) ---

  /** List the caller's CVs (metadata, newest edit first). */
  async function listCvs(): Promise<CvMeta[]> {
    return requestData<CvMeta[]>('/api/v1/me/cvs');
  }

  /** Create a CV, optionally seeded from the stored résumé structure. */
  async function createCv(input: CreateCvInput = {}): Promise<CvRecord> {
    return requestData<CvRecord>('/api/v1/me/cvs', jsonBody('POST', input));
  }

  /** Fetch one CV with its full document. */
  async function getCv(id: number): Promise<CvRecord> {
    return requestData<CvRecord>(`/api/v1/me/cvs/${id}`);
  }

  /** Replace a CV's title, template, and document. */
  async function updateCv(id: number, input: UpdateCvInput): Promise<CvMeta> {
    return requestData<CvMeta>(`/api/v1/me/cvs/${id}`, jsonBody('PUT', input));
  }

  /** Delete a CV. */
  async function deleteCv(id: number): Promise<void> {
    await call(`/api/v1/me/cvs/${id}`, { method: 'DELETE' });
  }

  /** The authenticated PDF URL for a CV (same-origin cookie rides along on download). */
  function cvPdfUrl(id: number): string {
    return `${baseUrl}/api/v1/me/cvs/${id}/pdf`;
  }

  return {
    listJobs,
    getJob,
    getSimilarJobs,
    getJobCopies,
    getJobMatch,
    getJobFit,
    runJobFit,
    jobFitStreamUrl,
    searchJobs,
    swipeDeck,
    recommendations,
    facetCounts,
    jobsActivity,
    userGrowth,
    engagementStats,
    ingestStatus,
    listCompanies,
    getCompany,
    listCompanySubindustries,
    insightsRoles,
    insightsSkills,
    insightsSalaryByCategory,
    sitemapJobs,
    sitemapCompanies,
    sitemapCompanyBoundaries,
    register,
    login,
    oauthProviders,
    logout,
    me,
    recordJobView,
    markJobApplied,
    saveJob,
    unsaveJob,
    dismissJob,
    undismissJob,
    clearJobStage,
    untrackJob,
    trackJob,
    listMyJobs,
    getMyPipeline,
    myAnalyses,
    listViewedSlugs,
    listSavedSlugs,
    listApiKeys,
    createApiKey,
    revokeApiKey,
    listSavedSearches,
    createSavedSearch,
    updateSavedSearch,
    deleteSavedSearch,
    shareSavedSearch,
    unshareSavedSearch,
    getBoard,
    getProfile,
    saveProfile,
    deleteProfile,
    extractResumeProfile,
    getResume,
    getProfileVerdict,
    getATSReport,
    runATSReview,
    listSubscriptions,
    createSubscription,
    setSubscriptionActive,
    deleteSubscription,
    telegramStatus,
    telegramLink,
    telegramUnlink,
    submitJob,
    listMySubmissions,
    submitContribution,
    listMyContributions,
    listPendingSubmissions,
    approveSubmission,
    rejectSubmission,
    reportJob,
    listPendingReports,
    resolveReport,
    dismissReport,
    gmailStatus,
    disconnectGmail,
    syncGmail,
    mailboxStatus,
    claimMailbox,
    releaseMailbox,
    getInbox,
    getEmail,
    markAllRead,
    deleteEmail,
    restoreEmail,
    getTrackedApplication,
    confirmEmailLink,
    rejectEmailLink,
    linkEmail,
    unlinkEmail,
    listCvs,
    createCv,
    getCv,
    updateCv,
    deleteCv,
    cvPdfUrl,
  };
}

export type MyJobsFilter = 'all' | 'viewed' | 'saved' | 'applied' | 'board';

/** The default browser client: global fetch, same-origin, cookie attached. Client
 *  components call methods on it (`api.foo()`); server `load` uses `serverApi(event.fetch)`. */
export const api = createApi();
