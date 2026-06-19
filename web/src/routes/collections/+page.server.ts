import { serverApi } from '$lib/server/api';
import { COLLECTIONS } from '$lib/collections';
import type { PageServerLoad } from './$types';

// Server-render the collection index. Per-collection open-job counts come from the
// `collections` facet distribution over all open jobs (one search call). The counts
// are decorative, so a failed facet fetch degrades to no counts rather than a 500.
export const load: PageServerLoad = async ({ fetch }) => {
  let counts: Record<string, number> = {};
  try {
    const facets = await serverApi(fetch).facetCounts(new URLSearchParams());
    counts = facets.facets.collections ?? {};
  } catch {
    // Counts are decorative; leave the empty default rather than failing the page.
  }
  return { collections: COLLECTIONS, counts };
};
