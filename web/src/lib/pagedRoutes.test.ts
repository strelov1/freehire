import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A listing route that reads `?page=N` must also refuse a page number that
// addresses nothing.
//
// `parsePage` clamps anything malformed or oversized into range rather than
// failing, which is right for a hand-edited or crawled URL — but it means the
// pages between the last one holding rows and MAX_PAGE answer 200 with an empty
// list and a canonical of their own, which Google reads as a soft 404. `pageExists`
// is the check that turns those into a real 404.
//
// This is a source-text audit rather than a behavioural test because the thing it
// guards is an OMISSION, and an omission has no call site to assert on. It is
// written the way the bug arrived: `/companies` gained `?page=N` in one PR while
// the soft-404 rule was being written in another, so the two were consistent on
// their own and wrong together. Neither PR conflicted, and no test failed.
const ROUTES = join(import.meta.dirname, '..', 'routes');

describe('paged listing routes', () => {
  const paged = readdirSync(ROUTES, { recursive: true, encoding: 'utf8' })
    .filter((file) => file.endsWith('+page.server.ts'))
    .map((file) => ({ file, source: readFileSync(join(ROUTES, file), 'utf8') }))
    .filter(({ source }) => source.includes('parsePage('));

  it('finds the listings that page', () => {
    // Guards the audit itself: a rename that stops matching would otherwise leave
    // this suite passing over an empty set, which is the failure mode of every test
    // that greps.
    expect(paged.length).toBeGreaterThanOrEqual(4);
  });

  it('refuses a page number past the last page with rows', () => {
    const unguarded = paged
      .filter(({ source }) => !source.includes('pageExists('))
      .map(({ file }) => file);
    expect(unguarded, 'routes reading ?page=N without checking the page exists').toEqual([]);
  });

  // The second, earlier refusal. `pageExists` needs a total, so it can only answer
  // after the search has been paid for — and the pages it would refuse are the
  // expensive ones (see FEED_WINDOW in pagination.ts). `pageWithinWindow` answers
  // from the number alone, which is what lets a route refuse ?page=900 without
  // searching for it at all.
  it('refuses a page past the feed window before searching for it', () => {
    const unguarded = paged
      .filter(({ source }) => !source.includes('pageWithinWindow('))
      .map(({ file }) => file);
    expect(unguarded, 'routes reading ?page=N without bounding it to the window').toEqual([]);
  });

  // Order is the whole point of the check above: placed after the search it still
  // 404s, but has already spent what it exists to avoid.
  it('bounds the page before it searches, not after', () => {
    const lateGuard = paged
      .filter(({ source }) => source.includes('pageWithinWindow('))
      .filter(({ source }) => source.indexOf('pageWithinWindow(') > source.indexOf('pageOffset('))
      .map(({ file }) => file);
    expect(lateGuard, 'routes bounding the page only after the search').toEqual([]);
  });
});
