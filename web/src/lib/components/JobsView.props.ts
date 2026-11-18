import type { Snippet } from 'svelte';
import type { Slice } from '#lib/api.js';
import type { Job } from '#lib/types.js';

/**
 * Filters live in the URL; the route `load` searches by them and returns the
 * first page as `initial`, so the rows are in the initial HTML for SSR/share/
 * reload. After hydration the view is client-driven: FilterStore writes the URL
 * synchronously and exposes a debounced `applied` snapshot; a filter change
 * reloads the list + counts client-side off `applied` (no navigation), and
 * infinite scroll pages the rest.
 *
 * `scope` pins extra search params that the user can't change (e.g. the company
 * page passes `{ company_slug }`): they're merged into every search but kept out
 * of `filters`/the URL, so they're not user-selectable facets. `excludeFacets`
 * hides facets that are redundant under that scope (e.g. Source on a company).
 * `sidebarTop` renders above the filter summary in the desktop sidebar (e.g. the
 * company page's facts card); the standalone /jobs list omits it.
 *
 * `initialParams` overrides the seed below with the exact param string the
 * route's `load` searched with (page.url.searchParams minus `page`, see
 * +page.server.ts): the client must seed off that same string, not the raw
 * URL, or the two would disagree and the mount effect would immediately
 * discard the SSR page and refetch.
 *
 * `currentPage` is the `?page=N` the route's `load` served, and it is REQUIRED:
 * page links are now the only way through the results, so a route that mounts
 * this without honouring `?page=N` would render a nav whose every link leads
 * back to the page already on screen. Required rather than optional so that
 * mistake is a type error instead of a listing that silently stops at twenty.
 */
export type JobsViewProps = {
  initial: Slice<Job>;
  scope?: Record<string, string>;
  excludeFacets?: string[];
  sidebarTop?: Snippet;
  initialParams?: string;
  currentPage: number;
};
