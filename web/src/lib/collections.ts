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
