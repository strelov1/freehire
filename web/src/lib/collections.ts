import type { FamilyIconName } from './familymarks';
import { POPULAR_COLLECTION_FALLBACK } from './popularCollections';
import type { Job } from './types';
import { type FilterCollection, FILTER_COLLECTIONS } from './filterCollections';

// toQuery expands a filter collection's params into a URL query string, repeating a
// key once per value for list params (OR semantics). It is the single source for
// both the feed filter URL (`/jobs?<query>`) and its open-job count request, so the two
// can never disagree.
export function toQuery(params: Record<string, string | string[]>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    for (const v of Array.isArray(value) ? value : [value]) {
      q.append(key, v);
    }
  }
  return q.toString();
}

// The company-tag registry is generated from the Go source of truth
// (internal/collections) by cmd/gen-contracts and re-exported here so every import
// site keeps its `#lib/collections` path. It carries `kind`, which decides whether
// a tag renders as an editorial collection or as a credential — a field a hand-kept
// mirror could get wrong silently, which is why this stopped being hand-kept.
export { COLLECTIONS } from './generated/contracts';
import { COLLECTIONS as COMPANY_COLLECTIONS } from './generated/contracts';
/** @public */
export type { Collection, CollectionKind } from './generated/contracts';

// A resolved collection: the display copy plus the fixed job-search facet params
// that scope its feed. `params` is single-valued (the shape JobsView's `scope`
// pins) — every current collection maps to single values; a multi-value filter
// collection would need `scope` widened first (see design's array seam).
export type ResolvedCollection = {
  title: string;
  description: string;
  params: Record<string, string>;
};

// One card in a "see also" style block (JobSeeAlso), mirroring the /collections
// hub's CollectionRow: count is the collection's live open-job total, or null
// when it couldn't be fetched — decorative, so a failure degrades to no count
// rather than breaking the block. mark is always present — resolved by
// resolveSeeAlsoMark (seeAlsoMark.ts) to whichever is most specific for the
// card's underlying collection: a backer's brand image (Y Combinator,
// Techstars, …), a technology's brand logo, a country's flag, or a
// color-coded family icon. The type lives here (not in a route file) so both
// the server load that computes it and the component that renders it import
// from one place.
export type SeeAlsoMark =
  | { kind: 'image'; src: string }
  | { kind: 'logo'; title: string; path: string; hex: string }
  | { kind: 'flag'; countryCode: string }
  | { kind: 'family'; icon: FamilyIconName; color: string };

export type SeeAlsoCard = {
  slug: string;
  title: string;
  count: number | null;
  mark: SeeAlsoMark;
};

// Flatten a filter collection's params to the single-valued scope shape. A
// single-element list collapses to its element; a genuine multi-value param is
// unsupported by `scope` today and takes its first value (no such data exists).
function scopeParams(params: Record<string, string | string[]>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    if (Array.isArray(value)) {
      const [first] = value;
      if (first !== undefined) out[key] = first;
    } else {
      out[key] = value;
    }
  }
  return out;
}

// Resolve a slug to its collection, checking filter collections first, then
// company-membership collections (which map to the `collections` facet). Returns
// undefined for an unknown slug. The single source used by the /collections/:slug
// landing route, the hub card links, and the sitemap, so they cannot drift.
export function collectionBySlug(slug: string): ResolvedCollection | undefined {
  const filter = FILTER_COLLECTIONS.find((c) => c.slug === slug);
  if (filter) {
    return { title: filter.title, description: filter.description, params: scopeParams(filter.params) };
  }
  const company = COMPANY_COLLECTIONS.find((c) => c.slug === slug);
  if (company) {
    return { title: company.title, description: company.description, params: { collections: company.slug } };
  }
  return undefined;
}

