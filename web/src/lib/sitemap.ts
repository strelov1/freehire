// Shared builders for the sitemap index and its sub-sitemaps. The catalogue is
// far larger than the 50,000-URL / 50 MB sitemap-protocol limit, so /sitemap.xml
// is an index that points at chunked sub-sitemaps (static pages, job chunks,
// company chunks). Each chunk is one keyset page fetched by cursor.

import { collectionSlugs } from './collections';

// Must equal the backend's companySitemapChunk — the chunk size the boundary
// cursors are computed with — so each sub-sitemap holds exactly one keyset chunk.
// Well under the protocol's 50,000-URL cap on purpose: these reads compete with the
// ingest for Postgres' buffer cache, and a 50k chunk measured both 0.9s warm and
// past the 60s proxy timeout during an ingest run (see internal/handler/sitemap.go).
export const SITEMAP_CHUNK = 10000;

// Must equal the backend's jobSitemapChunk. Larger than the company chunk because
// jobs_sitemap_idx (migration 0107) covers everything the job sitemap queries read,
// so a chunk is an index-only scan rather than the heap walk that once capped the
// whole job sitemap at a single 15,000-URL file.
export const JOB_SITEMAP_CHUNK = 25000;

/** The site's static, always-present pages (relative paths). */
export const STATIC_PATHS = [
  '/',
  '/about',
  '/companies',
  '/collections',
  '/for-companies',
  '/recruiters',
  '/features/inbox',
  '/features/referrals',
  '/features/tailor',
  '/features/ghost-jobs',
  // The data and API surfaces. Indexable pages that carry the site's most citable
  // material — live catalogue figures (/open), market rollups (/trends), the API
  // reference — so they belong in the sitemap even though nothing links to some of
  // them from the feed.
  '/open',
  '/trends',
  '/docs/api',
  '/cli',
  '/chatgpt',
  '/contribute',
  '/status',
  '/privacy',
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
  const newest = posts.map((post) => post.date).toSorted().at(-1);
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
