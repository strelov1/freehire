import { describe, expect, it } from 'vitest';
import {
  API_WINDOW,
  FEED_WINDOW,
  MAX_PAGE,
  PAGE_SIZE,
  canFetchMore,
  pageCount,
  pageExists,
  pageHref,
  pageOffset,
  pageWindow,
  pageWithinWindow,
  parsePage,
} from './pagination';

describe('parsePage', () => {
  it('defaults to page 1 when the param is absent', () => {
    expect(parsePage(new URLSearchParams())).toBe(1);
  });

  it('reads a positive integer', () => {
    expect(parsePage(new URLSearchParams('page=7'))).toBe(7);
  });

  // A crawler follows whatever it finds, and a URL can be hand-edited. None of
  // these should 500 or page from a negative offset — they mean "page 1".
  it('treats junk, zero and negatives as page 1', () => {
    for (const q of ['page=0', 'page=-3', 'page=abc', 'page=', 'page=1.5', 'page=1e3']) {
      expect(parsePage(new URLSearchParams(q))).toBe(1);
    }
  });

  // Reads a too-deep page back as asked rather than clamping it. Clamping answered
  // 200 with the last reachable page's rows under every deeper address, so one set of
  // twenty jobs stood at four hundred URLs and a crawler walked them all. The number
  // survives so the route can refuse it by name — see pageWithinWindow.
  it('reads a page past the feed window back unchanged', () => {
    expect(parsePage(new URLSearchParams(`page=${MAX_PAGE + 1}`))).toBe(MAX_PAGE + 1);
    expect(parsePage(new URLSearchParams('page=999999'))).toBe(999999);
  });
});

describe('pageWithinWindow', () => {
  it('accepts every page the feed window reaches', () => {
    expect(pageWithinWindow(1)).toBe(true);
    expect(pageWithinWindow(MAX_PAGE)).toBe(true);
  });

  // The refusal a listing `load` must make BEFORE it searches. `pageExists` cannot
  // serve here: it needs a total, and a total costs the very request this avoids —
  // the deepest ones being the slowest, which is how a crawler walking ?page=486..500
  // turned an ordinary load spike into 500s (2026-09-20).
  it('refuses a page past it, so no search is made for one', () => {
    expect(pageWithinWindow(MAX_PAGE + 1)).toBe(false);
    expect(pageWithinWindow(999999)).toBe(false);
  });
});

describe('pageOffset', () => {
  it('is zero on page 1 and a whole page per step after', () => {
    expect(pageOffset(1)).toBe(0);
    expect(pageOffset(2)).toBe(20);
    expect(pageOffset(5)).toBe(80);
  });

  // offset+limit must stay within the API's window, or it answers 400.
  it('keeps the last page inside the search window', () => {
    expect(pageOffset(MAX_PAGE) + PAGE_SIZE).toBeLessThanOrEqual(API_WINDOW);
  });
});

describe('the two windows', () => {
  // The feed pages within the API's ceiling, never past it. Raising FEED_WINDOW above
  // API_WINDOW would replace our own 404 with the API's 400 on every deep page — the
  // failure would surface as a broken listing, not as a number being wrong, so it is
  // held here rather than in a comment.
  it('keeps the feed window inside what the API will serve', () => {
    expect(FEED_WINDOW).toBeLessThanOrEqual(API_WINDOW);
  });

  it('spends the whole feed window on whole pages', () => {
    expect(FEED_WINDOW % PAGE_SIZE).toBe(0);
  });
});

describe('pageCount', () => {
  it('rounds up a partial last page', () => {
    expect(pageCount(0)).toBe(1);
    expect(pageCount(1)).toBe(1);
    expect(pageCount(20)).toBe(1);
    expect(pageCount(21)).toBe(2);
  });

  it('never advertises more pages than a listing will serve', () => {
    // /collections/python reports ~140k matches — seven thousand pages of them. The
    // nav must offer MAX_PAGE, since every link past it is one this app answers 404 to.
    expect(pageCount(140_754)).toBe(MAX_PAGE);
  });

  it('handles a missing total as a single page', () => {
    expect(pageCount(undefined)).toBe(1);
  });
});

describe('pageWindow', () => {
  it('lists every page when they all fit', () => {
    expect(pageWindow(1, 5)).toEqual([1, 2, 3, 4, 5]);
  });

  it('keeps the first and last page reachable, with a gap marker between', () => {
    expect(pageWindow(50, 500)).toEqual([1, null, 48, 49, 50, 51, 52, null, 500]);
  });

  it('does not open a gap that hides nothing', () => {
    // Page 4 of 9: the run already reaches page 1, so no leading ellipsis.
    expect(pageWindow(4, 9)).toEqual([1, 2, 3, 4, 5, 6, null, 9]);
  });

  it('is a single page when there is nothing to page through', () => {
    expect(pageWindow(1, 1)).toEqual([1]);
  });
});