// The curated feed for a category, where one exists — the counterpart link from the
// /roles country table, which is a market map rather than a feed and so wants to hand
// the reader the feed rather than reimplement it.
//
// Matches only a collection whose ONLY pin is the category. A feed pinning the
// category alongside anything else is narrower than "all <category> jobs", and
// offering it under that label would send the reader somewhere smaller than promised.
// 15 of the 37 categories have one; the rest returning undefined is ordinary.
export function collectionForCategory(category: string): { slug: string; title: string } | undefined {
  if (!category) return undefined;
  const match = FILTER_COLLECTIONS.find((c) => c.params.category === category && Object.keys(c.params).length === 1);
  return match ? { slug: match.slug, title: match.title } : undefined;
}

// The curated feed for a skill, where one exists — the counterpart link from the
// /skills glossary entry, which answers "what is this" and so wants to hand the reader
// the page that answers "who is hiring for it" rather than a facet URL whose canonical
// names /jobs. 45 of the 875 described skills have one; the rest returning undefined is
// ordinary, and their glossary pages keep the facet URL.
//
// Read from the PARAMS, never from a slug match, and the difference is not theoretical:
// `data-science`, `data-engineering`, `devops` and `machine-learning` each name both a
// skill and a collection, and each of those collections pins a CATEGORY. A category feed
// and a skill facet are different sets — /collections/devops is everything the classifier
// filed under DevOps, `skills=devops` is everything that names DevOps as a skill — so a
// slug match would publish a link promising one and delivering the other.
//
// Two guards, for two different mistakes. The single-key one is collectionForCategory's,
// for its reason: a feed pinning the skill alongside a region is narrower than "the open
// Kotlin jobs". The scalar one is because `params` values may be lists, which expand to
// repeated keys with OR semantics — `{ skills: ['go', 'rust'] }` is not the Go page's
// set. No entry is list-valued today; the guard is for the next one.
export function collectionForSkill(skill: string): { slug: string; title: string } | undefined {
  if (!skill) return undefined;
  const match = FILTER_COLLECTIONS.find((c) => c.params.skills === skill && Object.keys(c.params).length === 1);
  return match ? { slug: match.slug, title: match.title } : undefined;
}

// The inverse of collectionForSkill: the one skill a collection is about, or undefined
// when it is about anything else. The glossary link a collection landing page carries.
//
// It reads FILTER_COLLECTIONS rather than a ResolvedCollection's `params`, and that is
// the whole reason it exists instead of the landing page testing `collection.params`
// itself: `scopeParams` has already flattened a list-valued pin down to its FIRST value
// by then, so a `{ skills: ['go', 'rust'] }` feed would arrive there looking exactly like
// a `{ skills: 'go' }` one and earn a link to the Go glossary entry it is not about. The
// raw registry is the only place that distinction survives.
export function skillForCollection(slug: string): string | undefined {
  const c = FILTER_COLLECTIONS.find((x) => x.slug === slug);
  if (!c || Object.keys(c.params).length !== 1) return undefined;
  return typeof c.params.skills === 'string' ? c.params.skills : undefined;
}

// Every collection slug across both registries — the sitemap's source for the
// collection landing URLs. Slugs are unique across the two sets.
export function collectionSlugs(): string[] {
  return [...FILTER_COLLECTIONS.map((c) => c.slug), ...COMPANY_COLLECTIONS.map((c) => c.slug)];
}

// The facets of a viewed job that relatedCollectionSlugs matches against —
// deliberately only what the Job wire object actually carries (see
// web/src/lib/generated/contracts.ts): `category`/`seniority` come from
// `job.enrichment.category`/`job.enrichment.seniority` (not top-level fields),
// the rest map straight to `job.skills`/`work_mode`/`countries`/`regions`.
// There is no `role` here: roletag's seniority×category/named-role
// composition lives server-side only (internal/roletag), and duplicating it
// client-side would break the dict-only single-source-of-truth convention. A
// `role`-keyed FILTER_COLLECTIONS entry (e.g. senior-backend) therefore never
// matches via facets — it's only reachable through
// POPULAR_COLLECTION_FALLBACK.
export type JobFacets = {
  category?: string;
  seniority?: string;
  skills: string[];
  workMode?: string;
  countries: string[];
  regions: string[];
  collections: string[];
};

