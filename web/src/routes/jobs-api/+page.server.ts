import { serverApi } from '#lib/server/api.js';
import type { PageServerLoad } from './$types';

// The landing quotes live figures — catalogue scale, the plans' prices and limits, and the
// sources the catalogue is read from — from the same endpoints /about, /pricing and
// /sources read, so a number here can never be one the API does not stand behind. Loaded on
// the server so the HTML a crawler sees carries them.
//
// Every read degrades on its own: the page still renders without scale (static wording),
// without plans (no price cards) or without sources (no source strip), because the pitch
// and the docs links are the point.
//
// Nothing here depends on the visitor, every figure moves at the speed of a cron, and this
// host's traffic is mostly crawlers — so one build is memoized for a short window, the same
// trade /sources makes, instead of three upstream calls per bot. A degraded build is cached
// too; it refreshes on the next miss.
const PAGE_TTL_MS = 60 * 1000;

// How many sources the strip shows. Enough to read as a wall of recognisable platforms,
// few enough that the link to the full catalogue stays the obvious next step.
const SOURCE_STRIP_SIZE = 12;

let pageCache: { at: number; data: Awaited<ReturnType<typeof build>> } | null = null;

async function build(fetchImpl: typeof fetch) {
  const api = serverApi(fetchImpl);
  const [scale, plans, sources] = await Promise.all([
    api.catalogScale().catch(() => null),
    api.plans().catch(() => null),
    api.listSources().catch(() => null),
  ]);
  return {
    stats: scale
      ? {
          // The de-duplicated count /jobs shows; zero means not measured.
          jobs: scale.unique_open_jobs || null,
          companies: scale.exact ? scale.companies : null,
          atsPlatforms: scale.ats_platforms || null,
        }
      : { jobs: null, companies: null, atsPlatforms: null },
    plans,
    // Only the largest MEASURED sources, trimmed to what the strip renders: the full
    // catalogue is hundreds of entries and lives on /sources. An unmeasured count is
    // skipped rather than shown as zero.
    sources: sources
      ? {
          total: sources.length,
          top: sources
            .flatMap((s) => (s.jobs?.browsable ? [{ source: s.source, kind: s.kind, jobs: s.jobs.browsable }] : []))
            .sort((a, b) => b.jobs - a.jobs)
            .slice(0, SOURCE_STRIP_SIZE),
        }
      : null,
  };
}

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  // Let the CDN and the browser hold it for the same window the server does.
  setHeaders({ 'cache-control': 'public, max-age=60' });

  if (!pageCache || Date.now() - pageCache.at >= PAGE_TTL_MS) {
    pageCache = { at: Date.now(), data: await build(fetch) };
  }
  return pageCache.data;
};
