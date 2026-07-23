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

// Null until the dynamic import resolves and init runs; every entry point below
// guards on it, so calls before/without load are safe no-ops. `loading` coalesces
// concurrent init calls.
let ph: PostHog | null = null;
let loading = false;

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
  if (ph || loading || !config.key) return;
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
function gtag(...args: unknown[]): void {
  (window.dataLayer ??= []).push(args);
}

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

/** Capture an explicit funnel event. No-op until the SDK has loaded. */
export function track(event: string, props?: Record<string, unknown>): void {
  ph?.capture(event, props);
}

/** Bind analytics identity to a signed-in user by id only — never PII. */
export function identifyUser(user: { id: number }): void {
  ph?.identify(String(user.id));
}

/** Drop identity so subsequent events are anonymous (on sign-out). */
export function resetIdentity(): void {
  ph?.reset();
}

/** Start or stop session recording based on route privacy. */
export function syncReplayForRoute(path: string): void {
  if (!ph) return;
  if (isPrivateRoute(path)) ph.stopSessionRecording();
  else ph.startSessionRecording();
}

/** Capture a pageview for the current SPA route. */
export function capturePageview(): void {
  ph?.capture('$pageview');
}

/** Generic feature-flag reader: the flag's value when loaded, else the fallback.
 *  Wiring a concrete product default to a flag is left to the caller. */
export function isFeatureEnabled(flag: string, fallback: boolean): boolean {
  if (!ph) return fallback;
  const value = ph.isFeatureEnabled(flag);
  return value === undefined ? fallback : value;
}
