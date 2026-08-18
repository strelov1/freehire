import { env } from '$env/dynamic/public';
import { sequence } from '@sveltejs/kit/hooks';
import type { Handle } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { hasSessionCookie } from '$lib/authCookie';
import { cachePolicy } from '$lib/httpCache';
import { LOCALE_COOKIE } from '$lib/locale';

// Resolves the account-section locale for `<html lang>` before the response
// streams. Path-gated: only `/my/**` may render non-English — every other route
// is forced to `en` here, so "public pages are never translated" is a structural
// property of this one hook rather than a convention each page has to remember.
//
// `'ru'` is the only value that renders as anything but English — matches
// `t()`'s own fallback rule exactly (any account preference that isn't `ru`,
// including a valid-but-untranslated one like `es`, renders English content),
// so the attribute never disagrees with what's actually on the page.
//
// Seeds `event.locals.locale` from the cookie synchronously (no DB/network) as
// a best-effort guess, but `transformPageChunk` reads it lazily rather than
// capturing it here — `event.locals` is the same object for the whole request,
// and the root `+layout.server.ts` load (which runs during `resolve()`, before
// any HTML streams) overwrites it with the fresh, authoritative value once it
// resolves the account's real `users.language`. That closes the one gap a
// cookie-only guess can't: the very first request of a session, before any
// cookie exists.
const locale: Handle = async ({ event, resolve }) => {
  const onAccountSection = event.url.pathname === '/my' || event.url.pathname.startsWith('/my/');
  event.locals.locale = onAccountSection && event.cookies.get(LOCALE_COOKIE) === 'ru' ? 'ru' : 'en';
  return resolve(event, {
    transformPageChunk: ({ html }) => html.replace('%lang%', event.locals.locale),
  });
};

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
  // HEAD too: it is the same read, and RFC 9110 requires its headers to match what
  // GET would return — a crawler or monitor probing with HEAD must see the same
  // cache policy, not none at all.
  if (event.request.method !== 'GET' && event.request.method !== 'HEAD') return response;
  if (response.headers.has('cache-control')) return response;
  if (!response.headers.get('content-type')?.includes('text/html')) return response;

  // The status is passed because an error page is HTML like any other and was
  // otherwise handed the same shared-cache lifetime as the page it replaced — see
  // NO_CACHE in $lib/httpCache for what that cost in production.
  response.headers.set(
    'cache-control',
    cachePolicy({
      pathname: event.url.pathname,
      authenticated: hasSessionCookie(event.request.headers.get('cookie')),
      status: response.status,
    }),
  );
  // The response body differs by session, so a shared cache must key on it. Without
  // this a CDN could hand an anonymous copy to a signed-in visitor (and, worse, the
  // reverse) whenever a rule of its own decides to store the response anyway.
  response.headers.append('vary', 'Cookie');
  return response;
};

// Narrow what SvelteKit advertises in the response's `Link` header. The default
// policy (`type === 'js' || type === 'css'`) walks the whole module graph of the
// matched route, and this app's graph is wide: a route resolves to ~85 chunks,
// most of them under 1.5KB. Every one of them became a `rel=modulepreload` the
// browser started fetching before it had parsed a byte of HTML.
//
// On a fast connection that is free. On a throttled one it is the dominant cost:
// ~85 highest-priority streams share the link with the single render-blocking
// stylesheet, and Lighthouse (mobile, Slow 4G) measured LCP 6.2s on `/` and 7.0s
// on a job page with 89-93% of it spent in render delay — not in TTFB (~0.5-0.7s),
// and with no image or web font involved anywhere in the LCP.
//
// So: keep the stylesheet, keep the two entry modules (they are on the critical
// path either way and are what the inline bootstrap immediately imports), and let
// the rest be discovered through their own import statements. That trades a wide
// burst for a slightly deeper waterfall, which is the right way round when the
// burst is what delays first paint.
//
// Position in `sequence()` is not arbitrary: unlike `transformPageChunk`, whose
// hooks are merged, the FIRST `preload` wins and later ones are ignored. Nothing
// else sets one today — a second one added below this line would be dead code.
const preloadPolicy: Handle = async ({ event, resolve }) =>
  resolve(event, {
    preload: ({ type, path }) => type === 'css' || path.includes('immutable/entry/'),
  });

// sentryHandle scopes each SSR request; it is a passthrough when init was skipped.
export const handle = sequence(Sentry.sentryHandle(), preloadPolicy, locale, cacheControl);

// Reports uncaught SSR errors to Sentry; inert when init was skipped above.
export const handleError = Sentry.handleErrorWithSentry();
