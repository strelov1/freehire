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

// A filter collection is the second kind of collection: a curated card that maps
// to an arbitrary /jobs facet filter rather than company membership. Unlike
// COLLECTIONS it is frontend-only — no Go registry, no `collections` search-facet
// value, no company/job membership. Adding one is a single entry below.
export type FilterCollection = {
  slug: string;
  title: string;
  description: string;
  // Job-search facet params this collection maps to — the same param names the
  // /jobs feed accepts (see search.StringFacets). A value may be a single string
  // or a list; a list expands into repeated query keys (OR semantics), matching
  // the /jobs filter contract.
  params: Record<string, string | string[]>;
};

export const FILTER_COLLECTIONS: FilterCollection[] = [
  {
    slug: 'remote-worldwide',
    title: 'Remote Worldwide',
    description:
      'Fully remote roles open to candidates anywhere in the world, not tied to a country or region.',
    params: { work_mode: 'remote', regions: 'global' },
  },
];

// toQuery expands a filter collection's params into a URL query string, repeating a
// key once per value for list params (OR semantics). It is the single source for
// both a card's link (`/jobs?<query>`) and its open-job count request, so the two
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

export const COLLECTIONS: Collection[] = [
  {
    slug: 'yc',
    title: 'Y Combinator',
    description:
      'Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.',
  },
  {
    slug: 'techstars',
    title: 'Techstars',
    description: 'Open roles at Techstars-backed companies.',
  },
  {
    slug: 'european',
    title: 'European Startups',
    description: "Open roles at European startups across the continent's tech hubs.",
  },
  {
    slug: 'ai',
    title: 'AI Companies',
    description:
      'Open roles at AI-native companies — foundation-model labs, ML platforms and applied-AI products.',
  },
  {
    slug: 'mag7',
    title: 'Magnificent Seven',
    description:
      'Open roles at the Magnificent Seven — Apple, Microsoft, Alphabet, Amazon, Meta, Nvidia and Tesla.',
  },
  {
    slug: 'bigtech',
    title: 'Big Tech',
    description: 'Open roles at the largest, most established technology companies.',
  },
  {
    slug: 'unicorn',
    title: 'Unicorns',
    description: 'Open roles at unicorns — private companies valued at over $1 billion.',
  },
  {
    slug: 'fortune500',
    title: 'Fortune 500',
    description: 'Open roles at Fortune 500 companies — the largest US corporations by revenue.',
  },
  {
    slug: 'russian-roots',
    title: 'Russian Roots',
    description:
      'Open roles at globally distributed companies founded by Russian-speaking founders or with Russian-speaking engineering roots.',
  },
];
