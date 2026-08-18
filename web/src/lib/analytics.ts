// Non-essential trackers — a thin, guarded wrapper (PostHog + Google Analytics) so
// call sites never touch the SDKs directly and every entry point is a safe no-op
// when nothing is initialized (no key in dev, or SSR). Mirrors the Sentry env-gating
// posture: without PUBLIC_POSTHOG_KEY PostHog stays inert. Both trackers start only
// through initTrackers, which the caller gates on cookie consent.
//
// Only the pure/guarded surface (isPrivateRoute, track no-op, isFeatureEnabled
// fallback) is unit-tested; the SDK side effects (identify/reset/replay, gtag) are
// exercised via visual verification, per the frontend testing convention.
//
// The PostHog SDK (~40KB gzip) is dynamically imported inside initPostHog, so it
// only downloads when a key is actually configured — it never weighs down the entry
// chunk for visitors where analytics is inert (no key, or SSR). Same lazy posture
// as shiki/easymde elsewhere.
import type { PostHog } from 'posthog-js';

export interface AnalyticsConfig {
  /** PostHog project key; empty/absent leaves analytics inert. */
  key: string;
  /** Same-origin reverse-proxy path events are sent through (e.g. `/ingest`). */
  apiHost: string;
}

// Google Analytics measurement ID (public by design). Moved here from the inline
// app.html bootstrap so GA can be gated on consent alongside PostHog; the CSP still
// allow-lists the googletagmanager.com host it injects.
const GA_MEASUREMENT_ID = 'G-6P1PZ719T0';

declare global {
  interface Window {
    dataLayer?: unknown[];
    gtag?: (...args: unknown[]) => void;
  }
}

// Null until the dynamic import resolves and init runs. `loading` coalesces
// concurrent init calls.
let ph: PostHog | null = null;
let loading = false;

// Calls made before the SDK resolves are held here and replayed, in order, once it
// does. Without this they were silently dropped, and the very first pageview of
// every visit is exactly such a call: `afterNavigate` fires on the initial load too
// (see +layout.svelte), which is well before a dynamically imported ~40KB SDK can
// have finished loading. That race was always lost more often than won; deferring
// init to idle would have made losing it the rule.
//
// Replay makes the ordering load-bearing rather than merely nice: `syncReplayForRoute`
// carries privacy state, not an event. Dropping its `stopSessionRecording()` for
// /my/** and then starting the SDK would begin recording a CV page — the one thing
// that call exists to prevent. Queued in order, the stop still lands first.
//
// Null once the queue has been handed over (or abandoned), which also makes
// "already settled" and "still waiting" distinguishable at the push site.
let pending: Array<(sdk: PostHog) => void> | null = [];

// A visitor who never grants consent, or a deployment with no key, never drains the
// queue — so it is bounded. Deep enough for a browsing session's worth of pageviews,
// shallow enough that abandoning it costs nothing. Oldest entries are kept: the first
// pageview and the initial replay decision are the ones worth having.
const PENDING_LIMIT = 50;

/** Run `fn` against the SDK now, or as soon as it has loaded. A no-op forever once
 *  the queue has been abandoned (no key configured). */
function withPostHog(fn: (sdk: PostHog) => void): void {
  if (ph) {
    fn(ph);
    return;
  }
  if (pending && pending.length < PENDING_LIMIT) pending.push(fn);
}

/** Hand the queued calls to the freshly initialized SDK, in the order they were made. */
function drainPending(sdk: PostHog): void {
  const queued = pending;
  pending = null;
  queued?.forEach((fn) => fn(sdk));
}

// The runtime env read and browser guard live in the caller (hooks.client.ts,
// which is client-only) so this module stays free of SvelteKit runtime imports
// and unit-testable in a plain node environment.

/** Start every non-essential tracker: PostHog (when a key is configured) and
 *  Google Analytics. Idempotent, and the single entry point gated on consent by
 *  the caller — nothing here runs until consent allows it. */
