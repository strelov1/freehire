// The sources-to-avoid typeahead's candidate universe — the same `source` facet
// distribution the job-search filter panel uses, so a value picked here always matches a
// real `jobs.source`. Mirrors skillDictionary.ts's shape for the analogous source picker.

import { loadFacetDistribution, type FacetOption } from '#lib/facets.js';

/** Fetch the live source distribution and shape it into sorted typeahead options. */
export function loadSourceDistribution(): Promise<FacetOption[]> {
  return loadFacetDistribution('source', { facets: ['source'] });
}
