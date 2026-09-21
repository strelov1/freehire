// Page-number paging for the job/company listings, in the URL as `?page=N`.
//
// The listings are an infinite-scroll feed for a visitor, which leaves crawlers
// with nothing to follow: link discovery happens when HTML is parsed, and the
// feed's rows past the first page only ever exist after a fetch. These helpers
// back a plain <nav> of <a href="?page=N"> rendered alongside the feed, so the
// catalogue is reachable by links and not by the sitemap alone.
//
// Pure, and free of the Svelte runtime, so it unit-tests in plain Node.

import { toSearchString } from './urlSearchString';

/** Rows per page — the LIMIT every listing `load` searches with. */
export const PAGE_SIZE = 20;

/** How deep the search API will page at all: it refuses `offset + limit > API_WINDOW`
 *  with "pagination too deep" (internal/api/handler/handler.go, maxPageWindow — it
 *  bounds every list endpoint, not just search, since the 2026-09-14 outage).
 *
 *  A ceiling on what the API ACCEPTS, not on what these pages ask for. Direct API
 *  clients still have all of it; FEED_WINDOW below is the narrower figure the site's
 *  own listings page within, and the two are separate because they answer different
 *  questions and have moved apart once already. Exported so the invariant between
 *  them is a test rather than a sentence — a FEED_WINDOW raised past this one would
 *  hand every visitor the API's 400 instead of our 404. */
export const API_WINDOW = 10000;

/** How deep the site's own listings page — deliberately a fraction of API_WINDOW.
 *
 *  Depth is not free even well inside the API's ceiling: measured on prod 2026-09-20,
 *  one filter answered offset=0 in 0.49s and offset=9980 in 1.68s, the same search
 *  costing three times as much purely for being deep. That is survivable in isolation
 *  and is not what it competes with — the host runs the crawl fleet and Meilisearch on
 *  one disk, and under an ordinary load spike every latency there multiplies by about
 *  five. The SSR read gives up at ten seconds (`$lib/api`'s call()), so the most
 *  expensive pages are the first to cross it, and they cross it as a 500: on 2026-09-20
 *  every one of the day's 40 server errors on /jobs carried a `page=`, clustered at
 *  486-500 — the deepest pages that existed. Nothing was broken; they were merely the
 *  costliest thing a crawler could ask for.
 *
 *  2000 rows is 100 pages. No visitor pages that far — infinite scroll is how the feed
 *  is actually read — and the catalogue is reached through the sitemap and the
 *  per-posting pages, not by walking a listing to its end. */
export const FEED_WINDOW = 2000;

/** Last page a listing serves, and so the last one worth linking. Past it a URL is a
 *  404 rather than a clamped duplicate — see pageWithinWindow. */
export const MAX_PAGE = FEED_WINDOW / PAGE_SIZE;

/** Whether one more page can be fetched after `loaded` rows starting at `offset`.
 *
 *  Infinite scroll otherwise runs off the end of the window and surfaces the end of
 *  the reachable results as "Couldn't load more" — an error message for what is
 *  simply the end. `hasMore` can't answer this on its own: the search reports
 *  hundreds of thousands of matches, of which only the first FEED_WINDOW are
 *  retrievable here. */
export function canFetchMore(offset: number, loaded: number): boolean {
  return offset + loaded + PAGE_SIZE <= FEED_WINDOW;
}

/** The `?page=N` a URL asks for: 1 for anything absent or malformed.
 *  Crawlers follow whatever they find and URLs get hand-edited, so every bad value
 *  has to mean page 1 rather than a negative offset or a 400.
 *
 *  A well-formed number too DEEP is returned as written, which malformed input is
 *  not, and the difference is deliberate: "abc" names no page, while 900 names one
 *  that could exist and does not. Clamping it hid that — every address past the
 *  window answered 200 with the last reachable page's rows, so one set of twenty
 *  jobs stood at hundreds of URLs, each self-canonical, each worth re-walking to a
 *  crawler. The route refuses the number instead; see pageWithinWindow. */