export function initTrackers(config: AnalyticsConfig): void {
  initPostHog(config);
  initGoogleAnalytics();
}

/** Load and initialize PostHog once, only when a key is configured. Best-effort:
 *  a failed dynamic import must never break the app, so the SDK download is fired
 *  and forgotten with errors swallowed. */
function initPostHog(config: AnalyticsConfig): void {
  if (ph || loading) return;
  if (!config.key) {
    // Inert deployment (no key): nothing will ever drain the queue, so stop
    // holding closures for an SDK that is not coming.
    pending = null;
    return;
  }
  loading = true;
  void import('posthog-js')
    .then(({ default: posthog }) => {
      posthog.init(config.key, {
        api_host: config.apiHost,
        ui_host: 'https://eu.posthog.com',
        capture_pageview: false, // SPA navigation is captured manually (see layout)
        person_profiles: 'identified_only', // no anonymous profiles → saves quota
        session_recording: { maskAllInputs: true },
      });
      ph = posthog;
      drainPending(posthog);
    })
    .catch(() => {
      loading = false; // let a later call retry the load
    });
}

// True once GA has been injected, so a repeated initTrackers() (e.g. Accept after a
// re-open) does not load gtag.js twice.
let gaLoaded = false;

// Push a command onto GA's dataLayer, creating it on first use. At module scope (it
// captures no locals) so it isn't recreated on each init.
//
// WARNING: pushing `arguments` is load-bearing, not legacy style — it is the wire
// format gtag.js expects. It reads a dataLayer entry as a command only when the
// entry is an Arguments object; a plain Array (what rest parameters give) is taken
// for a GTM-style push and silently dropped, so gtag.js loads and registers the
// container but never sends a hit. Hence the untyped implementation behind an
// explicit call signature: callers keep `(...args: unknown[]) => void`.
const gtag: (...args: unknown[]) => void = function () {
  // eslint-disable-next-line prefer-rest-params
  (window.dataLayer ??= []).push(arguments);
};

/** Inject gtag.js and configure GA once. Skipped on localhost so dev traffic stays
 *  out of the property — matching the old app.html bootstrap. */
function initGoogleAnalytics(): void {
  if (gaLoaded) return;
  if (/^(localhost|127\.0\.0\.1)$/.test(location.hostname)) return;
  gaLoaded = true;
  const script = document.createElement('script');
  script.async = true;
  script.src = `https://www.googletagmanager.com/gtag/js?id=${GA_MEASUREMENT_ID}`;
  document.head.appendChild(script);
  window.gtag = gtag;
  gtag('js', new Date());
  gtag('config', GA_MEASUREMENT_ID);
}

/** Routes whose DOM must never be recorded (résumé, tracking, inbox all live
 *  under /my). Session replay is stopped for their duration. */
export function isPrivateRoute(path: string): boolean {
  return path === '/my' || path.startsWith('/my/');
}

/** Why a CV upload was rejected, as a bounded code. `other` is deliberate: an
 *  unrecognized failure still has to reach the funnel, and dropping it would make
 *  the success rate read better than it is. */
export type CvUploadReason = 'no_text' | 'missing_file' | 'unreadable' | 'bad_request' | 'other';

// The server rejects with prose meant for the user, so matching on it directly would
// tie the metric to copy — a reworded sentence would silently split one failure into
// two. Substring matching (not equality) keeps the mapping working when a message
// gains a suffix; the fragments are the stable part of each rejection.
const CV_UPLOAD_REASONS: ReadonlyArray<readonly [string, CvUploadReason]> = [
  ["couldn't read any text from this pdf", 'no_text'],
  ['missing resume file', 'missing_file'],
  ['cannot read resume file', 'unreadable'],
  ['invalid request body', 'bad_request'],
];

/** Map a server rejection message to a bounded reason code for `cv_upload`. */
export function cvUploadReason(message: string): CvUploadReason {
  const text = message.trim().toLowerCase();
  for (const [fragment, reason] of CV_UPLOAD_REASONS) {
    if (text.includes(fragment)) return reason;
  }
  return 'other';
}

