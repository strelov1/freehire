import { describe, expect, it } from 'vitest';
import {
  MAX_PAGE,
  canFetchMore,
  pageCount,
  pageExists,
  pageHref,
  pageOffset,
  pageWindow,
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

  it('clamps past the deep-pagination ceiling the search API enforces', () => {
    expect(parsePage(new URLSearchParams(`page=${MAX_PAGE + 1}`))).toBe(MAX_PAGE);
    expect(parsePage(new URLSearchParams('page=999999'))).toBe(MAX_PAGE);
  });
});

describe('pageOffset', () => {
  it('is zero on page 1 and a whole page per step after', () => {
    expect(pageOffset(1)).toBe(0);
    expect(pageOffset(2)).toBe(20);
    expect(pageOffset(5)).toBe(80);
  });

  // offset+limit must stay within the API's 10000 window, or it answers 400.
  it('keeps the last page inside the search window', () => {
    expect(pageOffset(MAX_PAGE) + 20).toBeLessThanOrEqual(10000);
  });
});

describe('pageCount', () => {
  it('rounds up a partial last page', () => {
    expect(pageCount(0)).toBe(1);
    expect(pageCount(1)).toBe(1);
    expect(pageCount(20)).toBe(1);
    expect(pageCount(21)).toBe(2);
  });

  it('never advertises more pages than the API will serve', () => {
    // /collections/python reports ~140k matches; only the first 500 pages are reachable.
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
  it('accepts every page the results fill', () => {
    // 9,402 matches at 20 a page is 471 pages, the last holding two rows.
    expect(pageExists(1, 9402)).toBe(true);
    expect(pageExists(470, 9402)).toBe(true);
    expect(pageExists(471, 9402)).toBe(true);
  });

  // The real bug this guards: parsePage clamps anything up to MAX_PAGE, so
  // /collections/remote-latam?page=472 through ?page=500 each answered 200 with no
  // rows, a self-referencing canonical and no noindex — ~29 empty indexable URLs
  // per collection, and one collection per landing page.
  it('rejects the pages past the last one holding rows', () => {
    expect(pageExists(472, 9402)).toBe(false);
    expect(pageExists(MAX_PAGE, 9402)).toBe(false);
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
