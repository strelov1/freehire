// Curated collections shown on /collections. This mirrors the Go registry in
// internal/collections (the source of truth for membership + the search facet).
// It is a hand-kept mirror for now — only the display copy lives here; if the set
// grows, fold it into the generated contracts (gen-contracts) so the two can't
// drift. Keep `slug` identical to the Go registry slugs and the `collections`
// search-facet values.
export type Collection = {
  slug: string;
  title: string;
  description: string;
};

export const COLLECTIONS: Collection[] = [
  {
    slug: 'yc',
    title: 'Y Combinator',
    description:
      'Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.',
  },
  {
    slug: 'bigtech',
    title: 'Big Tech',
    description: 'Open roles at the largest, most established technology companies.',
  },
];

export function findCollection(slug: string): Collection | undefined {
  return COLLECTIONS.find((c) => c.slug === slug);
}
