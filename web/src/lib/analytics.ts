// PostHog product analytics — a thin, guarded wrapper so call sites never touch
// the SDK directly and every entry point is a safe no-op when PostHog is not
// initialized (no key in dev, or SSR). Mirrors the Sentry env-gating posture:
// without PUBLIC_POSTHOG_KEY nothing initializes and no events are sent.
//
// Only the pure/guarded surface (isPrivateRoute, track no-op, isFeatureEnabled
// fallback) is unit-tested; the SDK side effects (identify/reset/replay) are
// exercised via visual verification, per the frontend testing convention.
import posthog from 'posthog-js';

export interface AnalyticsConfig {
  /** PostHog project key; empty/absent leaves analytics inert. */
  key: string;
  /** Same-origin reverse-proxy path events are sent through (e.g. `/ingest`). */
  apiHost: string;
}

let initialized = false;

// The runtime env read and browser guard live in the caller (hooks.client.ts,
// which is client-only) so this module stays free of SvelteKit runtime imports
// and unit-testable in a plain node environment.

/** Initialize PostHog once, only when a key is configured. */
export function initAnalytics(config: AnalyticsConfig): void {
  if (initialized || !config.key) return;
  posthog.init(config.key, {
    api_host: config.apiHost,
    ui_host: 'https://eu.posthog.com',
    capture_pageview: false, // SPA navigation is captured manually (see layout)
    person_profiles: 'identified_only', // no anonymous profiles → saves quota
    session_recording: { maskAllInputs: true },
  });
  initialized = true;
}

/** Routes whose DOM must never be recorded (résumé, tracking, inbox all live
 *  under /my). Session replay is stopped for their duration. */
export function isPrivateRoute(path: string): boolean {
  return path === '/my' || path.startsWith('/my/');
}

/** Capture an explicit funnel event. No-op until initialized. */
export function track(event: string, props?: Record<string, unknown>): void {
  if (!initialized) return;
  posthog.capture(event, props);
}

/** Bind analytics identity to a signed-in user by id only — never PII. */
export function identifyUser(user: { id: number }): void {
  if (!initialized) return;
  posthog.identify(String(user.id));
}

/** Drop identity so subsequent events are anonymous (on sign-out). */
export function resetIdentity(): void {
  if (!initialized) return;
  posthog.reset();
}

/** Start or stop session recording based on route privacy. */
export function syncReplayForRoute(path: string): void {
  if (!initialized) return;
  if (isPrivateRoute(path)) posthog.stopSessionRecording();
  else posthog.startSessionRecording();
}

/** Capture a pageview for the current SPA route. */
export function capturePageview(): void {
  if (!initialized) return;
  posthog.capture('$pageview');
}

/** Generic feature-flag reader: the flag's value when loaded, else the fallback.
 *  Wiring a concrete product default to a flag is left to the caller. */
export function isFeatureEnabled(flag: string, fallback: boolean): boolean {
  if (!initialized) return fallback;
  const value = posthog.isFeatureEnabled(flag);
  return value === undefined ? fallback : value;
}
