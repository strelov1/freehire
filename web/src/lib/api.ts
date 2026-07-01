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

import type {
  Job,
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
  SearchProfile,
  Subscription,
  TelegramStatus,
  Submission,
  SubmissionInput,
  Report,
  ReportInput,
  Verdict,
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

  async function listJobs(limit: number, offset: number): Promise<Slice<Job>> {
    return toSlice(await request<Page<Job>>(`/api/v1/jobs${query(limit, offset)}`), offset);
  }

  async function getJob(slug: string): Promise<Job> {
    const body = await request<{ data: Job }>(`/api/v1/jobs/${slug}`);
    return body.data;
  }

  /** Jobs semantically nearest to the one addressed by `slug` — the "Similar jobs"
   *  section on the detail page. Same Job wire shape as the list, so the same card
   *  renders them; the source job is excluded by the backend. */
  async function getSimilarJobs(slug: string): Promise<Job[]> {
    const body = await request<{ data: Job[] }>(`/api/v1/jobs/${slug}/similar`);
    return body.data;
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
   *  `searchJobs`, minus the caller's already-judged (saved or dismissed) jobs,
   *  excluded server-side. Authenticated; batched via limit/offset for prefetch. */
  async function swipeDeck(facets: URLSearchParams, limit: number, offset: number): Promise<Slice<Job>> {
    const params = new URLSearchParams(facets);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<Job>>(`/api/v1/me/jobs/swipe?${params}`), offset);
  }

  /** Facet-distribution counts for the analytics page. `params` carries the same
   *  query text and facet filters as `searchJobs` (built by the caller, e.g. via
   *  `filtersToParams`); the endpoint returns counts instead of a page of jobs.
   *  Empty `facets`/`stats` are normalized to `{}` so callers never see null. */
  async function facetCounts(params: URLSearchParams): Promise<FacetCounts> {
    const res = await request<{ data: FacetCounts }>(`/api/v1/jobs/facets?${params}`);
    return { total: res.data.total, facets: res.data.facets ?? {}, stats: res.data.stats ?? {} };
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
    const body = await request<{ data: { company: Company; jobs: Job[] } }>(
      `/api/v1/companies/${slug}${query(limit, offset)}`,
    );
    return body.data;
  }

  // --- Sitemap --------------------------------------------------------------
  //
  // Feeds behind the sitemap index (server routes only). Jobs ship their freshest
  // slice (one file); companies are keyset-paginated across chunks, with a boundary
  // endpoint returning the cursor ending each chunk so the index can enumerate them.

  /** The freshest open-job sitemap entries (newest first), one file. */
  async function sitemapJobs(): Promise<SitemapEntry[]> {
    const res = await request<{ data: SitemapEntry[] }>('/api/v1/jobs/sitemap');
    return res.data;
  }

  /** One chunk of company sitemap entries with slug > `after` ('' for the first). */
  async function sitemapCompanies(after: string, limit: number): Promise<SitemapEntry[]> {
    const res = await request<{ data: SitemapEntry[] }>(
      `/api/v1/companies/sitemap?after=${encodeURIComponent(after)}&limit=${limit}`,
    );
    return res.data;
  }

  /** The slug cursor ending each `chunk`-sized page of companies. */
  async function sitemapCompanyBoundaries(chunk: number): Promise<string[]> {
    const res = await request<{ data: string[] }>(`/api/v1/companies/sitemap/boundaries?chunk=${chunk}`);
    return res.data;
  }

  // --- Auth -----------------------------------------------------------------
  //
  // register/login set the httpOnly auth cookie server-side and return the user;
  // the token never reaches JS. Subsequent calls (me) are authenticated by the
  // cookie the browser attaches automatically.

  /** POST credentials and return the created/authenticated user. */
  async function postAuth(path: string, body: unknown): Promise<User> {
    const res = await request<{ data: User }>(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return res.data;
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
    const res = await request<{ data: string[] }>('/api/v1/auth/oauth/providers');
    return res.data;
  }

  /** Clear the session cookie server-side. */
  async function logout(): Promise<void> {
    await call('/api/v1/auth/logout', { method: 'POST' });
  }

  /** Fetch the current user using the auth cookie. Throws ApiError(401) if it is
   *  missing or rejected. */
  async function me(): Promise<User> {
    const res = await request<{ data: User }>('/api/v1/auth/me');
    return res.data;
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
    const res = await request<{ data: UserJob }>(`/api/v1/jobs/${slug}/${action}`, { method });
    return res.data;
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
    const res = await request<{ data: UserJob }>(`/api/v1/jobs/${slug}/track`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    return res.data;
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
      `/api/v1/me/jobs${query(limit, offset)}&filter=${filter}`,
    );
    return { ...toSlice(res, offset), counts: res.meta.counts };
  }

  /** The current user's application-pipeline snapshot: per-bucket application
   *  counts aggregated server-side, for the Pipeline tab's Sankey and rate cards. */
  async function getMyPipeline(): Promise<PipelineStats> {
    const res = await request<{ data: PipelineStats }>('/api/v1/me/jobs/pipeline');
    return res.data;
  }

  /** The public slugs of every job the current user has interacted with. The
   *  browse UI cross-references this set to dim already-viewed cards without
   *  authenticating the public job list. */
  async function listViewedSlugs(): Promise<string[]> {
    const res = await request<{ data: string[] }>('/api/v1/me/jobs/viewed');
    return res.data;
  }

  // --- API keys -------------------------------------------------------------
  //
  // Personal API keys for non-browser access. Management is cookie-only (these
  // calls ride the session cookie); the plaintext token is returned once, by
  // createApiKey, and never again.

  /** The current user's API keys (metadata only — no secret). */
  async function listApiKeys(): Promise<ApiKey[]> {
    const res = await request<{ data: ApiKey[] }>('/api/v1/me/api-keys');
    return res.data;
  }

  /** Create a key and return it with its one-time plaintext `token`. `expiresAt` is
   *  an RFC3339 string, or omitted for a key that never expires. */
  async function createApiKey(name: string, expiresAt?: string): Promise<CreatedApiKey> {
    const res = await request<{ data: CreatedApiKey }>('/api/v1/me/api-keys', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, expires_at: expiresAt ?? null }),
    });
    return res.data;
  }

  /** Revoke a key by id; it stops authenticating immediately. */
  async function revokeApiKey(id: number): Promise<void> {
    await call(`/api/v1/me/api-keys/${id}`, { method: 'DELETE' });
  }

  // Saved searches: named snapshots of the filter state (cookie-only on the server).

  /** The current user's saved searches, most recently updated first. */
  async function listSavedSearches(): Promise<SavedSearch[]> {
    const res = await request<{ data: SavedSearch[] }>('/api/v1/me/searches');
    return res.data;
  }

  /** Save the current filter state under a name. `query` is the canonical search
   *  query string (may be empty). A duplicate name or the per-user cap is a 409. */
  async function createSavedSearch(name: string, query: string): Promise<SavedSearch> {
    const res = await request<{ data: SavedSearch }>('/api/v1/me/searches', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, query }),
    });
    return res.data;
  }

  /** Overwrite a saved search's name and/or query; an omitted field is unchanged. */
  async function updateSavedSearch(
    id: number,
    patch: { name?: string; query?: string },
  ): Promise<SavedSearch> {
    const res = await request<{ data: SavedSearch }>(`/api/v1/me/searches/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    return res.data;
  }

  /** Delete a saved search by id. */
  async function deleteSavedSearch(id: number): Promise<void> {
    await call(`/api/v1/me/searches/${id}`, { method: 'DELETE' });
  }

  /** Publish a saved search as a public board (cookie-only). Returns the updated set,
   *  now carrying `public_slug`. An optional `authorLabel` is shown on the board; blank
   *  renders it anonymously. Re-sharing keeps the existing slug. */
  async function shareSavedSearch(id: number, authorLabel = ''): Promise<SavedSearch> {
    const res = await request<{ data: SavedSearch }>(`/api/v1/me/searches/${id}/share`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ author_label: authorLabel }),
    });
    return res.data;
  }

  /** Make a shared board private again (cookie-only). Idempotent. */
  async function unshareSavedSearch(id: number): Promise<void> {
    await call(`/api/v1/me/searches/${id}/share`, { method: 'DELETE' });
  }

  /** Public read of a shared board by slug — unauthenticated. Returns only display
   *  fields (name, query, author_label). An unknown/unshared slug throws (404). */
  async function getBoard(slug: string): Promise<Board> {
    const res = await request<{ data: Board }>(`/api/v1/boards/${encodeURIComponent(slug)}`);
    return res.data;
  }

  // Search profiles: named specialization + skills sets (cookie-only on the server).

  /** The current user's search profiles, most recently updated first. */
  async function listSearchProfiles(): Promise<SearchProfile[]> {
    const res = await request<{ data: SearchProfile[] }>('/api/v1/me/profiles');
    return res.data;
  }

  /** Create a profile from a name, a non-empty set of specializations (job categories),
   *  and a non-empty set of skills. A duplicate name or the per-user cap is a 409; a bad
   *  specialization or empty skills is a 400. */
  async function createSearchProfile(
    name: string,
    specializations: string[],
    skills: string[],
  ): Promise<SearchProfile> {
    const res = await request<{ data: SearchProfile }>('/api/v1/me/profiles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, specializations, skills }),
    });
    return res.data;
  }

  /** Overwrite a profile's name, specializations, and/or skills; an omitted field is
   *  unchanged. */
  async function updateSearchProfile(
    id: number,
    patch: { name?: string; specializations?: string[]; skills?: string[] },
  ): Promise<SearchProfile> {
    const res = await request<{ data: SearchProfile }>(`/api/v1/me/profiles/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    return res.data;
  }

  /** Delete a profile by id. */
  async function deleteSearchProfile(id: number): Promise<void> {
    await call(`/api/v1/me/profiles/${id}`, { method: 'DELETE' });
  }

  /** Extract canonical skill slugs from a resume — a PDF `File` (sent as multipart) or
   *  pasted text (sent as JSON). The resume is parsed server-side and discarded; only the
   *  slugs come back, ready to merge into a profile's skills. */
  async function extractResumeSkills(input: File | string): Promise<string[]> {
    let init: RequestInit;
    if (typeof input === 'string') {
      init = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: input }),
      };
    } else {
      const form = new FormData();
      form.append('file', input);
      init = { method: 'POST', body: form };
    }
    const res = await request<{ data: { skills: string[] } }>('/api/v1/me/resume/extract', init);
    return res.data.skills;
  }

  /** The résumé verdict for a profile: the deterministic market comparison plus any
   *  previously stored coherence/advice. Owner-scoped (404 for another user's profile). */
  async function getProfileVerdict(id: number): Promise<Verdict> {
    const res = await request<{ data: Verdict }>(`/api/v1/me/profiles/${id}/verdict`);
    return res.data;
  }

  /** Analyze a résumé (a PDF `File` sent as multipart, or pasted text sent as JSON)
   *  against a profile: the server runs the LLM coherence/advice over the text (never
   *  persisting it) and returns the full verdict with coherence attached. */
  async function analyzeProfileResume(id: number, input: File | string): Promise<Verdict> {
    let init: RequestInit;
    if (typeof input === 'string') {
      init = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: input }),
      };
    } else {
      const form = new FormData();
      form.append('file', input);
      init = { method: 'POST', body: form };
    }
    const res = await request<{ data: Verdict }>(`/api/v1/me/profiles/${id}/verdict`, init);
    return res.data;
  }

  /** The caller's notification subscriptions (one per saved search + channel). */
  async function listSubscriptions(): Promise<Subscription[]> {
    const res = await request<{ data: Subscription[] }>('/api/v1/me/subscriptions');
    return res.data;
  }

  /** Subscribe a saved search to a channel (telegram by default). A duplicate is a 409. */
  async function createSubscription(savedSearchId: number, channel = 'telegram'): Promise<Subscription> {
    const res = await request<{ data: Subscription }>('/api/v1/me/subscriptions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ saved_search_id: savedSearchId, channel }),
    });
    return res.data;
  }

  /** Pause or resume a subscription. */
  async function setSubscriptionActive(id: number, active: boolean): Promise<Subscription> {
    const res = await request<{ data: Subscription }>(`/api/v1/me/subscriptions/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ active }),
    });
    return res.data;
  }

  /** Unsubscribe by subscription id. */
  async function deleteSubscription(id: number): Promise<void> {
    await call(`/api/v1/me/subscriptions/${id}`, { method: 'DELETE' });
  }

  /** Whether Telegram notifications are configured and whether this user is linked. */
  async function telegramStatus(): Promise<TelegramStatus> {
    const res = await request<{ data: TelegramStatus }>('/api/v1/me/telegram');
    return res.data;
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
    const res = await request<{ data: Submission }>('/api/v1/submissions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    return res.data;
  }

  /** The caller's own submissions with their review status. */
  async function listMySubmissions(): Promise<Submission[]> {
    const res = await request<{ data: Submission[] }>('/api/v1/me/submissions');
    return res.data;
  }

  /** The moderator review queue: pending submissions, with submitter emails. */
  async function listPendingSubmissions(): Promise<Submission[]> {
    const res = await request<{ data: Submission[] }>('/api/v1/submissions');
    return res.data;
  }

  /** Approve a pending submission; the server mints a live job from it. */
  async function approveSubmission(id: number): Promise<Submission> {
    const res = await request<{ data: Submission }>(`/api/v1/submissions/${id}/approve`, {
      method: 'POST',
    });
    return res.data;
  }

  /** Reject a pending submission with an optional reason. */
  async function rejectSubmission(id: number, reason?: string): Promise<Submission> {
    const res = await request<{ data: Submission }>(`/api/v1/submissions/${id}/reject`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason: reason ?? '' }),
    });
    return res.data;
  }

  /** Report a problem with a live vacancy (by slug). Returns the pending report. */
  async function reportJob(slug: string, input: ReportInput): Promise<Report> {
    const res = await request<{ data: Report }>(`/api/v1/jobs/${slug}/reports`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    return res.data;
  }

  /** The moderator review queue: pending reports, with reporter email and job fields. */
  async function listPendingReports(): Promise<Report[]> {
    const res = await request<{ data: Report[] }>('/api/v1/reports');
    return res.data;
  }

  /** Resolve a pending report; optionally soft-close the reported job. */
  async function resolveReport(id: number, closeJob: boolean): Promise<Report> {
    const res = await request<{ data: Report }>(`/api/v1/reports/${id}/resolve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ close_job: closeJob }),
    });
    return res.data;
  }

  /** Dismiss a pending report with an optional reason; the job is unchanged. */
  async function dismissReport(id: number, reason?: string): Promise<Report> {
    const res = await request<{ data: Report }>(`/api/v1/reports/${id}/dismiss`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason: reason ?? '' }),
    });
    return res.data;
  }

  return {
    listJobs,
    getJob,
    getSimilarJobs,
    searchJobs,
    swipeDeck,
    facetCounts,
    listCompanies,
    getCompany,
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
    listViewedSlugs,
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
    listSearchProfiles,
    createSearchProfile,
    updateSearchProfile,
    deleteSearchProfile,
    extractResumeSkills,
    getProfileVerdict,
    analyzeProfileResume,
    listSubscriptions,
    createSubscription,
    setSubscriptionActive,
    deleteSubscription,
    telegramStatus,
    telegramLink,
    telegramUnlink,
    submitJob,
    listMySubmissions,
    listPendingSubmissions,
    approveSubmission,
    rejectSubmission,
    reportJob,
    listPendingReports,
    resolveReport,
    dismissReport,
  };
}

