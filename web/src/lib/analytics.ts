// PostHog product analytics — a thin, guarded wrapper so call sites never touch
// the SDK directly and every entry point is a safe no-op when PostHog is not
// initialized (no key in dev, or SSR). Mirrors the Sentry env-gating posture:
// without PUBLIC_POSTHOG_KEY nothing initializes and no events are sent.
//
// Only the pure/guarded surface (isPrivateRoute, track no-op, isFeatureEnabled
// fallback) is unit-tested; the SDK side effects (identify/reset/replay) are
// exercised via visual verification, per the frontend testing convention.
//
// The SDK (~40KB gzip) is dynamically imported inside initAnalytics, so it only
// downloads when a key is actually configured — it never weighs down the entry
// chunk for visitors where analytics is inert (no key, or SSR). Same lazy posture
// as shiki/easymde elsewhere.
import type { PostHog } from 'posthog-js';

export interface AnalyticsConfig {
  /** PostHog project key; empty/absent leaves analytics inert. */
  key: string;
  /** Same-origin reverse-proxy path events are sent through (e.g. `/ingest`). */
  apiHost: string;
}

// Null until the dynamic import resolves and init runs; every entry point below
// guards on it, so calls before/without load are safe no-ops. `loading` coalesces
// concurrent init calls.
let ph: PostHog | null = null;
let loading = false;

// The runtime env read and browser guard live in the caller (hooks.client.ts,
// which is client-only) so this module stays free of SvelteKit runtime imports
// and unit-testable in a plain node environment.

/** Load and initialize PostHog once, only when a key is configured. Best-effort:
 *  a failed dynamic import must never break the app, so the SDK download is fired
 *  and forgotten with errors swallowed. */
export function initAnalytics(config: AnalyticsConfig): void {
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
