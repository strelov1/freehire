// Page-number paging for the job/company listings, in the URL as `?page=N`.
//
// The listings are an infinite-scroll feed for a visitor, which leaves crawlers
// with nothing to follow: link discovery happens when HTML is parsed, and the
// feed's rows past the first page only ever exist after a fetch. These helpers
// back a plain <nav> of <a href="?page=N"> rendered alongside the feed, so the
// catalogue is reachable by links and not by the sitemap alone.
//
// Pure and dependency-free so it unit-tests without a Svelte runtime.

/** Rows per page — the LIMIT every listing `load` searches with. */
export const PAGE_SIZE = 20;

/** How deep the search API will page: it refuses `offset + limit > SEARCH_WINDOW`
 *  with "pagination too deep" (internal/handler/search.go, maxSearchWindow). */
export const SEARCH_WINDOW = 10000;

/** Last page the API will serve, and so the last one worth linking — a link past
 *  it walks a crawler (or a visitor) into a 400. */
export const MAX_PAGE = SEARCH_WINDOW / PAGE_SIZE;

/** Whether one more page can be fetched after `loaded` rows starting at `offset`.
 *
 *  Infinite scroll otherwise runs off the end of the window and surfaces the API's
 *  400 as "Couldn't load more" — an error message for what is simply the end of the
 *  reachable results. `hasMore` can't answer this on its own: the search reports
 *  hundreds of thousands of matches, of which only the first SEARCH_WINDOW are
 *  retrievable. */
export function canFetchMore(offset: number, loaded: number): boolean {
  return offset + loaded + PAGE_SIZE <= SEARCH_WINDOW;
}

/** The `?page=N` a URL asks for: 1 for anything absent, malformed or out of range.
 *  Crawlers follow whatever they find and URLs get hand-edited, so every bad value
 *  has to mean page 1 rather than a negative offset or a 400. */
export function parsePage(params: URLSearchParams): number {
  const raw = params.get('page');
  if (!raw || !/^\d+$/.test(raw)) return 1;
  const page = Number(raw);
  if (page < 1) return 1;
  return Math.min(page, MAX_PAGE);
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
 *  `parsePage` deliberately clamps anything malformed or oversized to a page in
 *  range rather than failing, so a visitor never meets a 400 for a hand-edited URL.
 *  That leaves the pages between the last one holding rows and MAX_PAGE answering
 *  200 with an empty feed, a self-referencing canonical and no noindex — the shape
 *  Google reads as a soft 404 and, at one listing per collection, thousands of them.
 *  Clamping is right for the number; serving what it clamps to is not.
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

  const pages = [...around].toSorted((a, b) => a - b);
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

/** `?page=N` applied to the current query string, keeping every other param
 *  (filters, sort) intact. Page 1 drops the param entirely so the canonical
 *  first page has one address, not two. */
export function pageHref(pathname: string, params: URLSearchParams, page: number): string {
  const next = new URLSearchParams(params);
  if (page <= 1) next.delete('page');
  else next.set('page', String(page));
  const query = next.toString();
  return query ? `${pathname}?${query}` : pathname;
}
