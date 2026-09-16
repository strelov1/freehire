import { env } from '$env/dynamic/public';
import * as Sentry from '@sentry/sveltekit';
import { isTransientNoise } from '$lib/sentryNoise';
import { startTrackersIfAllowed } from '$lib/trackers';

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
    // A Safari extension reads our JSON-LD and throws on a block that carries no
    // "@context" — inline, so the frame points at our own document and neither
    // denyUrls nor the third-party filter sees it as foreign. Nothing on the
    // client reads "@context" (seo.ts only serializes it server-side), so the
    // key alone identifies the injected script. Errors-only on a 5k/month quota
    // means noise like this is paid for in dropped real errors.
    ignoreErrors: [/\[['"]@context['"]\]\.toLowerCase/],
    // A condition is not a defect — see $lib/sentryNoise for the rule and what ignoring it
    // cost. `ignoreErrors` above cannot serve: it matches the message only, and two of the
    // conditions (an ApiError's status, an AbortError's name) are not in one.
    beforeSend: (event, hint) => (isTransientNoise(hint?.originalException) ? null : event),
  });
}

// Non-essential trackers (PostHog + Google Analytics) are gated on cookie consent:
// this starts them at boot only for visitors who are not consent-required or who
// have already granted consent. Consent-required visitors with no choice start
// nothing until they Accept in the banner (which calls startTrackers itself).
// Still env-gated end to end — inert when PUBLIC_POSTHOG_KEY is absent.
startTrackersIfAllowed();

// Reports uncaught client-side errors to Sentry; inert when init was skipped above.
export const handleError = Sentry.handleErrorWithSentry();
