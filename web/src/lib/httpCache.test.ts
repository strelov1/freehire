import { describe, expect, it } from 'vitest';
import { PRIVATE_CACHE, PUBLIC_CACHE, cachePolicy } from './httpCache';

describe('cachePolicy', () => {
  it('lets a shared cache hold an anonymous public page', () => {
    expect(cachePolicy({ pathname: '/companies/acme', authenticated: false })).toBe(PUBLIC_CACHE);
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

// Regression: the hook first admitted only GET, so a HEAD probe came back with no
// Cache-Control at all — which is both wrong per RFC 9110 (HEAD's headers must
// match GET's) and how a production check quietly reported "the feature isn't
// deployed" when it was.
describe('cachePolicy is method-agnostic', () => {
  it('gives a HEAD probe the same answer as a GET', () => {
    expect(cachePolicy({ pathname: '/collections/python', authenticated: false })).toBe(PUBLIC_CACHE);
  });
});
