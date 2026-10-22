import { serverApi } from '$lib/server/api';
import { loadSkillDescriptions } from '$lib/skillDescriptions';
import type { FacetCounts } from '$lib/types';
import type { PageServerLoad } from './$types';

// The lists come from the dictionaries themselves — the specialization vocabulary is
// static in the client, the skills are the glossary's catalog — so the page shows what
// we RECOGNISE, not only what has open jobs today. The counts are one unfiltered
// distribution, the numbers the filter modal shows, and are best-effort: with search
// unavailable the page still lists every term, just without numbers.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const [catalog, counts] = await Promise.all([
    loadSkillDescriptions(),
    serverApi(fetch)
      .facetCounts(new URLSearchParams(), { facets: ['category', 'skills'] })
      .catch((): FacetCounts | null => null),
  ]);

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return { skillSlugs: Object.keys(catalog), counts };
};
