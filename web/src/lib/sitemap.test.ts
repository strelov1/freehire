import { describe, it, expect } from 'vitest';
import { STATIC_PATHS, collectionPaths, blogPaths } from './sitemap';

describe('sitemap static paths', () => {
  it('includes the collections hub and the for-companies page', () => {
    expect(STATIC_PATHS).toContain('/collections');
    expect(STATIC_PATHS).toContain('/for-companies');
  });

  it('includes the feature landings', () => {
    expect(STATIC_PATHS).toContain('/features/inbox');
    expect(STATIC_PATHS).toContain('/features/referrals');
    expect(STATIC_PATHS).toContain('/features/tailor');
  });

  // /referrals now answers with a 301 to its new home. Listing a redirect in the
  // sitemap tells crawlers to index the address that no longer serves the page.
  it('drops the moved referrals URL', () => {
    expect(STATIC_PATHS).not.toContain('/referrals');
  });
});

describe('collectionPaths', () => {
  it('maps every collection slug to its /collections/:slug landing path', () => {
    const paths = collectionPaths();
    expect(paths).toContain('/collections/yc'); // company collection
    expect(paths).toContain('/collections/remote-worldwide'); // filter collection
    expect(new Set(paths).size).toBe(paths.length);
  });
});

describe('blogPaths', () => {
  it('includes the /blog index followed by one path per post', () => {
    const paths = blogPaths([{ slug: 'first' }, { slug: 'second' }]);
    expect(paths).toEqual(['/blog', '/blog/first', '/blog/second']);
  });

  it('is just the index when there are no posts', () => {
    expect(blogPaths([])).toEqual(['/blog']);
  });
});
