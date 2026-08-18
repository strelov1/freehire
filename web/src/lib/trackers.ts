// Client orchestration seam between env config, consent state, and the analytics
// wrapper. Kept out of analytics.ts so that module stays free of SvelteKit runtime
// imports and unit-testable; this file is client-only and verified via the banner
// visual pass. Both the boot hook and the banner's Accept action route through here
// so the PostHog config is assembled in exactly one place.
import { env } from '$env/dynamic/public';
import { type AnalyticsConfig, initTrackers } from './analytics';
import { trackersAllowed } from './consent.svelte';

function config(): AnalyticsConfig {
  return {
    key: env.PUBLIC_POSTHOG_KEY ?? '',
    apiHost: env.PUBLIC_POSTHOG_HOST || '/ingest',
  };
}

/** Start every non-essential tracker with the app's env config. Called on Accept
 *  and, at boot, only when consent already allows it. */
export function startTrackers(): void {
  initTrackers(config());
}

// How long the boot start may wait for an idle moment before going anyway. The
// trackers weigh roughly 300KB between them (gtag.js ~168KB, the PostHog SDK plus
// its recorder and surveys chunks the rest), and at boot they competed for
// bandwidth with the render-blocking stylesheet and the route's own modules —
// measurable in LCP, which is 6-7s on mobile. Yielding until the browser is idle
// takes them out of that contention.
//
// The timeout is what keeps this a deferral rather than a gamble: a page that
// never goes idle still starts them, just late. Consent has already been checked
// by the time we get here, so waiting cannot turn into consent-less tracking.
const BOOT_IDLE_TIMEOUT_MS = 3000;

/** Start trackers only when consent currently allows it: the visitor is not
 *  consent-required, or has already granted consent.
 *
 *  Deferred to the first idle moment. Calls made in the meantime are not lost —
 *  $lib/analytics queues them and replays them in order once the SDK is up. */
export function startTrackersIfAllowed(): void {
  if (!trackersAllowed()) return;
  // requestIdleCallback is unsupported on older Safari; a plain timer is the
  // same deferral without the idle heuristic.
  if (typeof requestIdleCallback === 'function') {
    requestIdleCallback(startTrackers, { timeout: BOOT_IDLE_TIMEOUT_MS });
  } else {
    setTimeout(startTrackers, BOOT_IDLE_TIMEOUT_MS);
  }
}
