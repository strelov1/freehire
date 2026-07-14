import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The /open transparency page pulls its numbers live at request time from our own
// public API plus the GitHub API. Every leg is best-effort (`Promise.allSettled`):
// a failing upstream yields null/[] and the page drops or falls back that one
// section, never the whole page.

export type GithubStats = {
  stars: number;
  forks: number;
  contributors: number | null;
  license: string | null;
};

// Unauthenticated GitHub REST is capped at 60 requests/hour per IP, so the repo
// stats are memoized module-side and refreshed at most once per TTL, shared across
// all page loads. (Date.now() is fine here — this is a server module, not a
// resumable workflow script.)
const GH_TTL_MS = 60 * 60 * 1000;
const GH_REPO = 'strelov1/freehire';
let ghCache: { at: number; data: GithubStats | null } | null = null;

async function fetchGithub(fetchImpl: typeof fetch): Promise<GithubStats | null> {
  if (ghCache && Date.now() - ghCache.at < GH_TTL_MS) return ghCache.data;

  const headers = { Accept: 'application/vnd.github+json' };
  let data: GithubStats | null = null;
  try {
    const res = await fetchImpl(`https://api.github.com/repos/${GH_REPO}`, { headers });
    if (!res.ok) throw new Error(`github repo ${res.status}`);
    const repo = await res.json();

    // Contributor count without pulling the whole list: page it one-per-page and
    // read the last-page number out of the Link header.
    let contributors: number | null = null;
    try {
      const c = await fetchImpl(
        `https://api.github.com/repos/${GH_REPO}/contributors?per_page=1&anon=true`,
        { headers },
      );
      const last = c.headers.get('link')?.match(/[?&]page=(\d+)>;\s*rel="last"/);
      contributors = last ? Number(last[1]) : c.ok ? 1 : null;
    } catch {
      contributors = null;
    }

    data = {
      stars: repo.stargazers_count ?? 0,
      forks: repo.forks_count ?? 0,
      contributors,
      license: repo.license?.spdx_id ?? null,
    };
  } catch {
    data = null; // caching the null too, so a rate-limit spell doesn't retry every request
  }

  ghCache = { at: Date.now(), data };
  return data;
}

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const api = serverApi(fetch);
  const [jobs, companies, activity, facets, growth, engagement, github] = await Promise.allSettled([
    api.listJobs(1, 0),
    api.listCompanies('', 1, 0),
    api.jobsActivity('day'),
    api.facetCounts(new URLSearchParams()),
    api.userGrowth(),
    api.engagementStats(),
    fetchGithub(fetch),
  ]);

  const value = <T>(r: PromiseSettledResult<T>): T | null =>
    r.status === 'fulfilled' ? r.value : null;

  // The figures move slowly; let the CDN/browser hold the page briefly.
  setHeaders({ 'cache-control': 'public, max-age=300' });

  return {
    scale: {
      jobs: value(jobs)?.total ?? null,
      companies: value(companies)?.total ?? null,
    },
    activity: value(activity) ?? [],
    facets: value(facets)?.facets ?? null,
    growth: value(growth) ?? [],
    engagement: value(engagement),
    github: value(github),
  };
};
