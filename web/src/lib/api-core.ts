// The API client's module-level helpers: errors, paging, request bodies. Shared by
// createApi (api.ts) and the method groups in api-methods/. Public names are
// re-exported from api.ts.

import { track } from './analytics';
import type { Page, PlanRefusalBody, Slice } from './api-types';

/** Max résumé upload size, mirroring the server's BodyLimit (cmd/server/main.go). The
 *  web client enforces it up front so an oversize PDF gets a clear message instead of the
 *  raw 413 the server would emit before the handler runs. The UI also shows it as a hint. */
export const RESUME_MAX_MB = 8;

export const RESUME_MAX_BYTES = RESUME_MAX_MB * 1024 * 1024;

/** Max headshot upload size. Deliberately UNDER the server's 8 MiB BodyLimit rather than
 *  equal to it: the limit covers the whole multipart request, so a file at exactly 8 MiB is
 *  refused by fasthttp with a bare 413 before any handler — and the member sees an unstyled
 *  error instead of this message. */
export const PHOTO_MAX_MB = 6;

export const PHOTO_MAX_BYTES = PHOTO_MAX_MB * 1024 * 1024;

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

/** Whether a failure is the server's plan refusal (HTTP 402) rather than a fault. Every model
 *  call a person can start is paid-only, so a free caller reaching one gets this — and a
 *  surface that renders it as "unavailable" or "couldn't read" states something false about
 *  the service or the candidate's own file. Callers show the upgrade path instead. */
export function isPlanRefusal(e: unknown): boolean {
  return e instanceof ApiError && e.status === 402;
}

/** Whether a failure is the server's same-origin refusal — HTTP 403 with code
 *  "cross_site_refused" — rather than a fault. The server refuses any sign-in that
 *  names another site in Origin/Referer (or whose own Sec-Fetch-Site says
 *  cross-site) BEFORE it reads the credentials, and the session cookie is NOT
 *  written. That is a condition to communicate ("open freehire and sign in there"),
 *  not an outage to apologize for. The check is on the error CODE, not the status:
 *  a future 403 from an unrelated rule is not an origin refusal. */
export function isCrossSiteRefusal(e: unknown): boolean {
  return e instanceof ApiError && e.status === 403 && e.body?.code === 'cross_site_refused';
}

/** A resource that has permanently moved, carrying the slug it moved TO.
 *
 *  Only raised for a request that opted into `redirect: 'manual'`. Left to the default,
 *  `fetch` follows the 301 and returns the canonical resource with a 200 — which renders
 *  the right page under the WRONG url, the one outcome a permanent redirect exists to
 *  prevent. A caller that cares about the url has to see the 301 itself. */
export class MovedError extends Error {
  constructor(
    /** The final path segment of Location: the canonical slug. */
    public readonly canonicalSlug: string,
  ) {
    super(`moved permanently to ${canonicalSlug}`);
    this.name = 'MovedError';
  }
}

/** Record the moment a plan limit said no.
 *
 *  This is the only record of DEMAND for a paid feature. What a user spent is in
 *  Postgres; what they were refused is nowhere else at all.
 *
 *  `toApiError` calls it for every ordinary JSON response, which covers a metered
 *  feature reached that way without anyone remembering to instrument it. It is EXPORTED
 *  because that is not all of them: the streaming features each own their transport and
 *  read the refusal themselves, so they call this directly —
 *
 *    - the assistant (assistant/client.ts, its own fetch and TurnRefused)
 *    - the cover letter (tailor/CoverLetter.svelte, a raw Response)
 *    - dictation (assistant/speech.ts, its own status ladder)
 *
 *  and one of them cannot: MatchAnalysisFull.svelte opens an EventSource, where the
 *  browser exposes no HTTP status at all and a 402 surfaces only as `onerror`. Counting
 *  that one needs a different transport, not a different call site.
 *
 *  The feature name and whether an upgrade was on offer come from the SERVER's own
 *  refusal body (handler.write402), so neither is guessed from the request path.
 *
 *  No browser guard of its own: toApiError also runs during SSR, and `track` already
 *  handles that — it queues into a bounded buffer that a server process simply never
 *  drains. A second guard here would only make this call behave unlike every other
 *  `track` in the app, and would be untestable in the node environment the unit tests
 *  run in. */
export function trackPlanRefusal(status: number, body: unknown): void {
  if (status !== 402) return;
  const refusal = body as PlanRefusalBody | null;
  const feature = refusal?.allowance?.feature;
  track('plan_refused', {
    feature: typeof feature === 'string' ? feature : 'unknown',
    upgrade_offered: typeof refusal?.upgrade_url === 'string',
  });
}

/** Parse a failed response into an ApiError. The backend's standard error envelope is
 *  `{ "error": msg }`; surface that as the message (falling back to the status line for a
 *  non-JSON error, e.g. a proxy 502) and keep the whole parsed body for callers that need
 *  extra fields. Reads the body once. */
export async function toApiError(res: Response): Promise<ApiError> {
  try {
    const body = await res.json();
    const msg = body && typeof body.error === 'string' ? body.error : `${res.status} ${res.statusText}`;
    trackPlanRefusal(res.status, body);
    return new ApiError(res.status, msg, body ?? undefined);
  } catch {
    return new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
}

export function query(limit: number, offset: number): string {
  return `?limit=${limit}&offset=${offset}`;
}

/** Turn a count-bearing page into a Slice; more remain unless we've reached total. */
export function toSlice<T>(page: Page<T>, offset: number): Slice<T> {
  return {
    items: page.data,
    total: page.meta.total,
    ...(page.meta.total_is_lower_bound ? { totalIsLowerBound: true } : {}),
    hasMore: offset + page.data.length < page.meta.total,
  };
}

/** Build the request init for a JSON body (POST/PATCH/PUT). Single-sources the
 *  Content-Type header and JSON.stringify so no call site repeats them. */
export function jsonBody(method: string, body: unknown): RequestInit {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}

export function insightsQuery(opts: {
  category?: string;
  seniority?: string;
  country?: string;
  sort?: 'open' | 'growth';
  limit?: number;
}): string {
  const q = new URLSearchParams();
  if (opts.category) q.set('category', opts.category);
  if (opts.seniority) q.set('seniority', opts.seniority);
  if (opts.country) q.set('country', opts.country);
  if (opts.sort) q.set('sort', opts.sort);
  if (opts.limit != null) q.set('limit', String(opts.limit));
  return q.toString();
}

/** Build the request init for a résumé payload: pasted text goes as JSON, a `File`
 *  goes as multipart. */
export function resumeInit(method: string, input: File | string): RequestInit {
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

/** Open the streaming draft. Returns the raw Response so the caller can read the body as it
 *  arrives AND see the status: a 402 for an exhausted allowance, a 409 for a vacancy with no
 *  fit analysis. EventSource would hide both behind a bare connection error, and could not
 *  POST — which this must be, because drafting spends an allowance and writes storage. */
export async function openCoverLetterStream(id: string, band: 'short' | 'standard'): Promise<Response> {
  return fetch(`/api/v1/me/cvs/${encodeURIComponent(id)}/cover-letter/stream?band=${band}`, {
    method: 'POST',
    credentials: 'include',
    headers: { accept: 'text/event-stream' },
  });
}

/** What a method group needs from its client: the three request helpers createApi
 *  builds around one fetch, and the base URL for links the browser opens itself. */
export interface Http {
  call(path: string, init?: RequestInit): Promise<Response>;
  request<T>(path: string, init?: RequestInit): Promise<T>;
  requestData<T>(path: string, init?: RequestInit): Promise<T>;
  baseUrl: string;
}
