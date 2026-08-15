import { describe, it, expect } from 'vitest';
import { SITEMAP_CHUNK, STATIC_PATHS, collectionPaths, blogPaths } from './sitemap';

describe('sitemap static paths', () => {
  it('includes the collections hub and the for-companies page', () => {
    expect(STATIC_PATHS).toContain('/collections');
    expect(STATIC_PATHS).toContain('/for-companies');
  });

  // Indexable pages that the job feed does not link to, so nothing but the sitemap
  // would lead a crawler to them.
  it('includes the data and API surfaces', () => {
    for (const path of ['/open', '/trends', '/docs/api', '/contribute', '/status', '/privacy']) {
      expect(STATIC_PATHS).toContain(path);
    }
  });

  it('lists each path once', () => {
    expect(new Set(STATIC_PATHS).size).toBe(STATIC_PATHS.length);
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
  it('includes the /blog index followed by one entry per post', () => {
    const paths = blogPaths([
      { slug: 'first', date: '2026-01-02' },
      { slug: 'second', date: '2026-01-05' },
    ]);
    expect(paths).toEqual([
      // The index changes whenever its newest post does.
      { path: '/blog', lastmod: '2026-01-05' },
      { path: '/blog/first', lastmod: '2026-01-02' },
      { path: '/blog/second', lastmod: '2026-01-05' },
    ]);
  });

  it('dates the index from the newest post regardless of input order', () => {
    const paths = blogPaths([
      { slug: 'older', date: '2025-11-30' },
      { slug: 'newest', date: '2026-03-01' },
      { slug: 'middle', date: '2026-01-15' },
    ]);
    expect(paths[0]).toEqual({ path: '/blog', lastmod: '2026-03-01' });
  });

  it('is just the index, undated, when there are no posts', () => {
    expect(blogPaths([])).toEqual([{ path: '/blog', lastmod: undefined }]);
  });
});

describe('SITEMAP_CHUNK', () => {
  // The chunk is a latency budget, not a protocol allowance: one chunk is one
  // Postgres read that has to finish inside the 60s proxy timeout even while an
  // ingest run is evicting the buffer cache. Keep it an order of magnitude under the
  // 50,000-URL protocol cap, and in step with internal/handler's companySitemapChunk.
  it('stays well under the sitemap protocol cap', () => {
    expect(SITEMAP_CHUNK).toBe(10000);
    expect(SITEMAP_CHUNK).toBeLessThanOrEqual(50000 / 4);
  });
});
