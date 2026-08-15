import { env } from '$env/dynamic/public';
import { sequence } from '@sveltejs/kit/hooks';
import type { Handle } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { hasSessionCookie } from '$lib/authCookie';
import { cachePolicy } from '$lib/httpCache';

// Same opt-in, env-gated init as the client: no PUBLIC_SENTRY_DSN ⇒ no init, and
// SSR runs unchanged. Errors-only, PII off. The server reports to the same
// (frontend) Sentry project as the browser.
if (env.PUBLIC_SENTRY_DSN) {
  Sentry.init({
    dsn: env.PUBLIC_SENTRY_DSN,
    environment: env.PUBLIC_SENTRY_ENVIRONMENT || 'development',
    tracesSampleRate: 0,
    sendDefaultPii: false,
  });
}

// Label server-rendered HTML for shared caches (a CDN, or nginx's own cache):
// anonymous public pages may be held and replayed, anything tied to a person may
// not. See $lib/httpCache for why that decision lives in the app rather than only
// in a CDN rule.
//
// Only GET HTML is labelled. A route that set its own Cache-Control is left alone
// — the sitemaps do this, and their hour-long TTL is a considered choice, not one
// to overwrite. Non-HTML responses (the OG image endpoints, robots.txt) carry
// their own semantics and are none of this hook's business.
const cacheControl: Handle = async ({ event, resolve }) => {
  const response = await resolve(event);
  if (event.request.method !== 'GET') return response;
  if (response.headers.has('cache-control')) return response;
  if (!response.headers.get('content-type')?.includes('text/html')) return response;

  response.headers.set(
    'cache-control',
    cachePolicy({
      pathname: event.url.pathname,
      authenticated: hasSessionCookie(event.request.headers.get('cookie')),
    }),
  );
  // The response body differs by session, so a shared cache must key on it. Without
  // this a CDN could hand an anonymous copy to a signed-in visitor (and, worse, the
  // reverse) whenever a rule of its own decides to store the response anyway.
  response.headers.append('vary', 'Cookie');
  return response;
};

// sentryHandle scopes each SSR request; it is a passthrough when init was skipped.
export const handle = sequence(Sentry.sentryHandle(), cacheControl);

// Reports uncaught SSR errors to Sentry; inert when init was skipped above.
export const handleError = Sentry.handleErrorWithSentry();
