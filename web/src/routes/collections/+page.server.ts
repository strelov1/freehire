import { serverApi } from '#lib/server/api.js';
import { loadCollectionSections } from '#lib/server/collectionSections.js';
import type { PageServerLoad } from './$types';

// Server-render the collection index: every section, each ranked by open jobs.
export const load: PageServerLoad = async ({ fetch }) => {
  return { sections: await loadCollectionSections(serverApi(fetch)) };
};