// Builds JobFacets from a wire Job — the one place that knows the
// enrichment.* vs top-level split documented on JobFacets above. Callers
// (currently jobs/[slug]/+page.server.ts) use this instead of hand-mapping
// fields, so a future Job shape change only needs updating here.
export function jobFacetsFromJob(job: Job): JobFacets {
  return {
    category: job.enrichment.category,
    seniority: job.enrichment.seniority,
    skills: job.skills,
    workMode: job.work_mode,
    countries: job.countries,
    regions: job.regions,
    collections: job.collections,
  };
}

// Whether a single FILTER_COLLECTIONS param entry is satisfied by the job's
// facets. `role` (and any other key the job carries no data for) never
// matches — see JobFacets' doc comment.
function facetMatches(paramKey: string, paramValue: string | string[], job: JobFacets): boolean {
  const values = Array.isArray(paramValue) ? paramValue : [paramValue];
  switch (paramKey) {
    case 'work_mode':
      return job.workMode !== undefined && values.includes(job.workMode);
    case 'countries':
      return values.some((v) => job.countries.includes(v));
    case 'regions':
      return values.some((v) => job.regions.includes(v));
    case 'skills':
      return values.some((v) => job.skills.includes(v));
    case 'category':
      return job.category !== undefined && values.includes(job.category);
    case 'seniority':
      return job.seniority !== undefined && values.includes(job.seniority);
    default:
      return false;
  }
}

// A collection matches a job when every one of its params is satisfied —
// mirroring how the collection's own landing page pins ALL of its params as
// a fixed scope (see collections/[slug]/+page.server.ts).
function collectionMatchesJob(entry: FilterCollection, job: JobFacets): boolean {
  return Object.entries(entry.params).every(([key, value]) => facetMatches(key, value, job));
}

// The job-detail-page "see also" block's link targets: a bounded, deduped,
// always-full list of collection slugs, built with no additional HTTP
// request from data the job page already has. Source A (the job's own
// facets, against FILTER_COLLECTIONS) is listed before Source B (the job's
// `collections` field — company membership — against the company registry),
// then padded with POPULAR_COLLECTION_FALLBACK up to `target`. Every
// returned slug resolves via collectionBySlug — this never invents a link to
// a collection that doesn't exist.
export function relatedCollectionSlugs(job: JobFacets, target = 10): string[] {
  const slugs: string[] = [];
  const seen = new Set<string>();
  const add = (slug: string) => {
    if (seen.has(slug)) return;
    seen.add(slug);
    slugs.push(slug);
  };

  for (const entry of FILTER_COLLECTIONS) {
    if (slugs.length >= target) break;
    if (collectionMatchesJob(entry, job)) add(entry.slug);
  }

  const companySlugs = new Set<string>(COMPANY_COLLECTIONS.map((c) => c.slug));
  for (const slug of job.collections) {
    if (slugs.length >= target) break;
    if (companySlugs.has(slug)) add(slug);
  }

  for (const slug of POPULAR_COLLECTION_FALLBACK) {
    if (slugs.length >= target) break;
    add(slug);
  }

  return slugs;
}

// relatedCollectionSlugs resolved to {slug, title} pairs via collectionBySlug
// — the same resolver the landing route and sitemap use — so a slug that
// somehow doesn't resolve is dropped here rather than rendered as a dead
// link. This is what JobSeeAlso renders; kept as a pure function (rather than
// resolving inline in the component) so it's unit-testable like the rest of
// this module, matching web/'s no-component-test-harness convention.
export function relatedCollectionLinks(job: JobFacets, target = 10): { slug: string; title: string }[] {
  return relatedCollectionSlugs(job, target)
    .map((slug) => {
      const collection = collectionBySlug(slug);
      return collection ? { slug, title: collection.title } : null;
    })
    .filter((link) => link !== null);
}