export type MyJobsFilter = 'all' | 'viewed' | 'saved' | 'applied' | 'board';

/** The default browser client: global fetch, same-origin, cookie attached. */
export type Api = ReturnType<typeof createApi>;
export const api = createApi();

// Named exports of the default browser client, for ergonomic imports in client
// components (e.g. `import { recordJobView } from '$lib/api'`). Server `load`
// code uses `serverApi(event.fetch)` instead. Safe to detach because the methods
// close over `fetchImpl`, not `this`.
export const {
  listJobs,
  getJob,
  getSimilarJobs,
  searchJobs,
  swipeDeck,
  facetCounts,
  listCompanies,
  getCompany,
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
  listViewedSlugs,
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
  listSearchProfiles,
  createSearchProfile,
  updateSearchProfile,
  deleteSearchProfile,
  extractResumeSkills,
  getProfileVerdict,
  analyzeProfileResume,
  listSubscriptions,
  createSubscription,
  setSubscriptionActive,
  deleteSubscription,
  telegramStatus,
  telegramLink,
  telegramUnlink,
  submitJob,
  listMySubmissions,
  listPendingSubmissions,
  approveSubmission,
  rejectSubmission,
  reportJob,
  listPendingReports,
  resolveReport,
  dismissReport,
} = api;
