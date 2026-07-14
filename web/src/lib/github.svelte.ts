import { browser } from '$app/environment';

// Live GitHub star count for the repo, rendered as a header badge. Fetched once
// per session on first mount and cached in localStorage for a few hours, so we
// never hammer the unauthenticated API (60 req/h per IP) — a stale-but-instant
// number beats a spinner, and on any failure the badge just drops the number
// (the icon link still works). SSR-safe: every browser API is `browser`-guarded.

const REPO = 'strelov1/freehire';
export const GITHUB_URL = `https://github.com/${REPO}`;

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
      const res = await fetch(`https://api.github.com/repos/${REPO}`, {
        headers: { Accept: 'application/vnd.github+json' },
      });
      if (!res.ok) return;
      const data = (await res.json()) as { stargazers_count?: number };
      if (typeof data.stargazers_count === 'number') {
        this.count = data.stargazers_count;
        writeCache({ count: data.stargazers_count, at: Date.now() });
      }
    } catch {
      // offline / rate-limited — keep the cached value (or none)
    }
  }
}

export const githubStars = new GithubStarsStore();

/** Compact star count: 10870 → "10.9k", 987 → "987". */
export function formatStars(n: number): string {
  if (n < 1000) return String(n);
  return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
}
