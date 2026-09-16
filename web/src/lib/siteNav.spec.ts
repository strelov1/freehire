import { describe, it, expect } from 'vitest';
import { NAV, HEADER_LINKS } from './siteNav';
import { STATIC_PATHS } from './sitemap';

// A `.spec.ts`, so it runs under the `components` project: siteNav carries a Lucide glyph
// per destination, and those are real `.svelte` files the `unit` project cannot load. That
// is also why this assertion cannot live in sitemap.test.ts beside the rest of them.

describe('site navigation', () => {
  // DERIVED from NAV, not a second hand-written list — which is the only version of this
  // worth having. Naming the paths here would pass for exactly the destinations somebody
  // remembered to name, and that is how both /talent and /analytics came to sit in the
  // header menu while no sitemap, and so no crawler, knew they existed. A page the site's
  // own navigation points at is a page we ask people to visit; a search engine should be
  // told the same thing.
  it('leads only to destinations the sitemap also offers', () => {
    for (const { href, label } of Object.values(NAV)) {
      expect(STATIC_PATHS, `${label} (${href}) is in the site nav but not the sitemap`).toContain(
        href,
      );
    }
  });

  // The homepage row stands in for the search box on that one route and is a shortcut to
  // the menu's own top, not a second navigation with its own opinions — see the constant's
  // doc comment. The ceiling is what fits beside the brand and the menu controls at `lg`,
  // measured rather than chosen; the row has no overflow behaviour, so passing it crowds
  // those controls instead of wrapping. Raising this number means looking at the header at
  // 1024px first, which is the whole reason it is asserted at all.
  it('keeps the homepage header row within what fits, all of it from NAV', () => {
    expect(HEADER_LINKS.length).toBeLessThanOrEqual(6);
    for (const link of HEADER_LINKS) {
      expect(Object.values(NAV)).toContain(link);
    }
  });
});
