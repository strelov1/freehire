import {
  COLLECTIONS,
  FILTER_COLLECTIONS,
  collectionBySlug,
  toQuery,
  type SeeAlsoMark,
} from '#lib/collections.js';
import { backerBadges } from '#lib/backers.js';
import { COLLECTION_SECTIONS, type CollectionSectionDef } from '#lib/collectionSections.js';
import COLLECTION_STACKS from '#lib/data/collectionStacks.json';
import { resolveSeeAlsoMark } from '#lib/seeAlsoMark.js';
import type { serverApi } from './api';

// One collection inside a section, normalized across both kinds of collection so
// the pages render them uniformly. `count` is the open-job count, or null when it
// could not be fetched — counts are decorative, so a failed fetch degrades to no count.
export type CollectionRow = {
  slug: string;
  title: string;
  description: string;
  count: number | null;
  mark: SeeAlsoMark;
  // Leading companies hiring in it, biggest first — drawn as an overlapping logo
  // stack in place of `mark`. Empty when the snapshot has none for it: `mark` is
  // the fallback.
  companies: string[];
  // Leading countries, for a regional remote collection — a flag stack, same rule.
  flags: string[];
};

export type CollectionSection = CollectionSectionDef & { rows: CollectionRow[] };

// Runs `fn` over `items` at most `limit` at a time, keeping order: a hub render
// issues a hundred-odd count searches, and firing them all at once is a burst the
// search index then serves in one go for nothing.
async function mapLimit<T, R>(items: T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out: R[] = [];
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      out[i] = await fn(items[i] as T);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
  return out;
}

const CONCURRENCY = 8;

// The logo and flag stacks, from the snapshot scripts/build-collection-stacks.mjs
// writes — who leads a collection moves over weeks, so it is not worked out per render.
function stackOf(slug: string): { companies: string[]; flags: string[] } {
  const stack = (COLLECTION_STACKS as Record<string, { companies?: string[]; flags?: string[] }>)[slug];
  return { companies: stack?.companies ?? [], flags: stack?.flags ?? [] };
}

// Build the given sections with their rows ranked by open jobs. Company-collection
// counts come from the `collections` facet distribution (one call, made only when a
// company section is asked for); filter-collection counts are one job-search total
// each, in parallel, and only for the sections asked for. Every count is decorative,
// so a failed fetch degrades to no count rather than a 500. Empty sections are dropped.
export async function loadCollectionSections(
  api: ReturnType<typeof serverApi>,
  sections: CollectionSectionDef[] = COLLECTION_SECTIONS,
): Promise<CollectionSection[]> {
  const wanted = new Set<string>(sections.map((s) => s.key));
  const grouped = new Map<string, CollectionRow[]>(sections.map((s) => [s.key, []]));

  const companies = COLLECTIONS.filter((c) => wanted.has(c.kind));
  let facetCounts: Record<string, number> | null = null;
  if (companies.length > 0) {
    try {
      facetCounts = (await api.facetCounts(new URLSearchParams())).facets.collections ?? {};
    } catch {
      // Whole facet call failed → company counts are unknown, not zero.
    }
  }

  const filterRows = await mapLimit(
    FILTER_COLLECTIONS.filter((fc) => wanted.has(fc.group)),
    CONCURRENCY,
    async (fc) => {
      let count: number | null = null;
      try {
        count = (await api.searchJobs(new URLSearchParams(toQuery(fc.params)), 0, 0)).total ?? null;
      } catch {
        // Count is decorative — leave it null on failure.
      }
      const params = collectionBySlug(fc.slug)?.params ?? null;
      const row: CollectionRow = {
        slug: fc.slug,
        title: fc.title,
        description: fc.description,
        count,
        ...stackOf(fc.slug),
        mark: fc.flag ? { kind: 'flag', countryCode: fc.flag } : resolveSeeAlsoMark(params, null),
      };
      return { key: fc.group, row };
    },
  );
  for (const { key, row } of filterRows) grouped.get(key)?.push(row);

  for (const c of companies) {
    grouped.get(c.kind)?.push({
      slug: c.slug,
      title: c.title,
      description: c.description,
      count: facetCounts ? (facetCounts[c.slug] ?? 0) : null,
      ...stackOf(c.slug),
      mark: resolveSeeAlsoMark({ collections: c.slug }, backerBadges([c.slug])[0]?.mark ?? null),
    });
  }

  // Biggest first; an unknown count sinks to the bottom, and the sort is stable, so
  // ties keep registry order.
  return sections
    .map((s) => ({
      ...s,
      rows: [...(grouped.get(s.key) ?? [])].sort((a, b) => (b.count ?? -1) - (a.count ?? -1)),
    }))
    .filter((s) => s.rows.length > 0);
}
