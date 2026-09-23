// Shared builders for the sitemap index and its sub-sitemaps. The catalogue is
// far larger than the 50,000-URL / 50 MB sitemap-protocol limit, so /sitemap.xml
// is an index that points at chunked sub-sitemaps (static pages, job chunks,
// company chunks). Each chunk is one keyset page fetched by cursor.

import { collectionSlugs } from './collections';
import { contributorGroups } from './contributors';
import type { ContributorsSnapshot } from './contributors';
import contributorsSnapshot from './data/contributors.json';

// Must equal the backend's companySitemapChunk — the page size the offsets are
// computed with — so each sub-sitemap holds exactly one index page. Well under the
// protocol's 50,000-URL cap, and left at the size the Postgres-backed version used
// so the existing sub-sitemap URLs keep pointing at the same tiling.
export const SITEMAP_CHUNK = 10000;

// Must equal the backend's jobSitemapChunk. Job chunks are pages of the search
// index, not of the jobs table: the index holds every posting the site's own search
// can find, and it can address any offset in it directly — which is what let the
// sitemap stop asking Postgres to number 3.4M rows on every render.
//
// Note the chunks page a NARROWED view of that index, not all of it: the backend's
// jobSitemapFilter admits only tech postings that are not flagged as perpetual
// listings, and the count these offsets are computed from is filtered the same way.
// So the number of job chunks tracks the tech slice (~669k, 68 chunks) rather than
// the whole index (~2.0M) — a sub-sitemap URL for a retired offset answers with an
// empty urlset, which is what the route already promises for any offset past the end.
//
// 10k rather than 25k because the deepest 25k page measured 8s against this route's
// 10s fetch timeout under load — see the backend constant for the trade.
export const JOB_SITEMAP_CHUNK = 10000;

// How many chunks the FRESHEST-first job sub-sitemap spans. Must not exceed the
// backend's freshSitemapMaxOffset tiling (`3 * jobSitemapChunk`, i.e. four pages):
// past that bound the API answers an empty page on purpose, so a fifth entry here
// would put an empty file in the index rather than extending the window.
//
// Four chunks is 40,000 URLs, about 3.5 days of this catalogue's intake, so a crawler
// that skips two days still misses nothing. The bound exists because this file is
// SORTED, and unlike the paged chunks a sorted read gets more expensive with depth —
// see the backend constant for the measurements.
//
// Changing the count means moving BOTH this and freshSitemapMaxOffset. Moving only the
// Go side leaves an entry here pointing at a file the API deliberately answers empty,
// which looks exactly like a working sitemap — the same two-sided coupling
// JOB_SITEMAP_CHUNK already has with jobSitemapChunk.
export const FRESH_SITEMAP_CHUNKS = 4;

/** The offset opening each chunk of the freshest-first job sub-sitemap.
 *
 *  Derived from the two constants rather than written out, so the tiling cannot drift
 *  from the page size the API serves: a hard-coded list would survive a change to
 *  JOB_SITEMAP_CHUNK and then address offsets that fall between pages. */
export function freshJobSitemapOffsets(): number[] {
  return Array.from({ length: FRESH_SITEMAP_CHUNKS }, (_, i) => i * JOB_SITEMAP_CHUNK);
}

/** Every `<loc>` of the sitemap index, in the order a crawler reads them.
 *
 *  A pure function rather than inline in the route because the ORDER is a requirement
 *  (web-ssr-seo: the freshest-first sub-sitemaps precede the paged ones) and an order
 *  built inside a route handler has nothing to assert on but a rendered XML string.
 *  The route still owns the two boundary reads; this owns what to do with them. */
