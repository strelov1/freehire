// Repo stats from the GitHub API, fetched server-side and memoized for the whole
// process. Two surfaces read them — the /open transparency page (stars, forks,
// contributors, licence) and the header's star badge (stars only) — and they share
// this one cache deliberately: unauthenticated GitHub REST is capped at 60
// requests/hour per IP, and two independent caches of the same resource would
// spend that budget twice for no extra information.
//
// The badge used to call api.github.com straight from the browser. That put the
// per-IP cap on the visitor rather than on us, which is fine from a home
// connection and fails from anything shared — a datacentre, a corporate proxy, a
// carrier NAT — where the reply is a 403 that surfaces as a console error and a
// badge with no number. Serving it from here makes the count a property of the
// deployment instead of a property of whoever is looking.

export type GithubStats = {
  stars: number;
  forks: number;
  contributors: number | null;
  license: string | null;
};

const TTL_MS = 60 * 60 * 1000;
const REPO = 'strelov1/freehire';

// (Date.now() is fine here — this is a server module, not a resumable workflow.)
let cache: { at: number; data: GithubStats | null } | null = null;

/** Repo stats, from cache when fresh. Best-effort: any failure yields null, and
 *  the null is cached too so a rate-limit spell doesn't retry on every request. */
export async function githubStats(fetchImpl: typeof fetch): Promise<GithubStats | null> {
  if (cache && Date.now() - cache.at < TTL_MS) return cache.data;

  const headers = { Accept: 'application/vnd.github+json' };
  let data: GithubStats | null; // assigned on both the success and catch paths below
  try {
    const res = await fetchImpl(`https://api.github.com/repos/${REPO}`, { headers });
    if (!res.ok) throw new Error(`github repo ${res.status}`);
    const repo = await res.json();

    // Contributor count without pulling the whole list: page it one-per-page and
    // read the last-page number out of the Link header.
    let contributors: number | null = null;
    try {
      const c = await fetchImpl(
        `https://api.github.com/repos/${REPO}/contributors?per_page=1&anon=true`,
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
    data = null;
  }

  cache = { at: Date.now(), data };
  return data;
}
