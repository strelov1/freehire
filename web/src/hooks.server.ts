import { env } from '$env/dynamic/public';
import { sequence } from '@sveltejs/kit/hooks';
import type { Handle } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { hasSessionCookie } from '$lib/authCookie';
import { cachePolicy } from '$lib/httpCache';
import { isTranslatedLocale, LOCALE_COOKIE } from '$lib/locale';
import { CAPTURE_MAX_AGE_SECONDS, REF_COOKIE, captureRef, capturePromo } from '$lib/referral';

// Resolves the account-section locale for `<html lang>` before the response
// streams. Path-gated: only `/my/**` may render non-English — every other route
// is forced to `en` here, so "public pages are never translated" is a structural
// property of this one hook rather than a convention each page has to remember.
//
// What may render as anything but English is `TRANSLATED_LOCALES`, not one
// hard-coded literal. The distinction matters in both directions: testing for
// `'ru'` alone would make a Spanish catalog unreachable however complete it was
// (the resolver, not the catalog, decides what a reader sees), while admitting
// every SUPPORTED locale would put `lang="es"` on a page still rendering English
// through `t()`'s fallback. Reading the list keeps `<html lang>` honest about
// what the page is actually written in, which is what this attribute is for.
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
  const cookieLocale = event.cookies.get(LOCALE_COOKIE);
  event.locals.locale =
    onAccountSection && isTranslatedLocale(cookieLocale) ? cookieLocale : 'en';
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

// Capture a referral or promo code arriving in ANY link, not only on a dedicated route.
//
// One hook rather than a page-level check, because the link is pasted into chats and lands
// wherever the sharer happened to be: `/?ref=x`, `/jobs/some-posting?ref=x`, or the short
// `/r/x`. A capture that only worked on the landing page would silently attribute nothing
// for most of the links people actually send.
//
// Set-Cookie from the SERVER and never from script: Safari's tracking prevention caps
// script-written cookies at seven days, and this window is thirty. httpOnly follows for
// free on the invite cookie — nothing in the browser needs to read it, and the OAuth
// callback that does is on the server.
//
// LAST in the sequence, and that position is the whole reason this is safe. `cacheControl`
// above labels anonymous public pages as storable by a shared cache. A response carrying
// Set-Cookie under such a label can be held by a CDN and replayed — handing one visitor's
// referral cookie to everybody who asks for that page. Running after it means the
// `no-store` below overwrites the label on exactly the responses that set a cookie, and
// nothing else changes.
const attribution: Handle = async ({ event, resolve }) => {
  const captures = [
    captureRef(event.url.searchParams.get('ref'), event.cookies.get(REF_COOKIE)),
    capturePromo(event.url.searchParams.get('promo')),
  ].filter((capture) => capture !== null);

  const response = await resolve(event);
  if (captures.length === 0) return response;

  for (const capture of captures) {
    response.headers.append(
      'set-cookie',
      event.cookies.serialize(capture.name, capture.value, {
        path: '/',
        maxAge: CAPTURE_MAX_AGE_SECONDS,
        // Lax, not Strict: the cookie has to survive the top-level GET redirect back from
        // an OAuth provider, which is the majority sign-up path and the one place a value
        // kept anywhere else could not travel.
        sameSite: 'lax',
        // The promo cookie is read by the pricing page to prefill its field, so it is not
        // httpOnly. The invite cookie is read only by the server.
        httpOnly: capture.name === REF_COOKIE,
        secure: event.url.protocol === 'https:',
      }),
    );
  }
  response.headers.set('cache-control', 'no-store');
  return response;
};

// sentryHandle scopes each SSR request; it is a passthrough when init was skipped.
export const handle = sequence(
  Sentry.sentryHandle(),
  preloadPolicy,
  locale,
  cacheControl,
  attribution,
);

// Reports uncaught SSR errors to Sentry; inert when init was skipped above.
export const handleError = Sentry.handleErrorWithSentry();
