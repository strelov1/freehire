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

import { ApiError, type Http, MovedError, openCoverLetterStream, toApiError } from './api-core';
import { jobsApi } from './api-methods/jobs';
import { companiesApi } from './api-methods/companies';
import { mentorsApi } from './api-methods/mentors';
import { insightsApi } from './api-methods/insights';
import { authApi } from './api-methods/auth';
import { trackingApi } from './api-methods/tracking';
import { accountApi } from './api-methods/account';
import { profileApi } from './api-methods/profile';
import { submissionsApi } from './api-methods/submissions';
import { referralsApi } from './api-methods/referrals';
import { buddiesApi } from './api-methods/buddies';
import { messagingApi } from './api-methods/messaging';
import { moderationApi } from './api-methods/moderation';
import { employerApi } from './api-methods/employer';
import { reportsApi } from './api-methods/reports';
import { mailApi } from './api-methods/mail';
import { cvApi } from './api-methods/cv';
import { threadsApi } from './api-methods/threads';
import { feedbackApi } from './api-methods/feedback';
import { emailPrefsApi } from './api-methods/email-prefs';
import { onboardingApi } from './api-methods/onboarding';

export type {
  Slice,
  EmailPrefs,
  InsightRoleCountryCell,
  InsightRoleCountries,
  InsightRolePulse,
  InsightRole,
  InsightSkill,
  InsightSalaryMonth,
  InsightSalaryBand,
  InsightCompany,
  SkillPulsePoint,
  SkillPulse,
  JobCopy,
  RoastResponse,
  GmailStatus,
  MailboxStatus,
  InboxSource,
  InboxMessage,
  EmailBody,
} from './api-types';
export {
  RESUME_MAX_MB,
  PHOTO_MAX_MB,
  PHOTO_ACCEPT,
  ApiError,
  isPlanRefusal,
  isCrossSiteRefusal,
  MovedError,
  trackPlanRefusal,
} from './api-core';

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
   *  SERVER client only (see `#lib/server/api`): a `load` that never finishes holds
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
    const outgoing: RequestInit = {
      credentials: 'include',
      ...init,
      headers: { ...defaultHeaders, ...init?.headers },
    };
    // Assigned after the spread, so a timeout is added to the init rather than
    // substituted for it. A caller that brought its own signal keeps it.
    //
    // `== null` is LOAD-BEARING, not a loose-equality slip: the abortable reads
    // (searchJobs/suggest/listCompanies) pass `{ signal }` unconditionally, so a caller
    // that named no signal arrives here with `signal: undefined` rather than with no
    // `signal` key at all. Tighten this to `===` and those three quietly stop getting
    // the SSR timeout above — which is the ~8 `+page.server.ts` callers, and exactly the
    // failure that comment describes.
    const timeout = timeoutMs != null && outgoing.signal == null ? AbortSignal.timeout(timeoutMs) : undefined;
    if (timeout) {
      outgoing.signal = timeout;
    }

    let res: Response;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, outgoing);
    } catch (err) {
      // Report the timeout as the gateway timeout it is, so a `load` renders the
      // error page for a slow backend instead of surfacing a bare DOMException.
      if (timeout?.aborted) {
        throw new ApiError(504, `The API did not answer within ${timeoutMs}ms: ${path}`);
      }
      throw err;
    }
    // A caller that asked for `redirect: 'manual'` wants the move itself, not the resource
    // behind it. Translating it here keeps the same shape as the non-ok translation below.
    if (res.status === 301 || res.status === 308) {
      const location = res.headers.get('location') ?? '';
      const [pathname = ''] = location.split('?');
      const slug = pathname.replace(/\/+$/, '').split('/').pop() ?? '';
      throw new MovedError(decodeURIComponent(slug));
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

  const http: Http = { call, request, requestData, baseUrl };
  return {
    ...jobsApi(http),
    ...companiesApi(http),
    ...mentorsApi(http),
    ...insightsApi(http),
    ...authApi(http),
    ...trackingApi(http),
    ...accountApi(http),
    ...profileApi(http),
    ...submissionsApi(http),
    ...referralsApi(http),
    ...buddiesApi(http),
    ...messagingApi(http),
    ...moderationApi(http),
    ...employerApi(http),
    ...reportsApi(http),
    ...mailApi(http),
    ...cvApi(http),
    ...threadsApi(http),
    ...feedbackApi(http),
    ...emailPrefsApi(http),
    ...onboardingApi(http),
    openCoverLetterStream,
  };
}

/** The default browser client: global fetch, same-origin, cookie attached. Client
 *  components call methods on it (`api.foo()`); server `load` uses `serverApi(event.fetch)`. */
export const api = createApi();
