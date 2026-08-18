import { serverApi } from '$lib/server/api';
import { githubStats } from '$lib/server/github';
import type { PageServerLoad } from './$types';

// The /open transparency page pulls its numbers live at request time from our own
// public API plus the GitHub API. Every leg is best-effort (`Promise.allSettled`):
// a failing upstream yields null/[] and the page drops or falls back that one
// section, never the whole page.
//
// The GitHub leg (memoized, rate-limit-aware) moved to $lib/server/github so the
// header's star badge reads the same cache instead of spending the same 60/hour
// budget a second time.
export type { GithubStats } from '$lib/server/github';

// Every figure on the page is a public read (no cookie, identical for all visitors)
// and moves slowly, so the whole assembled payload is memoized module-side for a
// short TTL — one build serves every request in the window instead of re-fanning out
// to six API legs plus GitHub each time. A degraded build (a failed leg → null) is
// cached too, matching the per-section best-effort semantics; it refreshes on the
// next miss. (Date.now() is fine here — a server module, not a resumable workflow.)
const PAGE_TTL_MS = 60 * 1000;
type OpenPayload = Awaited<ReturnType<typeof buildPayload>>;
let pageCache: { at: number; data: OpenPayload } | null = null;

async function buildPayload(fetchImpl: typeof fetch) {
  const api = serverApi(fetchImpl);
  // One call for the whole scale strip instead of two list totals: the figures come
  // from a single published snapshot, so this page and /about cannot show numbers
  // measured at different moments.
  const [scale, activity, facets, growth, engagement, github] = await Promise.allSettled([
    api.catalogScale(),
    api.jobsActivity('day'),
    api.statsFacets(),
    api.userGrowth(),
    api.engagementStats(),
    githubStats(fetchImpl),
  ]);

  const value = <T>(r: PromiseSettledResult<T>): T | null =>
    r.status === 'fulfilled' ? r.value : null;

  const catalog = value(scale);
  // A degraded snapshot carries the approximate job count and the registry figures;
  // the counts that exist only in the database come back as zero. Map those to null
  // rather than passing the zero on: "we could not measure this" and "we measured
  // zero" must not look the same to a renderer, or a page ends up printing a figure
  // nobody stands behind.
  const dbOnly = (n: number | undefined) => (catalog?.exact && n != null ? n : null);

  return {
    scale: {
      jobs: catalog?.open_jobs ?? null,
      companies: dbOnly(catalog?.companies),
      sources: catalog?.sources ?? null,
      telegramChannels: dbOnly(catalog?.telegram_channels),
    },
    activity: value(activity) ?? [],
    facets: value(facets) ?? null,
    growth: value(growth) ?? [],
    engagement: value(engagement),
    github: value(github),
  };
}

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  // The figures move slowly; let the CDN/browser hold the page briefly too.
  setHeaders({ 'cache-control': 'public, max-age=300' });

  if (pageCache && Date.now() - pageCache.at < PAGE_TTL_MS) return pageCache.data;

  const data = await buildPayload(fetch);
  pageCache = { at: Date.now(), data };
  return data;
};
