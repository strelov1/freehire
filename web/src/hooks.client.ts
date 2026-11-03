import { PUBLIC_OTEL_LOGS_ENDPOINT, PUBLIC_OTEL_ENVIRONMENT, PUBLIC_OTEL_SERVICE_VERSION } from '$app/env/public';
import { createErrorTelemetry, installErrorListeners } from '#lib/errorTelemetry.js';
import { startTrackersIfAllowed } from '#lib/trackers.js';

// Non-essential trackers (PostHog + Google Analytics) are gated on cookie consent:
// this starts them at boot only for visitors who are not consent-required or who
// have already granted consent. Consent-required visitors with no choice start
// nothing until they Accept in the banner (which calls startTrackers itself).
// Still env-gated end to end — inert when PUBLIC_POSTHOG_KEY is absent.
startTrackersIfAllowed();

// Reports uncaught client-side errors to the OTLP endpoint; inert without one.
const telemetry = createErrorTelemetry({ endpoint: PUBLIC_OTEL_LOGS_ENDPOINT, service: 'freehire-web', environment: PUBLIC_OTEL_ENVIRONMENT, version: PUBLIC_OTEL_SERVICE_VERSION });
if (telemetry) {
  installErrorListeners(window, telemetry);
  // The SDK flushes on document hide too; pagehide covers navigation/bfcache.
  window.addEventListener('pagehide', () => { void telemetry.flush(); });
}
export const handleError = ({ error }: { error: unknown }) => { telemetry?.capture(error); };
