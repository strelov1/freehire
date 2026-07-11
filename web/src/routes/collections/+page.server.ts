import { serverApi } from '$lib/server/api';
import { COLLECTIONS, FILTER_COLLECTIONS, toQuery } from '$lib/collections';
import type { PageServerLoad } from './$types';

// One card on the /collections hub, normalized across both kinds of collection so
// the page can render them uniformly. `href` is the collection's own landing page
// (`/collections/:slug`, the component wraps it with `resolve`) and doubles as the
// card's identity; `count` is the open-job count, or null when it could not be
// fetched — counts are decorative, so a failed fetch degrades to no count.
export type CollectionCard = {
  title: string;
  description: string;
  href: `/collections/${string}`;
  count: number | null;
};

// Server-render the collection index. Company-collection counts come from the
// `collections` facet distribution over all open jobs (one call); filter-collection
// counts are one job-search total each (in parallel). Every count is decorative, so
// a failed fetch degrades to no count rather than a 500. Filter collections render
// first — they are the broad "browse by attribute" entries.
export const load: PageServerLoad = async ({ fetch }) => {
  const api = serverApi(fetch);

  let facetCounts: Record<string, number> | null = null;
  try {
    const facets = await api.facetCounts(new URLSearchParams());
    facetCounts = facets.facets.collections ?? {};
  } catch {
    // Whole facet call failed → company counts are unknown, not zero.
  }

  const filterCards: CollectionCard[] = await Promise.all(
    FILTER_COLLECTIONS.map(async (fc): Promise<CollectionCard> => {
      const query = toQuery(fc.params);
      let count: number | null = null;
      try {
        count = (await api.searchJobs(new URLSearchParams(query), 0, 0)).total ?? null;
      } catch {
        // Count is decorative — leave it null on failure.
      }
      return { title: fc.title, description: fc.description, href: `/collections/${fc.slug}`, count };
    }),
  );

  const companyCards: CollectionCard[] = COLLECTIONS.map((c) => ({
    title: c.title,
    description: c.description,
    href: `/collections/${c.slug}`,
    count: facetCounts ? (facetCounts[c.slug] ?? 0) : null,
  }));

  // Feature two cards at the top of the grid — worldwide-remote first, then the
  // AI-native cohort — ahead of the rest (filter collections, then the remaining
  // company collections in registry order).
  const featuredHrefs: CollectionCard['href'][] = [
    '/collections/remote-worldwide',
    '/collections/ai-native',
  ];
  const all = [...filterCards, ...companyCards];
  const featured = featuredHrefs
    .map((href) => all.find((c) => c.href === href))
    .filter((c): c is CollectionCard => c !== undefined);
  const rest = all.filter((c) => !featuredHrefs.includes(c.href));

  return { cards: [...featured, ...rest] };
};
