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
  User,
  UserJob,
  ApiKey,
  CreatedApiKey,
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
  async function listCompanies(q: string, limit: number, offset: number): Promise<Slice<CompanyListItem>> {
    const params = new URLSearchParams();
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
    action: 'view' | 'apply' | 'save' | 'stage' | 'track',
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

  return {
    listJobs,
    getJob,
    searchJobs,
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
    clearJobStage,
    untrackJob,
    trackJob,
    listMyJobs,
    listViewedSlugs,
    listApiKeys,
    createApiKey,
    revokeApiKey,
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
  searchJobs,
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
  clearJobStage,
  untrackJob,
  trackJob,
  listMyJobs,
  listViewedSlugs,
  listApiKeys,
  createApiKey,
  revokeApiKey,
} = api;
