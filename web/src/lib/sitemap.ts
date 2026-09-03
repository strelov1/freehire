// Shared builders for the sitemap index and its sub-sitemaps. The catalogue is
// far larger than the 50,000-URL / 50 MB sitemap-protocol limit, so /sitemap.xml
// is an index that points at chunked sub-sitemaps (static pages, job chunks,
// company chunks). Each chunk is one keyset page fetched by cursor.

import { collectionSlugs } from './collections';

// Must equal the backend's companySitemapChunk — the page size the offsets are
// computed with — so each sub-sitemap holds exactly one index page. Well under the
// protocol's 50,000-URL cap, and left at the size the Postgres-backed version used
// so the existing sub-sitemap URLs keep pointing at the same tiling.
export const SITEMAP_CHUNK = 10000;

// Must equal the backend's jobSitemapChunk. Job chunks are pages of the search
// index, not of the jobs table: the index already holds exactly the postings worth
// crawling, and it can address any offset in it directly — which is what let the
// sitemap stop asking Postgres to number 3.4M rows on every render.
//
// 10k rather than 25k because the deepest 25k page measured 8s against this route's
// 10s fetch timeout under load — see the backend constant for the trade.
export const JOB_SITEMAP_CHUNK = 10000;

/** The site's static, always-present pages (relative paths). */
export const STATIC_PATHS = [
  '/',
  // The job feed. It used to BE `/`, which is now the landing page — so this entry is
  // not a second copy of the homepage but the URL every filtered and paginated feed
  // link resolves to, and the one page here that carries the catalogue itself.
  '/jobs',
  '/about',
  '/companies',
  '/collections',
  '/for-companies',
  '/recruiters',
  '/features/extension',
  '/features/inbox',
  '/features/referrals',
  '/features/tailor',
  '/features/tracking',
  '/features/notifications',
  '/features/ghost-jobs',
  '/features/advanced-search',
  '/how-it-works',
  // The discussions feed. The individual threads are NOT here: they live under their
  // subject's route, and a sub-sitemap for the handful that exist would be
  // infrastructure ahead of need.
  '/discussions',
  // The data and API surfaces. Indexable pages that carry the site's most citable
  // material — live catalogue figures (/open), market rollups (/trends), the API
  // reference — so they belong in the sitemap even though nothing links to some of
  // them from the feed.
  '/open',
  '/trends',
  '/docs/api',
  '/agents',
  '/cli',
  '/chatgpt',
  '/contribute',
  '/status',
  '/privacy',
  // The role×country hub. Its category and pair pages are gated on live counts, so
  // they ride in their own per-category sub-sitemaps rather than here.
  '/roles',
];

/** The curated collection landing pages (`/collections/:slug`), one per collection.
 *  A small, fixed set, so they ride in the static-pages sub-sitemap alongside
 *  STATIC_PATHS rather than needing their own chunked file. */
export function collectionPaths(): string[] {
  return collectionSlugs().map((slug) => `/collections/${slug}`);
}

/** Sitemap entries for the blog: the index (`/blog`) plus one per published post,
 *  each dated with the post's own publish date. Takes the posts (from
 *  `listPosts()`) rather than reading them itself, so it stays pure/testable — the
 *  glob-backed loader is called by the route.
 *
 *  The dates matter: without a `lastmod` a crawler has nothing to tell an edited
 *  or newly published post from one it already has, and re-reads the whole set on
 *  its own schedule. The index carries the newest post's date, since that is
 *  exactly when its content last changed. */
export function blogPaths(posts: { slug: string; date: string }[]): PathEntry[] {
  const newest = posts.map((post) => post.date).sort().at(-1);
  return [
    { path: '/blog', lastmod: newest },
    ...posts.map((post) => ({ path: `/blog/${post.slug}`, lastmod: post.date })),
  ];
}

/** Sitemap paths for the insights pages: the hub plus salary/skills/roles for each
 *  covered category. Takes the already-gated category tokens (from
 *  `coveredCategories`) so it stays pure — a thin category is never listed. */
export function insightsPaths(categories: string[]): string[] {
  const paths = ['/insights'];
  for (const c of categories) {
    paths.push(`/insights/salary/${c}`, `/insights/skills/${c}`, `/insights/roles/${c}`);
  }
  return paths;
}

/** Sitemap paths for one category's landings: its country table plus a page per
 *  country that clears the gate. Takes the already-gated countries (from
 *  `publishedCountries`) so it stays pure — a thin pair is never listed, matching
 *  what the route serves for one (404).
 *
 *  Split per category on purpose: the whole product is ~2,200 URLs, which fits one
 *  file, but building it in one would cost one facet call per category in a single
 *  request. One sub-sitemap per category costs one call each, and the index can name
 *  all 37 without reading anything. */
export function roleLandingPaths(categorySlug: string, countrySlugs: string[]): string[] {
  return [
    `/roles/${categorySlug}`,
    ...countrySlugs.map((country) => `/roles/${categorySlug}/${country}`),
  ];
}

/** Sitemap paths for the skill glossary: the index plus one page per skill that has a
 *  description.
 *
 *  It takes the described slugs rather than the whole canonical vocabulary because the
 *  route 404s on a skill with no entry — listing all of them would point a crawler at
 *  pages that do not exist, and a sitemap full of 404s is worse than a short one. The
 *  caller reads the set from the same catalog the route does, so the two cannot drift.
 *
 *  One file, unlike the role landings: the whole glossary is under a thousand URLs and
 *  costs no read at all to enumerate, so there is nothing to shard away from. */
export function skillGlossaryPaths(describedSlugs: readonly string[]): string[] {
  return ['/skills', ...describedSlugs.map((slug) => `/skills/${slug}`)];
}

/** A sitemap path with the date its content last changed, before an origin is
 *  prefixed. `lastmod` is undefined when there is no honest date to state — a
 *  guessed one is worse than none, since a crawler that learns the dates are
 *  noise stops using them. */
export interface PathEntry {
  path: string;
  lastmod?: string;
}

export interface UrlEntry {
  loc: string;
  lastmod?: string;
}

function escapeXml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function urlTag({ loc, lastmod }: UrlEntry): string {
  const mod = lastmod ? `\n    <lastmod>${escapeXml(lastmod)}</lastmod>` : '';
  return `  <url>\n    <loc>${escapeXml(loc)}</loc>${mod}\n  </url>`;
}

/** A `<urlset>` sub-sitemap document from page URLs. */
export function urlsetXml(entries: UrlEntry[]): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${entries.map(urlTag).join('\n')}
</urlset>
`;
}

/** A `<sitemapindex>` document referencing sub-sitemap URLs. */
export function sitemapIndexXml(locs: string[]): string {
  const items = locs.map((loc) => `  <sitemap>\n    <loc>${escapeXml(loc)}</loc>\n  </sitemap>`).join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${items}
</sitemapindex>
`;
}

/** Wrap sitemap XML in a cached response (crawlers/CDN don't re-run the paging). */
export function xmlResponse(body: string): Response {
  return new Response(body, {
    headers: {
      'content-type': 'application/xml; charset=utf-8',
      'cache-control': 'public, max-age=3600',
    },
  });
}
