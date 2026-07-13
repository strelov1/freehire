// Shared builders for the sitemap index and its sub-sitemaps. The catalogue is
// far larger than the 50,000-URL / 50 MB sitemap-protocol limit, so /sitemap.xml
// is an index that points at chunked sub-sitemaps (static pages, job chunks,
// company chunks). Each chunk is one keyset page fetched by cursor.

import { collectionSlugs } from './collections';

// Must equal the backend's sitemapMaxURLs — the chunk size the boundary cursors
// are computed with — so each sub-sitemap holds exactly one keyset chunk and
// never exceeds the protocol limit.
export const SITEMAP_CHUNK = 50000;

/** The site's static, always-present pages (relative paths). */
export const STATIC_PATHS = [
  '/',
  '/about',
  '/companies',
  '/collections',
  '/for-companies',
  '/cli',
  '/chatgpt',
  '/recruiters',
];

/** The curated collection landing pages (`/collections/:slug`), one per collection.
 *  A small, fixed set, so they ride in the static-pages sub-sitemap alongside
 *  STATIC_PATHS rather than needing their own chunked file. */
export function collectionPaths(): string[] {
  return collectionSlugs().map((slug) => `/collections/${slug}`);
}

/** Sitemap paths for the blog: the index (`/blog`) plus one path per published
 *  post. Takes the posts (from `listPosts()`) rather than reading them itself, so
 *  it stays pure/testable — the glob-backed loader is called by the route. */
export function blogPaths(posts: { slug: string }[]): string[] {
  return ['/blog', ...posts.map((post) => `/blog/${post.slug}`)];
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
