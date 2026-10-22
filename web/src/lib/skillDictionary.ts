// The skills typeahead's candidate universe — canonical tokens with live job counts,
// shared by the profile form and the Skills tab (both need the same source the job-search
// filter panel uses, not a bundled/static list).

import { loadFacetDistribution, type FacetOption } from '#lib/facets.js';

/** Fetch the live skills distribution and shape it into sorted typeahead options. */
export function loadSkillDistribution(): Promise<FacetOption[]> {
  return loadFacetDistribution('skills');
}