export function sitemapIndexLocs(params: {
  origin: string;
  roleCategorySlugs: string[];
  jobOffsets: number[];
  companyOffsets: number[];
}): string[] {
  const { origin, roleCategorySlugs, jobOffsets, companyOffsets } = params;
  const locs = [`${origin}/sitemap-pages.xml`, `${origin}/sitemap-insights.xml`];
  // The skills glossary is one file rather than a shard per letter: it is under a
  // thousand URLs and enumerating it reads nothing, so there is nothing to shard away
  // from — unlike the role landings below, where each shard pays its own facet call.
  locs.push(`${origin}/sitemap-skills.xml`);
  // One role sub-sitemap per category. The category list is a compile-time constant,
  // so naming all of them costs no read here — each shard pays its own single facet
  // call when a crawler actually follows it.
  for (const slug of roleCategorySlugs) {
    locs.push(`${origin}/sitemap-roles.xml?category=${slug}`);
  }
  // The freshest-first job chunks come BEFORE the paged ones. Position in a sitemap
  // index binds no crawler, but it is the only signal the format offers and it costs
  // nothing — and the paged chunks that follow are in arbitrary order, so without this
  // a crawler has no way at all to find what is new.
  for (const offset of freshJobSitemapOffsets()) {
    locs.push(`${origin}/sitemap-jobs-fresh.xml?offset=${offset}`);
  }
  // Both cursor lists already include the opening 0, so they are listed as they come.
  for (const offset of jobOffsets) {
    locs.push(`${origin}/sitemap-jobs.xml?offset=${offset}`);
  }
  for (const offset of companyOffsets) {
    locs.push(`${origin}/sitemap-companies.xml?offset=${offset}`);
  }
  return locs;
}

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
  // The public Talent Network catalogue. Indexable and linked from the header menu
  // and the footer, but it was in neither the sitemap nor anything a crawler reads,
  // so nothing told a search engine it exists. The per-candidate cards under it are
  // NOT here: a card is anonymous by design and 404s until its owner's CV has been
  // read, so offering crawlers a path to one is offering a path to a page we cannot
  // promise resolves.
  '/talent',
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
  // The market rollups. /analytics was reachable from the header menu and from
  // nowhere a crawler reads, the same gap /talent had.
  '/analytics',
  // Where the catalogue comes from: every source, what each carries, when it was last
  // read. Sits with /open rather than with /status because it is a citable statement
  // about the catalogue, not an operational dashboard.
  '/sources',
  '/trends',
  '/docs/api',
  '/agents',
  '/cli',
  '/chatgpt',
  // The public CV roast: an account-free landing page nothing in the feed links to,
  // built to be found by search rather than clicked to from elsewhere on the site.
  '/roast',
  '/contribute',
  // The contributor showcase. The per-person profiles are NOT here — they come from
  // the committed snapshot via contributorPaths(), the same way collections do.
  '/contributors',
  '/status',
  // Named as the App Store listing's Support URL, so it must resolve and stay
  // indexable — a 404 there is a rejection.
  '/support',
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

/** The per-contributor profile pages (`/contributors/:login`), one per person the
 *  showcase actually lists.
 *
 *  Derived through the same rules the showcase renders, not from the raw snapshot: a
 *  bot's profile 404s, so offering crawlers a path to one would be pointing them at a
 *  page we know does not exist. Each profile is its own indexable page because it is
 *  the page a person shares and is found by. */
export function contributorPaths(): string[] {
  const { maintainers, contributors } = contributorGroups(contributorsSnapshot as ContributorsSnapshot);

  return [...maintainers, ...contributors].map((entry) => `/contributors/${entry.login}`);
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
export function insightsPaths(categories: string[], roleLeaves: [string, string][] = []): string[] {
  const paths = ['/insights'];
  for (const c of categories) {
    paths.push(`/insights/salary/${c}`, `/insights/skills/${c}`, `/insights/roles/${c}`);
  }
  // The per-role leaves, passed already gated (from `coveredRoles`) so a pair the
  // route would 404 is never listed — the same rule read off the same numbers that
  // `roleLandingPaths` follows for the jobs landings.
  for (const [category, seniority] of roleLeaves) {
    paths.push(`/insights/roles/${category}/${seniority}`);
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
