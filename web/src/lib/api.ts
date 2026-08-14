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

import type { Answers, Display, RevisionView } from '$lib/generated/contracts';
import type {
  CvAtsDelta,
  CvJobMatch,
  CvMeta,
  CvRecord,
  CvTailoredItem,
  CvTemplate,
  CvFont,
  CvTracerLink,
  UpdateCvInput,
  TailorResult,
  JdResolveInput,
} from './cv';
import type {
  Job,
  EmailLinking,
  TrackedApplication,
  MailRecallResult,
  FollowUpDraft,
  Company,
  CompanyListItem,
  FacetCounts,
  ListMeta,
  MyJob,
  MyJobCounts,
  PipelineStats,
  ScheduledInterview,
  TimelineEvent,
  User,
  UserJob,
  VoteResult,
  ApiKey,
  CreatedApiKey,
  ConnectedIdentities,
  SavedSearch,
  Board,
  UserProfile,
  Subscription,
  TelegramStatus,
  DiscordStatus,
  DiscordLinkResult,
  Submission,
  SubmissionInput,
  PrefillResult,
  Contribution,
  ResolvedLink,
  ReferralOffer,
  ReferralRequestInput,
  SeekerReferralRequest,
  IncomingReferralRequest,
  Report,
  ReportInput,
  GhostReportInput,
  Verdict,
  ATSResponse,
  JobMatchResult,
  MatchAnalysisResponse,
  AiCredits,
  AiUsage,
  CreditHistoryEntry,
  MyAnalysisItem,
  ResumeProfile,
  PhotoMeta,
  ResumeMeta,
  CandidateContacts,
  ActivityGranularity,
  ActivityPoint,
  UserGrowthPoint,
  EngagementStats,
  IngestStatus,
  LocationPreferences,
  NotificationSettings,
  NotificationItem,
  CommunityThread,
  CommunityReply,
  CompanyFeedback,
  CompanyFeedbackSummary,
  ReportedCompanyFeedback,
  ExperienceAtom,
  ExperienceBank,
  TalentNetworkSetting,
  TalentNetworkVisibility,
  TalentNetworkProfile,
  ExperienceEmployment,
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
export interface InsightCompany {
  company_slug: string;
  company_name: string;
  open_now: number;
  open_prev_30d: number;
  growth_30d: number;
}

/** One weekly snapshot in a skill's demand series (GET /me/market-pulse). */
export interface SkillPulsePoint {
  week_start: string;
  open_count: number;
}
/** A signed-in user's own profile skill, joined against its retained weekly
 *  history. change_pct is null with fewer than two snapshots, or when the
 *  earliest snapshot's count is zero. Skills never yet seen in an open job are
 *  omitted by the server rather than reported with a fabricated count. */
export interface SkillPulse {
  skill: string;
  open_count: number;
  change_pct: number | null;
  series: SkillPulsePoint[];
}

/** One posting in a role cluster — a single city's opening under a collapsed role
 *  (see the /jobs/:slug/copies endpoint). Each keeps its own location and apply URL. */
export interface JobCopy {
  public_slug: string;
  location: string;
  apply_url: string;
  posted_at: string | null;
}

/** Max résumé upload size, mirroring the server's BodyLimit (cmd/server/main.go). The
 *  web client enforces it up front so an oversize PDF gets a clear message instead of the
 *  raw 413 the server would emit before the handler runs. The UI also shows it as a hint. */
export const RESUME_MAX_MB = 8;
const RESUME_MAX_BYTES = RESUME_MAX_MB * 1024 * 1024;

/** Max headshot upload size. Deliberately UNDER the server's 8 MiB BodyLimit rather than
 *  equal to it: the limit covers the whole multipart request, so a file at exactly 8 MiB is
 *  refused by fasthttp with a bare 413 before any handler — and the member sees an unstyled
 *  error instead of this message. */
export const PHOTO_MAX_MB = 6;
const PHOTO_MAX_BYTES = PHOTO_MAX_MB * 1024 * 1024;

/** The image types the server can decode (internal/headshot). Used as the file input's
 *  `accept` list and as the pre-flight check. */
export const PHOTO_ACCEPT = 'image/jpeg,image/png,image/webp';

/** A non-2xx API response. Carries the HTTP status so callers can branch on it
 *  (e.g. 401 invalid credentials, 409 email taken) instead of parsing strings.
 *  `message` is the backend's `{ "error": msg }` text when present, so logs and
 *  any raw-error surface read the real reason rather than a bare status line. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    /** The parsed JSON error body, when present — so callers can read extra fields a
     *  specific endpoint attaches to a failure (e.g. the already-tracked company). */
    public readonly body?: Record<string, unknown>,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/** Parse a failed response into an ApiError. The backend's standard error envelope is
 *  `{ "error": msg }`; surface that as the message (falling back to the status line for a
 *  non-JSON error, e.g. a proxy 502) and keep the whole parsed body for callers that need
 *  extra fields. Reads the body once. */
async function toApiError(res: Response): Promise<ApiError> {
  try {
    const body = await res.json();
    const msg = body && typeof body.error === 'string' ? body.error : `${res.status} ${res.statusText}`;
    return new ApiError(res.status, msg, body ?? undefined);
  } catch {
    return new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
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

// --- Gmail inbox wire shapes ---------------------------------------------

export interface GmailStatus {
  connected: boolean;
  available?: boolean; // whether the connect flow is configured server-side
  email?: string;
  status?: string;
  /** Whether the same Google grant also covers the calendar. Read from the recorded
   *  scopes rather than from the connection existing: the two consents are separate, so
   *  a connected mailbox says nothing about the calendar, and a calendar grant may have
   *  no mailbox behind it at all. */
  calendar_connected?: boolean;
}

/** The hosted-mailbox option: the caller's address (null when none) + whether
 *  the feature is configured server-side. */
export interface MailboxStatus {
  available: boolean;
  address: string | null;
}

/** The account switcher value: '' = all sources. 'external' is mail the caller's
 *  own agent harness pushed — it has no connection to configure, it simply
 *  arrives. */
export type InboxSource = '' | 'gmail' | 'hosted' | 'external';

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

/** Build an API client bound to a specific fetch and base URL.
 *
 *  - Browser: the default `api` uses global fetch and an empty base, so requests
 *    are relative and same-origin (the auth cookie rides along; see SPA-era note).
 *  - SvelteKit server `load`: pass `event.fetch` and the internal API origin
 *    (`serverApi`), because a server-side relative `/api` would hit the Node app
 *    itself, not nginx→Go. `baseUrl` resolves that to a real server-to-server call. */
export function createApi(
  fetchImpl: typeof fetch = fetch,
  baseUrl = '',
  defaultHeaders: Record<string, string> = {},
  /** Abort a call that has not answered within this many milliseconds. Set by the
   *  SERVER client only (see `$lib/server/api`): a `load` that never finishes holds
   *  its socket open forever, and enough of those fill the accept queue and take the
   *  whole SSR process down. The browser client leaves this unset — it drives the
   *  LLM-backed calls, which legitimately run for minutes. */
  timeoutMs?: number,
) {
  /** The single place this module touches fetch. Always sends credentials so the
   *  auth cookie rides along, and turns a non-2xx into an ApiError. `defaultHeaders`
   *  lets a server caller forward the request's Cookie to an absolute API URL
   *  (where `event.fetch` would not). */
  async function call(path: string, init?: RequestInit): Promise<Response> {
    const request: RequestInit = {
      credentials: 'include',
      ...init,
      headers: { ...defaultHeaders, ...init?.headers },
    };
    // Assigned after the spread, so a timeout is added to the init rather than
    // substituted for it. A caller that brought its own signal keeps it.
    const timeout = timeoutMs != null && request.signal == null ? AbortSignal.timeout(timeoutMs) : undefined;
    if (timeout) {
      request.signal = timeout;
    }

    let res: Response;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, request);
    } catch (err) {
      // Report the timeout as the gateway timeout it is, so a `load` renders the
      // error page for a slow backend instead of surfacing a bare DOMException.
      if (timeout?.aborted) {
        throw new ApiError(504, `The API did not answer within ${timeoutMs}ms: ${path}`);
      }
      throw err;
    }
    if (!res.ok) {
      throw await toApiError(res);
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

  /** The questions this job's application will ask, as the ATS published them. 404s for
   *  the majority of postings — a form can only be read from a few platforms — so callers
   *  treat a failure as "not known" rather than as an error worth surfacing. */
  async function getApplyForm(slug: string): Promise<Display> {
    return requestData<Display>(`/api/v1/jobs/${slug}/apply-form`);
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

  /** The cached LLM fit analysis for a job (never runs the model). `has_cv` is false
   *  when no CV is stored; `analysis` is null when none is cached yet; `stale` marks a
   *  cached analysis whose CV or job changed since. Safe to call on expand. */
  async function getMatchAnalysis(slug: string): Promise<MatchAnalysisResponse> {
    return requestData<MatchAnalysisResponse>(`/api/v1/jobs/${slug}/match-analysis`);
  }

  /** Run the three-stage fit prompt-chain over the caller's CV and this job, cache it
   *  per (user, job), and return it fresh. Bound to the explicit compute/recompute
   *  action. With no LLM configured this returns `has_cv` with a null analysis. */
  async function runMatchAnalysis(slug: string): Promise<MatchAnalysisResponse> {
    return requestData<MatchAnalysisResponse>(`/api/v1/jobs/${slug}/match-analysis`, { method: 'POST' });
  }

  /** The same-origin URL for the fit SSE stream — the fit page opens an EventSource on
   *  it (EventSource takes a URL, not our fetch wrapper; the session cookie rides along). */
  function matchAnalysisStreamUrl(slug: string): string {
    return `${baseUrl}/api/v1/jobs/${slug}/match-analysis/stream`;
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
  ): Promise<{ company: Company; jobs: Job[]; referral_available: boolean }> {
    return requestData<{ company: Company; jobs: Job[]; referral_available: boolean }>(
      `/api/v1/companies/${slug}${query(limit, offset)}`,
    );
  }

  /** The subindustry facet vocabulary (each clean YC leaf + its company count),
   *  count-ordered, backing the company "Industry" filter's searchable options. */
  async function listCompanySubindustries(): Promise<{ value: string; count: number }[]> {
    return requestData<{ value: string; count: number }[]>('/api/v1/companies/subindustries');
  }

  /** Population-ranked city-name search over the embedded GeoNames dictionary,
   *  backing the profile's base-city and relocation-cities autocomplete. `country`
   *  narrows to one ISO 3166-1 alpha-2 code; each result carries its own raw code
   *  too (unrelated cities can share a name), not a pre-composed label. */
  async function searchCities(q: string, country?: string): Promise<{ value: string; country: string }[]> {
    const params = new URLSearchParams({ q });
    if (country) params.set('country', country);
    return requestData<{ value: string; country: string }[]>(`/api/v1/geo/cities?${params}`);
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

  /** Company hiring-signal leaderboard: companies ranked by 30-day open-job growth
   *  (`growth` ramping, `-growth` freezing) or `open` size. `minOpen` floors the
   *  current open-count to blunt ingest-artifact spikes. */
  async function insightsCompanies(
    opts: { sort?: 'growth' | '-growth' | 'open'; minOpen?: number; limit?: number } = {},
  ): Promise<InsightCompany[]> {
    const q = new URLSearchParams();
    if (opts.sort) q.set('sort', opts.sort);
    if (opts.minOpen != null) q.set('min_open', String(opts.minOpen));
    if (opts.limit != null) q.set('limit', String(opts.limit));
    return requestData<InsightCompany[]>(`/api/v1/insights/companies?${q.toString()}`);
  }

  /** The signed-in caller's own profile skills, joined against their retained
   *  weekly demand history (/my/market-pulse). Cookie-only, unlike the public
   *  insights* reads above. */
  async function marketPulse(): Promise<SkillPulse[]> {
    return requestData<SkillPulse[]>('/api/v1/me/market-pulse');
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

  /** Registers with the browser's detected IANA timezone attached, so the account
   *  starts with one set rather than waiting for a profile-page visit. The server
   *  silently drops an unrecognized value rather than failing the signup, so a
   *  detection quirk here is harmless. */
  function register(email: string, password: string): Promise<User> {
    let timezone: string | undefined;
    try {
      timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      timezone = undefined;
    }
    return postAuth('/api/v1/auth/register', { email, password, timezone });
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

  // --- Account recovery -----------------------------------------------------
  //
  // Email verification and password recovery are code-based: the server mails a
  // six-digit code and these endpoints exchange it. The code is never in a URL,
  // so nothing here takes one from the query string.

  /** Ask the server to mail a fresh email-verification code to the signed-in
   *  user's address. 429 while a code issued in the last minute is outstanding. */
  async function requestEmailVerification(): Promise<void> {
    await call('/api/v1/auth/verify/request', { method: 'POST' });
  }

  /** Confirm the signed-in user's address with the mailed code; returns the
   *  updated user (email_verified true). */
  function confirmEmailVerification(code: string): Promise<User> {
    return requestData<User>('/api/v1/auth/verify/confirm', jsonBody('POST', { code }));
  }

  /** Ask for a password-reset code. Always succeeds, whether or not the address
   *  has an account — the response deliberately says nothing either way. */
  async function forgotPassword(email: string): Promise<void> {
    await call('/api/v1/auth/password/forgot', jsonBody('POST', { email }));
  }

  /** Set a new password with a mailed reset code. Every existing session is
   *  revoked, so the caller signs in again afterwards. */
  async function resetPassword(email: string, code: string, password: string): Promise<void> {
    await call('/api/v1/auth/password/reset', jsonBody('POST', { email, code, password }));
  }

  /** Change a known password. Other sessions are revoked; this one is re-issued. */
  async function changePassword(currentPassword: string, password: string): Promise<void> {
    await call('/api/v1/me/password', jsonBody('POST', { current_password: currentPassword, password }));
  }

  async function reauthenticatePassword(password: string): Promise<string> {
    const result = await requestData<{ recent_auth_expires_at: string }>(
      '/api/v2/auth/reauth/password', jsonBody('POST', { password }),
    );
    return result.recent_auth_expires_at;
  }

  async function exchangeOAuthReauthentication(code: string, codeVerifier: string): Promise<string> {
    const result = await requestData<{ recent_auth_expires_at: string }>(
      '/api/v2/auth/oauth/exchange', jsonBody('POST', { code, code_verifier: codeVerifier }),
    );
    return result.recent_auth_expires_at;
  }

  function connectedIdentities(): Promise<ConnectedIdentities> {
    return requestData<ConnectedIdentities>('/api/v2/auth/identities');
  }

  /** Sign out everywhere: revokes every session for the account, including this one. */
  async function logoutEverywhere(): Promise<void> {
    await call('/api/v1/auth/logout-all', { method: 'POST' });
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

  /** Save (bookmark) a job for the current user. Whether a reminder is scheduled
   *  is decided entirely by the account's notification settings — there is no
   *  per-job override. */
  function saveJob(slug: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${slug}/save`, { method: 'POST' });
  }

  /** Set a job's application stage and/or notes (partial update — omit a field to
   *  leave it unchanged). Returns the updated interaction. */
  async function trackJob(slug: string, patch: { stage?: string; notes?: string }): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${slug}/track`, jsonBody('PATCH', patch));
  }

  /** Set an application's stage and/or notes, addressing it by the row `id` the tracking
   *  listing served rather than by a posting slug.
   *
   *  The board holds row ids, and an application whose posting the catalogue has removed
   *  has no slug to hold — `trackJob` cannot move it. Same body and same rules,
   *  including the partial update. */
  function trackApplication(id: string, patch: { stage?: string; notes?: string }): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}`, jsonBody('PATCH', patch));
  }

  /** Drop an application's pipeline progress, keeping its saved mark — the board's
   *  backward "move to Saved" drag, addressed by the listing's row id. */
  function clearApplicationStage(id: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}/stage`, { method: 'DELETE' });
  }

  /** Remove an application from the board entirely, addressed by the listing's row id. */
  function untrackApplication(id: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}`, { method: 'DELETE' });
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

  /** Cast a thumbs vote on a job (toggle/flip). Returns the job's resulting public
   *  counters and the caller's own vote. Requires a session — gate on auth first. */
  function voteJob(slug: string, dir: 'up' | 'down'): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/jobs/${slug}/vote`, jsonBody('POST', { vote: dir }));
  }

  /** Clear the caller's thumbs vote on a job. Idempotent (no-op when none). */
  function clearJobVote(slug: string): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/jobs/${slug}/vote`, { method: 'DELETE' });
  }

  /** Cast a thumbs vote on a company (toggle/flip). Requires a session. */
  function voteCompany(slug: string, dir: 'up' | 'down'): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/companies/${slug}/vote`, jsonBody('POST', { vote: dir }));
  }

  /** Clear the caller's thumbs vote on a company. Idempotent (no-op when none). */
  function clearCompanyVote(slug: string): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/companies/${slug}/vote`, { method: 'DELETE' });
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

  /** What happened to the caller's applications between two instants, oldest first —
   *  the application-event ledger behind the Tracking → Calendar view.
   *
   *  Both bounds are RFC3339 instants and the server does not group them into days: which
   *  day an event falls on depends on the reader's clock, so calendarModel does that here.
   *  Bounds are percent-encoded because a bare `+` in an offset decodes as a space. */
  async function myTimeline(from: string, to: string): Promise<TimelineEvent[]> {
    return requestData<TimelineEvent[]>(
      `/api/v1/me/timeline?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    );
  }

  /** The interviews arranged for the caller's applications whose start falls in the
   *  range — the calendar's second layer, beside what myTimeline reports as having
   *  happened. Cancelled meetings come back marked rather than withheld. */
  async function myInterviews(from: string, to: string): Promise<ScheduledInterview[]> {
    return requestData<ScheduledInterview[]>(
      `/api/v1/me/interviews?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    );
  }

  /** The jobs the current user has run the AI fit analysis on (newest first), plus their
   *  AI-credits balance — powers the Tracking → AI fit tab. Never triggers the LLM. */
  async function myAnalyses(): Promise<{ items: MyAnalysisItem[]; credits: AiCredits | null }> {
    const res = await request<{ data: MyAnalysisItem[]; meta: { credits: AiCredits | null } }>(
      '/api/v1/me/tracking/analyses',
    );
    return { items: res.data, credits: res.meta.credits };
  }

  /** The caller's current AI-credits balance (credits left this month + reset date).
   *  Never triggers the LLM. Powers the Credits page balance headline. */
  async function myCredits(): Promise<AiCredits> {
    return requestData<AiCredits>('/api/v1/me/credits');
  }

  /** What the caller's account did this period: model calls, failures and tokens, read
   *  from the LLM gateway. Never fails for anything the caller can act on — an account
   *  that has never used AI, and a gateway that is down, both answer zeroes. */
  async function myUsage(): Promise<AiUsage> {
    return requestData<AiUsage>('/api/v1/me/usage');
  }

  /** The caller's credit transaction history, newest first — grants, match/tailor debits,
   *  and contribution rewards, each labelled for display. Powers the Credits page list. */
  async function myCreditsHistory(): Promise<CreditHistoryEntry[]> {
    return requestData<CreditHistoryEntry[]>('/api/v1/me/credits/history');
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

  // --- Notification settings -------------------------------------------------
  //
  // One account-level rule (enable, channels) gates saved-job reminders and both
  // lifecycle nudges (follow-up, interview-prep). Cookie-only.

  /** The caller's notification rule (on by default for a never-configured account). */
  async function getNotificationSettings(): Promise<NotificationSettings> {
    return requestData<NotificationSettings>('/api/v1/me/notification-settings');
  }

  /** Replace the caller's notification rule. An enabled rule needs at least one
   *  channel, else the server rejects it (400). */
  async function updateNotificationSettings(settings: NotificationSettings): Promise<NotificationSettings> {
    return requestData<NotificationSettings>('/api/v1/me/notification-settings', jsonBody('PUT', settings));
  }

  /** Sets the account's IANA timezone (e.g. "Europe/Moscow"). 400s on a name Go's
   *  tzdata does not recognize. Returns the updated user (carries the new value). */
  async function updateTimezone(timezone: string): Promise<User> {
    return requestData<User>('/api/v1/me/timezone', jsonBody('PATCH', { timezone }));
  }

  /** The public slugs of every job the current user has hidden (dismissed). The
   *  browse UI cross-references this set to exclude hidden jobs from the feed
   *  without authenticating the public job list. */
  async function listDismissedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/dismissed');
  }

  // --- Notification center ---------------------------------------------------
  //
  // The durable, readable record of every subscription-digest/reminder/nudge
  // delivered to the caller over any channel — independent of the notification
  // *rule* above, which only gates whether those events fire at all.

  /** A page of the caller's notification-center entries, newest first.
   *  `meta.unread_count` rides alongside the page (not a separate endpoint — see
   *  design.md's non-goals) so the bell badge can be refreshed from the same call
   *  that fetches a page. */
  async function getNotifications(
    limit = 20,
    offset = 0,
  ): Promise<{ data: NotificationItem[]; meta: { total: number; unread_count: number; limit: number; offset: number } }> {
    return request<{
      data: NotificationItem[];
      meta: { total: number; unread_count: number; limit: number; offset: number };
    }>(`/api/v1/me/notifications${query(limit, offset)}`);
  }

  /** One notification, including its `jobs` snapshot when it has one — the
   *  read a direct visit/bookmark of the digest jobs-list page needs, since
   *  getNotifications alone only ever serves the caller's current page. */
  async function getNotification(id: number): Promise<NotificationItem> {
    return requestData<NotificationItem>(`/api/v1/me/notifications/${id}`);
  }

  /** Mark one notification read. Idempotent; owner-scoped (404s for another
   *  user's, but that never happens from this client since ids come from the
   *  caller's own list). */
  async function markNotificationRead(id: number): Promise<void> {
    await call(`/api/v1/me/notifications/${id}/read`, { method: 'POST' });
  }

  /** Mark every currently-unread notification of the caller's read; returns the
   *  count marked. */
  async function markAllNotificationsRead(): Promise<{ marked: number }> {
    return requestData<{ marked: number }>('/api/v1/me/notifications/read-all', { method: 'POST' });
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

  // The experience bank: what the product has recorded about what the user has done.

  /** The caller's whole experience bank, grouped by role. */
  async function getExperience(): Promise<ExperienceBank> {
    return requestData<ExperienceBank>('/api/v1/me/experience');
  }

  /** Rewrite one achievement. Editing it re-stamps it as the owner's own statement, which
   *  is how something the assistant inferred becomes usable on a CV. */
  async function updateExperienceAtom(id: string, atom: Partial<ExperienceAtom>): Promise<ExperienceAtom> {
    return requestData<ExperienceAtom>(`/api/v1/me/experience/atoms/${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(atom),
    });
  }

  /** Fold two achievements into one richer keep. Cookie-only — merge deletes the other. */
  async function mergeExperienceAtoms(ids: [string, string]): Promise<ExperienceAtom> {
    return requestData<ExperienceAtom>(
      '/api/v1/me/experience/atoms/merge',
      jsonBody('POST', { ids }),
    );
  }

  /** Remove one achievement. This is the only path that takes evidence out of the bank. */
  async function deleteExperienceAtom(id: string): Promise<void> {
    await call(`/api/v1/me/experience/atoms/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** Remove a role, and with it the achievements that were evidence of it. */
  async function deleteExperienceEmployment(id: string): Promise<void> {
    await call(`/api/v1/me/experience/employments/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** Create a job or project employment in the bank. */
  async function createExperienceEmployment(
    body: Partial<ExperienceEmployment> & { kind: 'job' | 'project' },
  ): Promise<ExperienceEmployment> {
    return requestData<ExperienceEmployment>(
      '/api/v1/me/experience/employments',
      jsonBody('POST', body),
    );
  }

  /** Update employment metadata (name/company, link, dates). Cookie-only. */
  async function updateExperienceEmployment(
    id: string,
    body: Partial<ExperienceEmployment>,
  ): Promise<ExperienceEmployment> {
    return requestData<ExperienceEmployment>(
      `/api/v1/me/experience/employments/${encodeURIComponent(id)}`,
      jsonBody('PUT', body),
    );
  }

  // The single per-user profile: a specialization + skills set (cookie-only on the server).

  /** The current user's profile, or null when they have not saved one yet. */
  async function getProfile(): Promise<UserProfile | null> {
    return requestData<UserProfile | null>('/api/v1/me/profile');
  }

  /** Create-or-replace the user's profile from a non-empty set of specializations (job
   *  categories), a non-empty set of skills, an optional set of excluded skills (skills to
   *  avoid; may be empty), and an optional location-preferences block (null clears it). A
   *  bad specialization, empty skills, or an out-of-vocabulary location value is a 400. */
  async function saveProfile(
    specializations: string[],
    skills: string[],
    excludedSkills: string[],
    location: LocationPreferences | null,
  ): Promise<UserProfile> {
    return requestData<UserProfile>(
      '/api/v1/me/profile',
      jsonBody('PUT', {
        specializations,
        skills,
        excluded_skills: excludedSkills,
        location_preferences: location,
      }),
    );
  }

  /** Clear the user's profile. Idempotent. */
  async function deleteProfile(): Promise<void> {
    await call('/api/v1/me/profile', { method: 'DELETE' });
  }

  // The candidate's own screening answers — the handful of questions that repeat across
  // ATS application forms (visa, salary, notice period, relocation, …) and no CV states.
  // Distinct from the profile above: a different lifecycle, see
  // internal/screeninganswers/AGENTS.md.

  /** The current user's screening answers, or null when they have stated none yet. */
  async function getScreeningAnswers(): Promise<Answers | null> {
    return requestData<Answers | null>('/api/v1/me/screening-answers');
  }

  /** Partially update the user's screening answers: a field the patch omits keeps
   *  whatever was already stored. An unrecognized country code, a malformed currency, an
   *  out-of-vocabulary period, a non-positive salary, or a negative notice period is a
   *  400 naming the invalid field. */
  async function updateScreeningAnswers(patch: Partial<Answers>): Promise<Answers> {
    return requestData<Answers>('/api/v1/me/screening-answers', jsonBody('PUT', patch));
  }

  // Talent Network: the caller's own opt-in visibility setting (distinct from the public,
  // unauthenticated profile page at GET /talent-network/:publicID — see
  // internal/handler/me_talent_network.go and talent_network_profile.go).

  /** The caller's current Talent Network visibility and public id. A user who has never
   *  touched the setting reads "off". */
  async function getTalentNetwork(): Promise<TalentNetworkSetting> {
    return requestData<TalentNetworkSetting>('/api/v1/me/talent-network');
  }

  /** Update the caller's own Talent Network visibility. Returns the echoed setting rather
   *  than assuming success, same as saveProfile. */
  async function setTalentNetworkVisibility(
    visibility: TalentNetworkVisibility,
  ): Promise<TalentNetworkSetting> {
    return requestData<TalentNetworkSetting>(
      '/api/v1/me/talent-network',
      jsonBody('PUT', { visibility }),
    );
  }

  /** The public, unauthenticated Talent Network profile page for one candidate, keyed by
   *  their opaque `talent_network_public_id`. No auth: this is the shareable-link
   *  counterpart to getTalentNetwork above. A hidden ("off") or nonexistent id both
   *  answer 404 — the caller must not try to tell them apart. */
  async function getTalentNetworkProfile(publicId: string): Promise<TalentNetworkProfile> {
    return requestData<TalentNetworkProfile>(
      `/api/v1/talent-network/${encodeURIComponent(publicId)}`,
    );
  }

  /** Permanently erase the signed-in account and everything it owns. Irreversible:
   *  there is no restore path on either side. `email` is the confirmation — it must be
   *  the caller's own address, or the server answers 400. A 503 means nothing was
   *  deleted (stored files were unreachable) and the request can be retried. */
  async function deleteAccount(email: string): Promise<void> {
    await call('/api/v1/me', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    });
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
    if (input instanceof File && input.size > RESUME_MAX_BYTES) {
      throw new ApiError(
        413,
        `This PDF is larger than ${RESUME_MAX_MB} MB. Compress it or export a lighter PDF and try again.`,
      );
    }
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

  /** Replace candidate-owned contacts without re-uploading a CV. */
  async function putResumeContacts(contacts: CandidateContacts): Promise<CandidateContacts> {
    return requestData<CandidateContacts>('/api/v1/me/resume/contacts', jsonBody('PUT', contacts));
  }

  /** Overwrite owned contacts from the current structured extract. */
  async function replaceResumeContactsFromCV(): Promise<CandidateContacts> {
    return requestData<CandidateContacts>('/api/v1/me/resume/contacts/replace-from-cv', {
      method: 'POST',
    });
  }

  /** Re-run structured parse for the stored CV (no re-upload). */
  async function retryResumeParse(): Promise<{ parse_status: string }> {
    return requestData<{ parse_status: string }>('/api/v1/me/resume/parse', { method: 'POST' });
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

  /** Whether Discord contribution linking is configured and whether this user is linked. */
  async function discordStatus(): Promise<DiscordStatus> {
    return requestData<DiscordStatus>('/api/v1/me/discord');
  }

  /** Mint a one-time token: the user runs `/link token:<token>` in the freehire Discord
   *  server to connect their account. */
  async function discordLink(): Promise<DiscordLinkResult> {
    return requestData<DiscordLinkResult>('/api/v1/me/discord/link', { method: 'POST' });
  }

  /** Disconnect the user's linked Discord account. */
  async function discordUnlink(): Promise<void> {
    await call('/api/v1/me/discord', { method: 'DELETE' });
  }

  /** Submit a vacancy for moderation. Returns the pending submission. */
  async function submitJob(input: SubmissionInput): Promise<Submission> {
    return requestData<Submission>('/api/v1/submissions', jsonBody('POST', input));
  }

  /** The caller's own submissions with their review status. */
  async function listMySubmissions(): Promise<Submission[]> {
    return requestData<Submission[]>('/api/v1/me/submissions');
  }

  /** Parse a job URL into draft field values for the submit form to prefill — never
   *  persisted. An unrecognized or non-vacancy URL comes back with every field empty,
   *  not an error. */
  async function prefillSubmission(url: string): Promise<PrefillResult> {
    return requestData<PrefillResult>('/api/v1/submissions/prefill', jsonBody('POST', { url }));
  }

  /** Hand a job link to freehire. One sequence serves every surface: the catalog is checked,
   *  the vacancy imported when anything can read it, and the board behind it recorded for
   *  onboarding either way. The outcome says which of those happened (422 for a non-URL). */
  async function resolveJobLink(url: string): Promise<ResolvedLink> {
    return requestData<ResolvedLink>('/api/v1/jobs/resolve', jsonBody('POST', { url, surface: 'web' }));
  }

  /** The caller's own link contributions, newest first. */
  async function listMyContributions(): Promise<Contribution[]> {
    return requestData<Contribution[]>('/api/v1/me/contributions');
  }

  // ── Employee referrals ────────────────────────────────────────────────────

  /** Request a referral into a company's approved-referrer pool. 409 when the company has
   *  no referrer or an active request already exists; 422 on a bad CV/contact; 429 on cap. */
  async function createReferralRequest(input: ReferralRequestInput): Promise<SeekerReferralRequest> {
    return requestData<SeekerReferralRequest>('/api/v1/me/referrals/requests', jsonBody('POST', input));
  }

  /** The caller's own referral requests, newest first. */
  async function listMyReferralRequests(): Promise<SeekerReferralRequest[]> {
    return requestData<SeekerReferralRequest[]>('/api/v1/me/referrals/requests');
  }

  /** Submit an offer to refer into a company: a proof CV (PDF) uploaded as multipart, with
   *  the company slug and the referrer's LinkedIn URL as form fields. Enters moderation.
   *  409 on a duplicate offer, 422 on a bad LinkedIn URL. */
  async function submitReferralOffer(
    companySlug: string,
    linkedinUrl: string,
    file: File,
  ): Promise<ReferralOffer> {
    const form = new FormData();
    form.append('company_slug', companySlug);
    form.append('linkedin_url', linkedinUrl);
    form.append('file', file);
    return requestData<ReferralOffer>('/api/v1/me/referrals/offers', { method: 'POST', body: form });
  }

  /** The caller's own referral offers with moderation status, newest first. */
  async function listMyReferralOffers(): Promise<ReferralOffer[]> {
    return requestData<ReferralOffer[]>('/api/v1/me/referrals/offers');
  }

  /** Stop being a referrer: delete one of the caller's own offers. */
  async function withdrawReferralOffer(id: string): Promise<void> {
    await call(`/api/v1/me/referrals/offers/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** The referrer inbox: open requests for the companies the caller is approved for. */
  async function listIncomingReferrals(): Promise<IncomingReferralRequest[]> {
    return requestData<IncomingReferralRequest[]>('/api/v1/me/referrals/incoming');
  }

  /** Mark an incoming request contacted or declined on the caller's behalf. */
  async function resolveReferral(
    id: string,
    status: 'contacted' | 'declined',
  ): Promise<IncomingReferralRequest> {
    return requestData<IncomingReferralRequest>(
      `/api/v1/me/referrals/incoming/${encodeURIComponent(id)}/resolve`,
      jsonBody('POST', { status }),
    );
  }

  /** The URL that streams an incoming request's attached CV (opened in a new tab). */
  function referralCvUrl(id: string): string {
    return `${baseUrl}/api/v1/me/referrals/incoming/${encodeURIComponent(id)}/cv`;
  }

  /** The moderator queue: referral offers awaiting a decision, oldest first. */
  async function listPendingReferralOffers(): Promise<ReferralOffer[]> {
    return requestData<ReferralOffer[]>('/api/v1/referrals/offers');
  }

  /** Approve or reject a pending offer. Moderator-only. */
  async function decideReferralOffer(id: string, approve: boolean): Promise<ReferralOffer> {
    return requestData<ReferralOffer>(`/api/v1/referrals/offers/${encodeURIComponent(id)}/decide`, jsonBody('POST', { approve }));
  }

  /** The URL that streams an offer's proof CV (moderator-only, opened in a new tab). */
  function referralProofUrl(id: string): string {
    return `${baseUrl}/api/v1/referrals/offers/${encodeURIComponent(id)}/proof`;
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

  /** File a ghost-signal claim: applied on this date, never answered. Unlike
   *  reportJob this reaches no moderator and closes nothing — it accumulates as
   *  evidence, and `withdrawGhostReport` takes it back when an employer answers. */
  async function reportGhostJob(slug: string, input: GhostReportInput): Promise<void> {
    await call(`/api/v1/jobs/${slug}/ghost-report`, jsonBody('POST', input));
  }

  /** Withdraw this caller's ghost claim about a job (204, or 404 if there is none). */
  async function withdrawGhostReport(slug: string): Promise<void> {
    // call(), not request(): a 204 carries no body, and .json() on an empty one
    // throws on a call that in fact succeeded — the mirror of why the endpoint
    // answers 204 rather than a 200 whose body would read "OK".
    await call(`/api/v1/jobs/${slug}/ghost-report`, { method: 'DELETE' });
  }

  /** The moderator review queue: pending reports, with reporter email and job fields. */
  async function listPendingReports(): Promise<Report[]> {
    return requestData<Report[]>('/api/v1/reports');
  }

  /** Resolve a pending report; optionally soft-close the reported job. The note is
   *  emailed to the reporter when `notifyReporter` is set. */
  async function resolveReport(
    id: number,
    closeJob: boolean,
    note = '',
    notifyReporter = false,
  ): Promise<Report> {
    return requestData<Report>(
      `/api/v1/reports/${id}/resolve`,
      jsonBody('POST', { close_job: closeJob, note, notify_reporter: notifyReporter }),
    );
  }

  /** Dismiss a pending report with an optional reason; the job is unchanged. The reason
   *  is emailed to the reporter when `notifyReporter` is set. */
  async function dismissReport(id: number, reason = '', notifyReporter = false): Promise<Report> {
    return requestData<Report>(
      `/api/v1/reports/${id}/dismiss`,
      jsonBody('POST', { reason, notify_reporter: notifyReporter }),
    );
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

  /** A page of the flat inbox listing, newest first. Optional search term filters
   *  by subject, sender, or body; optional source narrows to one account (the
   *  switcher). `meta.total` reflects the filtered count, so the Paginator pages
   *  over the matches. */
  async function getInbox(
    opts: {
      q?: string;
      limit?: number;
      offset?: number;
      source?: InboxSource;
      unread?: boolean;
      status?: string;
      /** Ask for mail the classifier judged not to be about an application at all. The
       *  listing hides it by default and reports how many it hid. */
      includeOther?: boolean;
    } = {},
  ): Promise<Slice<InboxMessage> & { hidden: number }> {
    const params = new URLSearchParams({
      limit: String(opts.limit ?? 20),
      offset: String(opts.offset ?? 0),
    });
    if (opts.q) params.set('q', opts.q);
    if (opts.source) params.set('source', opts.source);
    if (opts.unread) params.set('unread', '1');
    if (opts.status) params.set('status', opts.status);
    if (opts.includeOther) params.set('include_other', '1');
    const page = await request<Page<InboxMessage> & { meta: { hidden?: number } }>(
      `/api/v1/me/inbox?${params.toString()}`
    );
    return { ...toSlice(page, opts.offset ?? 0), hidden: page.meta.hidden ?? 0 };
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

  /** Sweep the caller's mailbox for mail belonging to this application. The matches come
   *  back as SUGGESTIONS — nothing is linked — and are resolved with confirmEmailLink /
   *  rejectEmailLink, the same calls the inbox uses. 502 when the model could not be
   *  reached, which is deliberately not an empty result: "your mailbox holds nothing" is
   *  the wrong thing to say about a gateway being down. */
  async function recallApplicationMail(slug: string): Promise<MailRecallResult> {
    return requestData<MailRecallResult>(
      `/api/v1/me/tracking/${encodeURIComponent(slug)}/mail-recall`,
      { method: 'POST' }
    );
  }

  /** Import a message the sweep found in the mailbox and link it to the application.
   *  The sweep itself stores nothing — a proposal lives on screen only — so this is the
   *  call that makes the message ours, and it is idempotent: a message the mail sync had
   *  already fetched is linked rather than copied. */
  async function linkRecalledMail(slug: string, providerId: string): Promise<EmailBody> {
    return requestData<EmailBody>(
      `/api/v1/me/tracking/${encodeURIComponent(slug)}/mail-recall/link`,
      jsonBody('POST', { provider_id: providerId })
    );
  }

  /** The assembled follow-up draft for a silent application. 409 when the
   *  application is not waiting on a reply — the same verdict the board's badge
   *  renders, so a card that offers the draft never gets one. */
  async function getFollowUpDraft(slug: string): Promise<FollowUpDraft> {
    return requestData<FollowUpDraft>(`/api/v1/me/tracking/${encodeURIComponent(slug)}/followup`);
  }

  /** Record that the caller chased this application. Sends nothing — it stamps
   *  followed_up_at, which stays outside the silence derivation. 204, no body. */
  async function recordFollowUp(slug: string): Promise<void> {
    await call(`/api/v1/me/tracking/${encodeURIComponent(slug)}/followup`, { method: 'POST' });
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

  // --- CVs and tailoring (open to every signed-in user; credits meter the AI spend) ---

  /** The caller's headshot: whether the feature is configured at all (`enabled`), whether
   *  one is stored, and when it was uploaded. Always 200 — "no photo yet" and "storage is
   *  off" are both states the profile renders. */
  async function getPhoto(): Promise<PhotoMeta> {
    return requestData<PhotoMeta>('/api/v1/me/photo');
  }

  /** Store (or replace) the caller's headshot. The server normalizes it to a square JPEG,
   *  so an image it cannot decode comes back as a 400 and nothing is stored. */
  async function putPhoto(file: File): Promise<PhotoMeta> {
    if (file.size > PHOTO_MAX_BYTES) {
      throw new ApiError(
        413,
        `This image is larger than ${PHOTO_MAX_MB} MB. Pick a smaller photo and try again.`,
      );
    }
    const form = new FormData();
    form.append('file', file);
    return requestData<PhotoMeta>('/api/v1/me/photo', { method: 'PUT', body: form });
  }

  /** Remove the caller's headshot (object + pointer). */
  async function deletePhoto(): Promise<void> {
    await call('/api/v1/me/photo', { method: 'DELETE' });
  }

  /** List the available CV templates (id, label, style, photo) for the gallery. */
  async function listCvTemplates(): Promise<CvTemplate[]> {
    return requestData<CvTemplate[]>('/api/v1/cv-templates');
  }

  /** List the typefaces a CV may use (id, label, note, css) for the font picker. */
  async function listCvFonts(): Promise<CvFont[]> {
    return requestData<CvFont[]>('/api/v1/cv-fonts');
  }

  /** Switch a CV's template only (title + document untouched). */
  async function setCvTemplate(id: string, templateId: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/template`, jsonBody('PUT', { template_id: templateId }));
  }

  /** List the caller's TAILORED CVs (the re-open list): each with its vacancy slug + bound
   *  agent session, newest edit first. */
  async function listCvs(): Promise<CvTailoredItem[]> {
    return requestData<CvTailoredItem[]>('/api/v1/me/cvs');
  }

  /** Bind a roy agent session to a CV so its workspace can re-open that exact session. */
  async function setCvSession(id: string, sessionId: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/session`, jsonBody('PUT', { session_id: sessionId }));
  }

  /** Fetch one CV with its full document. */
  async function getCv(id: string): Promise<CvRecord> {
    return requestData<CvRecord>(`/api/v1/me/cvs/${id}`);
  }

  /** What tailoring did to a tailored CV's ATS readiness, against the base CV it came from.
   *  Cookie-only and recomputed per request. 409 for a CV that is not a tailored copy; the
   *  response itself reports `available: false` when the comparison could not be made. */
  /** The history of what changed this CV, newest first. Each entry names who made it and
   *  which parts of the document it touched — the addresses the preview underlines. */
  async function listCvRevisions(id: string): Promise<RevisionView[]> {
    return requestData<RevisionView[]>(`/api/v1/me/cvs/${encodeURIComponent(id)}/revisions`);
  }

  /** Undo one entry. Edits made after it survive: only what that revision did is reversed.
   *  409 when its inverse no longer applies — the part of the CV it changed is gone. */
  async function undoCvRevision(cvId: string, revisionId: string): Promise<CvMeta> {
    const { cv } = await requestData<{ cv: CvMeta; revision: RevisionView }>(
      `/api/v1/me/cvs/${encodeURIComponent(cvId)}/revisions/${encodeURIComponent(revisionId)}/undo`,
      jsonBody('POST', {}),
    );
    return cv;
  }

  /** Undo every standing edit of one assistant run, newest first. */
  async function undoCvRevisionRun(cvId: string, batchId: string): Promise<CvMeta> {
    return requestData<CvMeta>(
      `/api/v1/me/cvs/${encodeURIComponent(cvId)}/revisions/batch/${encodeURIComponent(batchId)}/undo`,
      jsonBody('POST', {}),
    );
  }

  /** Rebuild this tailored CV (and the base) from the current résumé seed. Cookie-only.
   *  Same id and agent session; presentation preserved. 409 when the CV is not tailored or
   *  there is nothing to seed from. */
  async function resetCvFromResume(id: string): Promise<CvRecord> {
    return requestData<CvRecord>(
      `/api/v1/me/cvs/${encodeURIComponent(id)}/reset-from-resume`,
      jsonBody('POST', {}),
    );
  }

  /** Rebuild the base CV from the current résumé seed. Cookie-only. Does not touch
   *  tailored copies. 409 when there is nothing to seed from. */
  async function resetBaseCvFromResume(): Promise<CvRecord> {
    return requestData<CvRecord>('/api/v1/me/cvs/base/reset-from-resume', jsonBody('POST', {}));
  }

  async function getCvAtsDelta(id: string): Promise<CvAtsDelta> {
    return requestData<CvAtsDelta>(`/api/v1/me/cvs/${id}/ats-delta`);
  }

  /** How well a tailored CV matches its vacancy, scored on the current document. Cookie-only
   *  and recomputed per request. 409 for a CV bound to no vacancy; the response itself reports
   *  `available: false` when no score could be produced. */
  async function getCvJobMatch(id: string): Promise<CvJobMatch> {
    return requestData<CvJobMatch>(`/api/v1/me/cvs/${id}/job-match`);
  }

  /** Replace a CV's title, template, and document. */
  async function updateCv(id: string, input: UpdateCvInput): Promise<CvMeta> {
    return requestData<CvMeta>(`/api/v1/me/cvs/${id}`, jsonBody('PUT', input));
  }


  /** Turn link tracing on or off for one CV. 409 when the deployment has no visitor salt: there
   *  is then no honest way to count visitors, so the consent is refused rather than accepted and
   *  quietly under-recorded. Turning it OFF is never refused. */
  async function setCvTracerLinks(id: string, enabled: boolean): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/tracer-links`, jsonBody('PUT', { enabled }));
  }

  /** What is known about each of a CV's traced links. Empty for a CV that was never traced. */
  async function listCvTracerLinks(id: string): Promise<CvTracerLink[]> {
    return requestData<CvTracerLink[]>(`/api/v1/me/cvs/${id}/tracer-links`);
  }

  /** Delete a CV. */
  async function deleteCv(id: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}`, { method: 'DELETE' });
  }

  /** The authenticated PDF URL for a CV (same-origin cookie rides along on download). */
  function cvPdfUrl(id: string): string {
    return `${baseUrl}/api/v1/me/cvs/${id}/pdf`;
  }

  /**
   * Bootstrap a tailoring session for a vacancy: seeds/creates the base CV, makes a
   * vacancy-bound tailored copy, and returns its id plus the cached analysis. Requires a
   * cached fit analysis and a stored résumé (409 otherwise); beta-gated.
   */
  async function tailorCv(jobSlug: string): Promise<TailorResult> {
    return requestData<TailorResult>('/api/v1/me/cvs/tailor', jsonBody('POST', { job_slug: jobSlug }));
  }

  /**
   * Re-establish a tailoring session for an EXISTING tailored CV that has no bound agent session
   * (one created before session binding): mints a fresh CLI token and returns the CV + base ids
   * so the workspace can seed a new agent session against the same CV.
   */
  async function startTailorSession(id: string): Promise<TailorResult> {
    return requestData<TailorResult>(`/api/v1/me/cvs/${id}/tailor-session`, jsonBody('POST', {}));
  }

  /**
   * Turn a job slug, an external URL, or pasted JD text into a job slug the tailoring
   * workspace can open. Exactly one of `job_slug`/`url`/`text` must be set. A URL a
   * recognized ATS can read becomes a normal catalog job; anything else (a generic scrape
   * or plain text) becomes a private job — visible only via its own slug, never listed or
   * searchable. 422 when a URL cannot be read at all.
   */
  async function resolveJd(input: JdResolveInput): Promise<string> {
    const { job_slug } = await requestData<{ job_slug: string }>(
      '/api/v1/me/jd/resolve',
      jsonBody('POST', input),
    );
    return job_slug;
  }

  /** A subject's open discussion threads, newest first. `subjectType` is 'company'
   *  or 'job', `subjectSlug` the subject's public slug. `nextCursor` (when present)
   *  fetches the following keyset page. Public — no auth needed to read. */
  async function listThreads(
    subjectType: string,
    subjectSlug: string,
    cursor?: string,
  ): Promise<{ threads: CommunityThread[]; nextCursor?: string }> {
    const qs = new URLSearchParams({ subject_type: subjectType, subject_slug: subjectSlug });
    if (cursor) qs.set('cursor', cursor);
    const res = await request<{ data: CommunityThread[]; meta: { next_cursor?: string } }>(
      `/api/v1/threads?${qs}`,
    );
    return { threads: res.data, nextCursor: res.meta?.next_cursor };
  }

  /** How many open discussion threads a subject has — the detail-page badge. Public. */
  async function countThreads(subjectType: string, subjectSlug: string): Promise<number> {
    const qs = new URLSearchParams({ subject_type: subjectType, subject_slug: subjectSlug });
    return (await requestData<{ count: number }>(`/api/v1/threads/count?${qs}`)).count;
  }

  /** A single thread with its first page of replies (oldest first). Public. */
  async function getThread(
    id: number,
    cursor?: string,
  ): Promise<{ thread: CommunityThread; replies: CommunityReply[]; nextCursor?: string }> {
    const suffix = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
    const res = await request<{
      data: { thread: CommunityThread; replies: CommunityReply[] };
      meta: { next_cursor?: string };
    }>(`/api/v1/threads/${id}${suffix}`);
    return { thread: res.data.thread, replies: res.data.replies, nextCursor: res.meta?.next_cursor };
  }

  /** Open a thread on a subject. Requires a signed-in session (401 otherwise). */
  async function createThread(input: {
    subject_type: string;
    subject_slug: string;
    title: string;
    body: string;
  }): Promise<CommunityThread> {
    return requestData<CommunityThread>('/api/v1/threads', jsonBody('POST', input));
  }

  /** Post a reply to a thread. Requires a signed-in session. `parentReplyId` nests
   *  the reply under another reply (0/omitted = a top-level reply). */
  async function createReply(
    threadId: number,
    body: string,
    parentReplyId = 0,
  ): Promise<CommunityReply> {
    return requestData<CommunityReply>(
      `/api/v1/threads/${threadId}/replies`,
      jsonBody('POST', { body, parent_reply_id: parentReplyId }),
    );
  }

  /** A company's feedback, newest first, offset-paginated. Public. */
  async function listCompanyFeedback(slug: string, limit: number, offset: number): Promise<Slice<CompanyFeedback>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return toSlice(await request<Page<CompanyFeedback>>(`/api/v1/companies/${slug}/feedback?${params}`), offset);
  }

  /** The caller's own feedback on a company, or null when they have not left one
   *  yet — the edit form's prefill. Requires a session. */
  function getMyCompanyFeedback(slug: string): Promise<CompanyFeedback | null> {
    return requestData<CompanyFeedback | null>(`/api/v1/companies/${slug}/feedback/mine`);
  }

  /** Create or overwrite the caller's feedback on a company in place
   *  (edit-by-resubmit). Requires a session. `company` is always present here
   *  (unlike the shared CompanyFeedback shape, where it's List/Mine's unused
   *  optional field) — the freshly recomputed counters, so the caller can
   *  update its own view of the company without a second round trip. */
  function upsertCompanyFeedback(
    slug: string,
    input: { rating: number; feedback_type: string; body: string },
  ): Promise<CompanyFeedback & { company: CompanyFeedbackSummary }> {
    return requestData<CompanyFeedback & { company: CompanyFeedbackSummary }>(
      `/api/v1/companies/${slug}/feedback`,
      jsonBody('POST', input),
    );
  }

  /** Delete the caller's own feedback on a company (idempotent — a no-op when
   *  none exists still succeeds), returning the company's freshly recomputed
   *  counters so the caller never needs a follow-up fetch to learn the new
   *  count. */
  function deleteCompanyFeedback(slug: string): Promise<CompanyFeedbackSummary> {
    return requestData<CompanyFeedbackSummary>(`/api/v1/companies/${slug}/feedback`, { method: 'DELETE' });
  }

  /** Flag a specific review as spam/offensive/false/other — evidence for a
   *  moderator, not a full report ticket (see reportJob for that fuller
   *  shape). A second report of the same review by the same caller is a
   *  silent no-op. Requires a session. */
  async function reportCompanyFeedback(feedbackId: number, reason: string): Promise<void> {
    await call(`/api/v1/company-feedback/${feedbackId}/report`, jsonBody('POST', { reason }));
  }

  /** The moderator queue: every review with at least one report, most-reported
   *  first. Role-gated. */
  function listReportedCompanyFeedback(): Promise<ReportedCompanyFeedback[]> {
    return requestData<ReportedCompanyFeedback[]>('/api/v1/company-feedback/reported');
  }

  /** Hide a specific review, dropping it out of its company's public list and
   *  average immediately. Idempotent. Role-gated. */
  async function hideCompanyFeedback(feedbackId: number): Promise<void> {
    await call(`/api/v1/company-feedback/${feedbackId}/hide`, { method: 'POST' });
  }

  return {
    listJobs,
    getJob,
    getSimilarJobs,
    getJobCopies,
    getApplyForm,
    getJobMatch,
    getMatchAnalysis,
    runMatchAnalysis,
    matchAnalysisStreamUrl,
    searchJobs,
    swipeDeck,
    recommendations,
    facetCounts,
    jobsActivity,
    userGrowth,
    engagementStats,
    statsFacets,
    ingestStatus,
    listCompanies,
    getCompany,
    listCompanySubindustries,
    searchCities,
    insightsRoles,
    insightsSkills,
    insightsSalaryByCategory,
    insightsCompanies,
    marketPulse,
    sitemapJobs,
    sitemapCompanies,
    sitemapCompanyBoundaries,
    register,
    login,
    requestEmailVerification,
    confirmEmailVerification,
    forgotPassword,
    resetPassword,
    changePassword,
    reauthenticatePassword,
    exchangeOAuthReauthentication,
    connectedIdentities,
    logoutEverywhere,
    oauthProviders,
    logout,
    me,
    recordJobView,
    markJobApplied,
    saveJob,
    unsaveJob,
    dismissJob,
    undismissJob,
    voteJob,
    clearJobVote,
    voteCompany,
    clearCompanyVote,
    clearJobStage,
    untrackJob,
    trackJob,
    trackApplication,
    clearApplicationStage,
    untrackApplication,
    listMyJobs,
    getMyPipeline,
    myTimeline,
    myInterviews,
    myAnalyses,
    myCredits,
    myCreditsHistory,
    myUsage,
    listViewedSlugs,
    listSavedSlugs,
    getNotificationSettings,
    updateNotificationSettings,
    updateTimezone,
    listDismissedSlugs,
    getNotifications,
    getNotification,
    markNotificationRead,
    markAllNotificationsRead,
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
    getExperience,
    updateExperienceAtom,
    mergeExperienceAtoms,
    deleteExperienceAtom,
    deleteExperienceEmployment,
    createExperienceEmployment,
    updateExperienceEmployment,
    getProfile,
    saveProfile,
    deleteProfile,
    getScreeningAnswers,
    updateScreeningAnswers,
    getTalentNetwork,
    setTalentNetworkVisibility,
    getTalentNetworkProfile,
    deleteAccount,
    extractResumeProfile,
    getResume,
    putResumeContacts,
    replaceResumeContactsFromCV,
    retryResumeParse,
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
    discordStatus,
    discordLink,
    discordUnlink,
    submitJob,
    listMySubmissions,
    prefillSubmission,
    resolveJobLink,
    listMyContributions,
    createReferralRequest,
    listMyReferralRequests,
    submitReferralOffer,
    listMyReferralOffers,
    withdrawReferralOffer,
    listIncomingReferrals,
    resolveReferral,
    referralCvUrl,
    listPendingReferralOffers,
    decideReferralOffer,
    referralProofUrl,
    listPendingSubmissions,
    approveSubmission,
    rejectSubmission,
    reportJob,
    reportGhostJob,
    withdrawGhostReport,
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
    recallApplicationMail,
    linkRecalledMail,
    getFollowUpDraft,
    recordFollowUp,
    confirmEmailLink,
    rejectEmailLink,
    linkEmail,
    unlinkEmail,
    listCvs,
    getPhoto,
    putPhoto,
    deletePhoto,
    listCvTemplates,
    listCvFonts,
    setCvTemplate,
    getCv,
    getCvAtsDelta,
    getCvJobMatch,
    updateCv,
    setCvTracerLinks,
    listCvTracerLinks,
    deleteCv,
    setCvSession,
    cvPdfUrl,
    listCvRevisions,
    undoCvRevision,
    undoCvRevisionRun,
    resetCvFromResume,
    resetBaseCvFromResume,
    tailorCv,
    startTailorSession,
    resolveJd,
    listThreads,
    countThreads,
    getThread,
    createThread,
    createReply,
    listCompanyFeedback,
    getMyCompanyFeedback,
    upsertCompanyFeedback,
    deleteCompanyFeedback,
    reportCompanyFeedback,
    listReportedCompanyFeedback,
    hideCompanyFeedback,
  };
}

export type MyJobsFilter = 'all' | 'viewed' | 'saved' | 'applied' | 'board' | 'dismissed';

/** The default browser client: global fetch, same-origin, cookie attached. Client
 *  components call methods on it (`api.foo()`); server `load` uses `serverApi(event.fetch)`. */
export const api = createApi();
