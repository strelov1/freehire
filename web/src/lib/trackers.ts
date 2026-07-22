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

/** Start trackers only when consent currently allows it: the visitor is not
 *  consent-required, or has already granted consent. */
export function startTrackersIfAllowed(): void {
  if (trackersAllowed()) startTrackers();
}
