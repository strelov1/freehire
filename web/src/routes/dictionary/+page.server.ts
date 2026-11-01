import { serverApi } from '#lib/server/api.js';
import { loadSkillDescriptions } from '#lib/skillDescriptions.js';
import type { FacetCounts } from '#lib/types.js';
import type { PageServerLoad } from './$types';

// The lists come from their source dictionaries, so the page shows what we recognize,
// not only what has open jobs today. Specialization/skill counts are job facets; we omit
// industry counts rather than scan the company catalogue on a request path.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const client = serverApi(fetch);
  const [catalog, counts] = await Promise.all([
    loadSkillDescriptions(),
    client
      .facetCounts(new URLSearchParams(), { facets: ['category', 'skills'] })
      .catch((): FacetCounts | null => null),
  ]);

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });
  return { skillSlugs: Object.keys(catalog), counts };
};
