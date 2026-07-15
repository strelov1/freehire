import { env } from '$env/dynamic/public';
import * as Sentry from '@sentry/sveltekit';
import { initAnalytics } from '$lib/analytics';

// Error tracking is opt-in and env-gated: without PUBLIC_SENTRY_DSN Sentry stays
// uninitialized and the app runs unchanged (mirrors the backend integration).
// Errors-only — no performance tracing, no session replay; sendDefaultPii is off
// so URLs/inputs/headers are not shipped.
if (env.PUBLIC_SENTRY_DSN) {
  Sentry.init({
    dsn: env.PUBLIC_SENTRY_DSN,
    environment: env.PUBLIC_SENTRY_ENVIRONMENT || 'development',
    tracesSampleRate: 0,
    sendDefaultPii: false,
  });
}

// Product analytics is likewise opt-in and env-gated. This hook is client-only,
// so no browser guard is needed; ingestion goes through the same-origin /ingest
// reverse proxy (overridable via PUBLIC_POSTHOG_HOST). initAnalytics is inert
// when the key is absent.
initAnalytics({
  key: env.PUBLIC_POSTHOG_KEY ?? '',
  apiHost: env.PUBLIC_POSTHOG_HOST || '/ingest',
});

// Reports uncaught client-side errors to Sentry; inert when init was skipped above.
export const handleError = Sentry.handleErrorWithSentry();
