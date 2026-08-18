import { browser } from '$app/environment';

// Live GitHub star count for the repo, rendered as a header badge. Fetched once
// per session on first mount and cached in localStorage for a few hours — a
// stale-but-instant number beats a spinner, and on any failure the badge just
// drops the number (the icon link still works). SSR-safe: every browser API is
// `browser`-guarded.
//
// The count comes from our own origin, not api.github.com. Called from the browser
// it spent the visitor's share of GitHub's unauthenticated cap (60 requests/hour
// per IP), which holds up from a home connection and fails from anything with a
// shared address — a datacentre, a corporate proxy, carrier NAT — where the reply
// is a 403: a console error, and a badge with no number. The server route answers
// from one process-wide hourly cache (see $lib/server/github).

const REPO = 'strelov1/freehire';
export const GITHUB_URL = `https://github.com/${REPO}`;
const STARS_ENDPOINT = '/github-stars';

const CACHE_KEY = 'hire.gh_stars';
const TTL_MS = 6 * 60 * 60 * 1000; // 6h

type Cached = { count: number; at: number };

function readCache(): Cached | null {
  if (!browser) return null;
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const v = JSON.parse(raw) as Cached;
    if (typeof v?.count === 'number' && typeof v?.at === 'number') return v;
  } catch {
    // corrupt / unavailable — ignore
  }
  return null;
}

function writeCache(v: Cached) {
  if (!browser) return;
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(v));
  } catch {
    // best-effort: private mode / quota
  }
}

class GithubStarsStore {
  count = $state<number | null>(null);
  private started = false;

  /** Populate the star count: instant from cache, then refreshed from the API
   *  when the cache is stale. Idempotent — safe to call from every mount. */
  async load() {
    if (!browser || this.started) return;
    this.started = true;

    const cached = readCache();
    if (cached) {
      this.count = cached.count;
      if (Date.now() - cached.at < TTL_MS) return; // fresh enough, skip the fetch
    }

    try {
      const res = await fetch(STARS_ENDPOINT);
      if (!res.ok) return;
      // `stars: null` means the server could not reach GitHub. Keep whatever the
      // cache held rather than overwriting a real number with an absence.
      const data = (await res.json()) as { stars?: number | null };
      if (typeof data.stars === 'number') {
        this.count = data.stars;
        writeCache({ count: data.stars, at: Date.now() });
      }
    } catch {
      // offline — keep the cached value (or none)
    }
  }
}

export const githubStars = new GithubStarsStore();

/** Compact star count: 10870 → "10.9k", 987 → "987". */
export function formatStars(n: number): string {
  if (n < 1000) return String(n);
  return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
}