describe('canFetchMore', () => {
  it('allows another page well inside the window', () => {
    expect(canFetchMore(0, 20)).toBe(true);
    expect(canFetchMore(pageOffset(10), 20)).toBe(true);
  });

  // The real bug this guards: seeded on the last reachable page, infinite scroll
  // asked for the row after it, the API answered 400 "pagination too deep", and the
  // feed showed "Couldn't load more" — an error where "that's the end" belongs.
  it('refuses the fetch that would overrun the search window', () => {
    expect(canFetchMore(pageOffset(MAX_PAGE), 20)).toBe(false);
  });

  it('allows the fetch that lands exactly on the last row', () => {
    expect(canFetchMore(pageOffset(MAX_PAGE - 1), 20)).toBe(true);
  });
});

describe('pageExists', () => {
  // A listing that runs out of rows BEFORE the window runs out of pages — the only
  // shape in which this check decides anything, since a longer one is settled by
  // pageWithinWindow instead. Derived from MAX_PAGE rather than written as a literal
  // so that narrowing the window again cannot quietly turn these into the other case.
  const LAST_FILLED_PAGE = MAX_PAGE - 24;
  const ROWS = (LAST_FILLED_PAGE - 1) * PAGE_SIZE + 2; // last page holds two rows

  it('accepts every page the results fill', () => {
    expect(pageExists(1, ROWS)).toBe(true);
    expect(pageExists(LAST_FILLED_PAGE - 1, ROWS)).toBe(true);
    expect(pageExists(LAST_FILLED_PAGE, ROWS)).toBe(true);
  });

  // The real bug this guards: /collections/remote-latam answered 200 with no rows, a
  // self-referencing canonical and no noindex on every page between its last filled
  // one and the window's end — empty indexable URLs, one set per collection, and one
  // collection per landing page.
  it('rejects the pages past the last one holding rows', () => {
    expect(pageExists(LAST_FILLED_PAGE + 1, ROWS)).toBe(false);
    expect(pageExists(MAX_PAGE, ROWS)).toBe(false);
  });

  // An empty listing is still a page: a collection nobody is hiring for today is a
  // real landing page that should keep answering 200, and be there when it refills.
  it('keeps page 1 whatever the result count', () => {
    expect(pageExists(1, 0)).toBe(true);
    expect(pageExists(1, undefined)).toBe(true);
  });

  it('rejects a later page of an empty listing', () => {
    expect(pageExists(2, 0)).toBe(false);
  });

  // pageCount caps at MAX_PAGE because the search API refuses a deeper offset, so a
  // catalogue far larger than the window still stops where the window does.
  it('stops at the deepest page the search window reaches', () => {
    expect(pageExists(MAX_PAGE, 5_000_000)).toBe(true);
    expect(pageExists(MAX_PAGE + 1, 5_000_000)).toBe(false);
  });
});

describe('pageHref', () => {
  // The bug this guards: the page links were built from `page.url.searchParams`
  // while the filters reach the URL through a shallow replaceState that SvelteKit
  // never writes back into `page.url`. On the site's own front door — open `/`,
  // pick a dozen facets, click "2" — that read returned nothing and the link was a
  // bare `?page=2`, dropping every filter the visitor had set.
  it('carries every filter param onto the next page', () => {
    const params = new URLSearchParams({
      regions: 'global,latam',
      work_mode: 'hybrid,remote',
      skills_exclude: 'java,ruby',
      q: 'staff engineer',
    });
    expect(pageHref('/', params, 2)).toBe(
      '/?regions=global,latam&work_mode=hybrid,remote&skills_exclude=java,ruby&q=staff+engineer&page=2',
    );
  });

  // Page 1 is the canonical address of a filtered listing, so it carries the
  // filters and NOT a redundant `page=1`.
  it('drops the page param on the first page but keeps the filters', () => {
    expect(pageHref('/companies', new URLSearchParams({ industries: 'fintech' }), 1)).toBe(
      '/companies?industries=fintech',
    );
  });

  it('replaces the page already in the params rather than appending one', () => {
    const params = new URLSearchParams({ category: 'backend', page: '4' });
    expect(pageHref('/', params, 5)).toBe('/?category=backend&page=5');
  });

  // Commas stay literal: the filter store writes `skills=go,react` to the address
  // bar, so paging must not swap the visitor onto a percent-escaped twin of the
  // URL they are already on.
  it('keeps comma-joined facet values readable', () => {
    expect(pageHref('/', new URLSearchParams({ skills: 'go,react' }), 3)).toBe(
      '/?skills=go,react&page=3',
    );
  });

  it('leaves an unfiltered listing with a plain path', () => {
    expect(pageHref('/jobs', new URLSearchParams(), 1)).toBe('/jobs');
  });
});
