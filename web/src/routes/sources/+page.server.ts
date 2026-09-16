import { serverApi } from '$lib/server/api';
import type { SourceEntry } from '$lib/types';
import type { PageServerLoad } from './$types';

// The public /sources page reads the whole source catalogue in ONE call to our own
// public API. Best-effort: a failing read renders an "unavailable" state rather than
// erroring the route, the same contract /status keeps.
//
// The payload is identical for every visitor (no cookie, no per-request input) and moves
// at the speed of a cron, so it is memoized module-side for a short window — one build
// serves every request in it instead of re-calling the API per visitor. This page is
// public and this host's traffic is mostly crawlers, so that window is the difference
// between one upstream call a minute and one per bot. A degraded build (null) is cached
// too, matching /open's per-leg best-effort semantics; it refreshes on the next miss.
const PAGE_TTL_MS = 60 * 1000;

let pageCache: { at: number; sources: SourceEntry[] | null } | null = null;

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  // Let the CDN and the browser hold it for the same window the server does.
  setHeaders({ 'cache-control': 'public, max-age=60' });

  if (pageCache && Date.now() - pageCache.at < PAGE_TTL_MS) {
    return { sources: pageCache.sources };
  }

  let sources: SourceEntry[] | null;
  try {
    sources = await serverApi(fetch).listSources();
  } catch {
    sources = null;
  }

  pageCache = { at: Date.now(), sources };
  return { sources };
};
