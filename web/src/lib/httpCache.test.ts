import { describe, expect, it } from 'vitest';
import { NO_CACHE, PRIVATE_CACHE, PUBLIC_CACHE, PUBLIC_DETAIL_CACHE, cachePolicy } from './httpCache';

describe('cachePolicy', () => {
  it('lets a shared cache hold an anonymous public page', () => {
    expect(cachePolicy({ pathname: '/', authenticated: false })).toBe(PUBLIC_CACHE);
    expect(cachePolicy({ pathname: '/collections/python', authenticated: false })).toBe(PUBLIC_CACHE);
  });

  // The whole point of the guard. A signed-in response carries that person's
  // saved jobs, applications and header state; if a CDN stored it under the plain
  // URL, the next visitor would be served someone else's page.
  it('never lets a signed-in response be stored', () => {
    expect(cachePolicy({ pathname: '/companies/acme', authenticated: true })).toBe(PRIVATE_CACHE);
    expect(cachePolicy({ pathname: '/', authenticated: true })).toBe(PRIVATE_CACHE);
  });

  // Belt and braces: these are private even to an anonymous request, because an
  // anonymous hit on them is a sign-in page or a redirect — never something worth
  // serving to the next person from a shared cache.
  it('never caches account, auth or moderation routes, signed in or not', () => {
    for (const pathname of [
      '/my',
      '/my/inbox',
      '/auth/callback',
      '/moderation',
      '/delete-account',
    ]) {
      expect(cachePolicy({ pathname, authenticated: false })).toBe(PRIVATE_CACHE);
      expect(cachePolicy({ pathname, authenticated: true })).toBe(PRIVATE_CACHE);
    }
  });

  // A route that names one of the private prefixes without being under it.
  it('matches whole path segments, not string prefixes', () => {
    expect(cachePolicy({ pathname: '/myths', authenticated: false })).toBe(PUBLIC_CACHE);
    expect(cachePolicy({ pathname: '/authors', authenticated: false })).toBe(PUBLIC_CACHE);
  });

  it('states a shared-cache lifetime and a revalidation window', () => {
    expect(PUBLIC_CACHE).toContain('s-maxage=300');
    expect(PUBLIC_CACHE).toContain('stale-while-revalidate=');
    // max-age=0 keeps the BROWSER revalidating: a visitor who signs in must not
    // be handed their own stale anonymous copy from disk cache.
    expect(PUBLIC_CACHE).toContain('max-age=0');
  });

  it('tells a shared cache to store nothing at all for a private response', () => {
    expect(PRIVATE_CACHE).toContain('no-store');
    expect(PRIVATE_CACHE).toContain('private');
  });
});

// A page about ONE entity is not re-rendered by an ingest run the way a listing is,
// and there are hundreds of thousands of them — the long tail is where a five-minute
// lifetime costs the most, because each page is seen once or twice per data centre
// per window and pays for a revalidation every time.
describe('cachePolicy holds an entity page longer than a listing', () => {
  it('gives a detail page the longer shared-cache lifetime', () => {
    for (const pathname of [
      '/jobs/senior-go-engineer-acme-1a2b',
      '/companies/acme',
      '/blog/why-we-built-this',
    ]) {
      expect(cachePolicy({ pathname, authenticated: false })).toBe(PUBLIC_DETAIL_CACHE);
    }
  });

  // These move with every ingest run: counts, newest-first ordering, which jobs a
  // filter now matches. They keep the short lifetime.
  it('leaves listings and index pages on the short lifetime', () => {
    for (const pathname of [
      '/',
      '/jobs',
      '/companies',
      '/collections/python',
      '/insights/skills/backend',
    ]) {
      expect(cachePolicy({ pathname, authenticated: false })).toBe(PUBLIC_CACHE);
    }
  });

  // A detail page's CHILDREN are a different thing: discussion carries
  // user-submitted comments, which should not sit at the edge for an hour.
  it('does not extend the lifetime to a detail page sub-route', () => {
    for (const pathname of ['/jobs/some-slug/discussion', '/companies/acme/discussion']) {
      expect(cachePolicy({ pathname, authenticated: false })).toBe(PUBLIC_CACHE);
    }
  });

  // The lifetime split must never weaken the rule it sits next to.
  it('still refuses to store a signed-in detail page', () => {
    expect(cachePolicy({ pathname: '/jobs/some-slug', authenticated: true })).toBe(PRIVATE_CACHE);
    expect(cachePolicy({ pathname: '/companies/acme', authenticated: true })).toBe(PRIVATE_CACHE);
  });

  it('differs from the listing policy only in the shared-cache lifetime', () => {
    expect(PUBLIC_DETAIL_CACHE).toContain('s-maxage=3600');
    // Same two guarantees as PUBLIC_CACHE: the browser keeps revalidating, and the
    // edge may keep serving while it refreshes.
    expect(PUBLIC_DETAIL_CACHE).toContain('max-age=0');
    expect(PUBLIC_DETAIL_CACHE).toContain('stale-while-revalidate=86400');
  });
});

// Regression: the hook first admitted only GET, so a HEAD probe came back with no
// Cache-Control at all — which is both wrong per RFC 9110 (HEAD's headers must
// match GET's) and how a production check quietly reported "the feature isn't
// deployed" when it was.
describe('cachePolicy is method-agnostic', () => {
  it('gives a HEAD probe the same answer as a GET', () => {
    expect(cachePolicy({ pathname: '/collections/python', authenticated: false })).toBe(PUBLIC_CACHE);
  });
});

// Regression, observed in production: an error page is HTML like any other, so it
// was handed the same `s-maxage=3600, stale-while-revalidate=86400` as the page it
// replaced. Cloudflare stored a 500 and served it as a HIT for the next hour — a
// transient failure during a blue/green flip turned into a page that stayed broken
// long after the origin had recovered.
describe('cachePolicy never lets a server error be stored', () => {
  it('refuses to cache a 5xx, whatever the route would otherwise get', () => {
    expect(cachePolicy({ pathname: '/', authenticated: false, status: 500 })).toBe(NO_CACHE);
    expect(cachePolicy({ pathname: '/companies/acme', authenticated: false, status: 502 })).toBe(
      NO_CACHE,
    );
    expect(cachePolicy({ pathname: '/collections/python', authenticated: false, status: 503 })).toBe(
      NO_CACHE,
    );
  });

  // A 404 is a fact about the URL, not a symptom: closed postings are a routine,
  // permanent outcome here and there are a great many of them, so letting the edge
  // absorb those repeats is the point. Only 5xx is withheld.
  it('still caches a 404, which is a stable answer rather than a failure', () => {
    expect(cachePolicy({ pathname: '/jobs/gone', authenticated: false, status: 404 })).toBe(
      PUBLIC_DETAIL_CACHE,
    );
    expect(cachePolicy({ pathname: '/nope', authenticated: false, status: 404 })).toBe(PUBLIC_CACHE);
  });

  // The signed-in guard is the one rule that must not be weakened by any of this.
  it('keeps a signed-in error response private, not merely uncached', () => {
    expect(cachePolicy({ pathname: '/', authenticated: true, status: 500 })).toBe(PRIVATE_CACHE);
  });

  // Omitting the status keeps the old signature working and means "nothing wrong".
  it('treats an absent status as a success', () => {
    expect(cachePolicy({ pathname: '/', authenticated: false })).toBe(PUBLIC_CACHE);
    expect(cachePolicy({ pathname: '/', authenticated: false, status: 200 })).toBe(PUBLIC_CACHE);
  });
});
