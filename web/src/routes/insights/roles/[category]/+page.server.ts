import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { coveredCategories, isCovered, categoryLabel, rolesIntro } from '$lib/insights';
import type { PageServerLoad } from './$types';

// Per-category roles landing page: the category's seniorities ranked by open-job
// demand. Gated + SSR-live like the sibling pages. The global roles ranking drives
// the gate + cross-links; the page's own rows come from the category-scoped read so
// a low-volume seniority isn't lost past the global top-N cutoff.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = params.category;
  const api = serverApi(fetch);

  const globalRoles = await api.insightsRoles({ limit: 200 });
  if (!isCovered(globalRoles, category)) error(404, 'No role insights for this category yet');

  const roles = await api.insightsRoles({ category, sort: 'open', limit: 20 });
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    category,
    label: categoryLabel(category),
    covered: coveredCategories(globalRoles),
    roles,
    intro: rolesIntro(category, roles),
  };
};