export function parsePage(params: URLSearchParams): number {
  const raw = params.get('page');
  if (!raw || !/^\d+$/.test(raw)) return 1;
  const page = Number(raw);
  if (page < 1) return 1;
  return page;
}

/** Whether `page` is one the feed window reaches at all — the check a listing `load`
 *  makes BEFORE it searches, answering 404 when it fails.
 *
 *  Separate from `pageExists` because it is answerable without a total, and that is
 *  the entire point: a total costs the search this avoids, and the too-deep searches
 *  are the expensive ones (see FEED_WINDOW). Asking "does this page hold rows?" first
 *  would pay the cost to learn it should not have.
 *
 *  Both checks are needed and neither implies the other: this one refuses page 900 of
 *  anything, `pageExists` refuses page 40 of a listing holding thirty rows. */
export function pageWithinWindow(page: number): boolean {
  return page <= MAX_PAGE;
}

/** Row offset the given page starts at. */
export function pageOffset(page: number): number {
  return (page - 1) * PAGE_SIZE;
}

/** How many pages `total` matches make, capped at what the API will serve. Always
 *  at least 1: an empty result set is still one (empty) page. */
export function pageCount(total: number | undefined): number {
  if (!total || total <= 0) return 1;
  return Math.min(Math.ceil(total / PAGE_SIZE), MAX_PAGE);
}

/** Whether `page` addresses rows that exist, which is what a listing `load` has to
 *  ask before serving one.
 *
 *  The second of the two refusals, and the one that needs a total. `pageWithinWindow`
 *  has already turned away anything past MAX_PAGE; what is left is the range between
 *  the last page holding rows and the window's end, which answered 200 with an empty
 *  feed, a self-referencing canonical and no noindex — the shape Google reads as a
 *  soft 404 and, at one listing per collection, thousands of them.
 *
 *  Page 1 always exists: a listing with nothing in it today is still a real landing
 *  page, and 404-ing it would drop a URL that refills tomorrow. */
export function pageExists(page: number, total: number | undefined): boolean {
  return page <= pageCount(total);
}

/** Page numbers to render as links, with `null` marking an elided run.
 *  Always includes the first and last page, plus a couple either side of the
 *  current one, so a crawler reaches both ends of the range from any page. */
export function pageWindow(current: number, total: number): (number | null)[] {
  const SPREAD = 2;
  const around = new Set<number>([1, total]);
  for (let p = current - SPREAD; p <= current + SPREAD; p++) {
    if (p >= 1 && p <= total) around.add(p);
  }

  const pages = [...around].sort((a, b) => a - b);
  const out: (number | null)[] = [];
  for (const [i, page] of pages.entries()) {
    const previous = pages[i - 1];
    if (previous !== undefined) {
      const hidden = page - previous - 1;
      // A gap marker costs the same room as a page number, so when it would hide
      // exactly one page, show that page instead — and link it rather than elide it.
      if (hidden === 1) out.push(previous + 1);
      else if (hidden > 1) out.push(null);
    }
    out.push(page);
  }
  return out;
}

/** `?page=N` applied to `params` — the query string as it stands in the ADDRESS BAR
 *  (see Pagination.svelte's prop note) — keeping every other param intact. Page 1
 *  drops the param entirely so the canonical first page has one address, not two.
 *
 *  Serialized with `toSearchString`, the same call the filter store's URL write
 *  uses, so paging can't swap the visitor onto a percent-escaped twin of the
 *  address they are already on. */
export function pageHref(pathname: string, params: URLSearchParams, page: number): string {
  const next = new URLSearchParams(params);
  if (page <= 1) next.delete('page');
  else next.set('page', String(page));
  const query = toSearchString(next);
  return query ? `${pathname}?${query}` : pathname;
}