/** Whether an account was created just now, within `windowMs` of `now`.
 *
 *  OAuth sign-up is a full-page redirect, so the app comes back holding a session
 *  with no way to tell a first-ever sign-in from the hundredth — a freshly stamped
 *  `created_at` is the only signal the browser has. Creation slightly in the future
 *  counts as fresh: that is clock skew between server and browser, not an old
 *  account. An absent or unparseable stamp is treated as not fresh, so a missing
 *  value can never manufacture a sign-up. */
export function isFreshAccount(createdAt: string | null, now: number, windowMs: number): boolean {
  if (!createdAt) return false;
  const created = Date.parse(createdAt);
  if (Number.isNaN(created)) return false;
  return created > now - windowMs;
}

/** The slice of Storage the sign-up claim needs, so a test can hand it a map. */
type SignupStore = Pick<Storage, 'getItem' | 'setItem'>;

// How recently an account must have been created to read as a sign-up rather than a
// returning sign-in. Wide enough to survive a slow first load and a redirect back
// from an OAuth provider; far short of a session.
const SIGNUP_WINDOW_MS = 2 * 60 * 1000;

/** Claim the single `signup` event for an account: true exactly once, false on
 *  every repeat. A storage failure also returns false — a browser that refuses
 *  storage cannot be deduplicated, and inflating sign-ups is worse than missing
 *  a few. */
export function claimSignupOnce(userId: number, store: SignupStore): boolean {
  const key = `fh:signup:${userId}`;
  try {
    if (store.getItem(key) !== null) return false;
    store.setItem(key, '1');
    return true;
  } catch {
    return false;
  }
}

/** Capture `signup` when identity binds to an account that was just created.
 *
 *  This is the ONLY sign-up detector, and it deliberately covers both routes into
 *  an account. Tracking at the password-registration call site as well would count
 *  that account twice: it arrives here fresh too, just with `has_password` set. */
export function trackSignupIfNew(user: {
  id: number;
  created_at: string | null;
  has_password: boolean;
}): void {
  if (!isFreshAccount(user.created_at, Date.now(), SIGNUP_WINDOW_MS)) return;
  if (typeof localStorage === 'undefined') return;
  if (!claimSignupOnce(user.id, localStorage)) return;
  track('signup', { method: user.has_password ? 'password' : 'oauth' });
}

/** Capture an explicit funnel event. Queued until the SDK has loaded. */
export function track(event: string, props?: Record<string, unknown>): void {
  withPostHog((sdk) => sdk.capture(event, props));
}

/** Bind analytics identity to a signed-in user by id only — never PII. */
export function identifyUser(user: { id: number }): void {
  withPostHog((sdk) => sdk.identify(String(user.id)));
}

/** Drop identity so subsequent events are anonymous (on sign-out). */
export function resetIdentity(): void {
  withPostHog((sdk) => sdk.reset());
}

/** Start or stop session recording based on route privacy. Queued like the rest,
 *  so a /my/** stop decided before the SDK loaded is still applied — and applied
 *  before anything queued after it. */
export function syncReplayForRoute(path: string): void {
  const isPrivate = isPrivateRoute(path);
  withPostHog((sdk) => {
    if (isPrivate) sdk.stopSessionRecording();
    else sdk.startSessionRecording();
  });
}

/** Capture a pageview for the current SPA route. */
export function capturePageview(): void {
  withPostHog((sdk) => sdk.capture('$pageview'));
}

/** Generic feature-flag reader: the flag's value when loaded, else the fallback.
 *  Wiring a concrete product default to a flag is left to the caller. */
export function isFeatureEnabled(flag: string, fallback: boolean): boolean {
  if (!ph) return fallback;
  const value = ph.isFeatureEnabled(flag);
  return value === undefined ? fallback : value;
}
