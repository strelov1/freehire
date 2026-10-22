import { defineEnvVars } from '@sveltejs/kit/env';

// Every environment variable the app reads. SvelteKit 3 exposes ONLY what is declared
// here — `$app/env/public` and `$app/env/private` carry nothing else, and the legacy
// `$env/*` modules re-export the same set — so a variable missing from this list is
// undefined at runtime however it is set on the host, and nothing reports it: Sentry or
// PostHog simply stop initialising.
//
// A declared variable with no schema is REQUIRED: the server refuses to start without
// it. All of these are optional today (each reader has its own fallback), so each
// carries a pass-through schema that admits `undefined`.
const optional = (value: string | undefined) => value;

export const variables = defineEnvVars({
  PUBLIC_SENTRY_DSN: { public: true, schema: optional },
  PUBLIC_SENTRY_ENVIRONMENT: { public: true, schema: optional },
  PUBLIC_POSTHOG_KEY: { public: true, schema: optional },
  PUBLIC_POSTHOG_HOST: { public: true, schema: optional },
  PUBLIC_OPEN_WITHIN: { public: true, schema: optional },
  API_INTERNAL_URL: { schema: optional },
  SSR_API_TIMEOUT_MS: { schema: optional },
});
