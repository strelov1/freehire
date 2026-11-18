import { PUBLIC_OTEL_LOGS_ENDPOINT, PUBLIC_OTEL_ENVIRONMENT, PUBLIC_OTEL_SERVICE_VERSION } from '$app/env/public';
import { createErrorTelemetry, installErrorListeners } from '#lib/errorTelemetry.js';
import { startTrackersIfAllowed } from '#lib/trackers.js';
import { claimModuleReload } from '#lib/moduleReload.js';
import { isModuleLoadFailure } from '#lib/transientNoise.js';
import type { HandleClientError } from '@sveltejs/kit/hooks';

// Non-essential trackers (PostHog + Google Analytics) are gated on cookie consent:
// this starts them at boot only for visitors who are not consent-required or who
// have already granted consent. Consent-required visitors with no choice start
// nothing until they Accept in the banner (which calls startTrackers itself).
// Still env-gated end to end — inert when PUBLIC_POSTHOG_KEY is absent.
startTrackersIfAllowed();

// Reports uncaught client-side errors to the OTLP endpoint; inert without one.
const telemetry = createErrorTelemetry({
  endpoint: PUBLIC_OTEL_LOGS_ENDPOINT,
  service: 'freehire-web',
  environment: PUBLIC_OTEL_ENVIRONMENT,
  version: PUBLIC_OTEL_SERVICE_VERSION,
});
if (telemetry) {
  installErrorListeners(window, telemetry);
  // The SDK flushes on document hide too; pagehide covers navigation/bfcache.
  window.addEventListener('pagehide', () => {
    void telemetry.flush();
  });
}
/** Reports a client error. Every call here ends on the error page, so the visitor gets a
 *  code to quote (shown by +error.svelte, sent as `error.id`) and a failed module load is
 *  reported even though it reads as transient noise. That failure usually means stale
 *  cached markup or chunks, so the page reloads once (claimModuleReload stops a loop)
 *  after the report is flushed. */
export const handleError: HandleClientError = ({ error }) => {
  const id = Math.random().toString(36).slice(2, 10); // a lookup code, not a secret
  const moduleLoad = isModuleLoadFailure(error);
  telemetry?.capture(error, { errorId: id, reportNoise: moduleLoad });
  if (moduleLoad && claimModuleReload(sessionStorageOrUndefined(), location.href, Date.now())) {
    void Promise.resolve(telemetry?.flush()).finally(() => location.reload());
  }
  return { id };
};

/** This tab's sessionStorage, or undefined where merely reading it throws. */
function sessionStorageOrUndefined(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined; // access itself throws when storage is blocked
  }
}
