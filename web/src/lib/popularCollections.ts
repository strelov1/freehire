// The popular collections: the curated pool that pads a sparse "see also" block so
// it's never empty or too thin, and the strip of collection links in the site footer.
//
// A module of its own, apart from collections.ts, because the footer renders on every
// page: importing collections.ts there pulled the whole FILTER_COLLECTIONS table
// (~13 KB gzipped) into the root layout for ten links. The titles are therefore
// stated here rather than looked up, and a test holds each one to the title
// collectionBySlug resolves — the same resolver the landing route and the sitemap
// use — so a slug that stops resolving, or a title that drifts, fails the build
// instead of shipping a footer link to a 404.
//
// Each was verified to have a healthy live count before being added. Ordered by what
// visitors actually type into search (the `search_queries` table, read 2026-10-24):
// roles lead by a wide margin — data engineer, data analyst, AI engineer, backend,
// software engineer — while TypeScript, Go and AWS were not in the top sixty.
// `remote-worldwide` stays although it never appears there: remote is picked with a
// filter, not typed, and that table records only `q`.
//
// Collection landing pages are the site's programmatic-SEO surface, and until the
// footer strip existed the only internal links they had came from the /collections
// hub and the job-detail "see also" block. The homepage — the strongest page on the
// domain — linked 20 jobs, 4 feature pages and not one collection.
//
// Slug and title only, not an href: the footer builds the URL with
// `resolve('/collections/[slug]', …)` like every other dynamic route in the app, so
// the link stays correct under a non-empty `paths.base`.
export const POPULAR_COLLECTIONS: readonly { slug: string; title: string }[] = [
  { slug: 'remote-worldwide', title: 'Remote Worldwide' },
  { slug: 'data-engineering', title: 'Data Engineering' },
  { slug: 'data-analytics', title: 'Data Analytics' },
  { slug: 'ai-engineering', title: 'AI Engineering' },
  { slug: 'software-engineer', title: 'Software Engineer' },
  { slug: 'backend', title: 'Backend' },
  { slug: 'python', title: 'Python' },
  { slug: 'data-science', title: 'Data Science' },
  { slug: 'machine-learning', title: 'Machine Learning' },
  { slug: 'fullstack', title: 'Full-Stack' },
];

/** The popular slugs alone, in curated order — the "see also" padding pool. */
export const POPULAR_COLLECTION_FALLBACK: string[] = POPULAR_COLLECTIONS.map((c) => c.slug);
