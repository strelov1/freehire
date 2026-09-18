import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { loadInsightsGate } from '$lib/server/insights';
import { coveredCategories, isCovered, roleQualifies, rolesIntro } from '$lib/insights';
import { categoryLabel } from '$lib/labels';
import type { PageServerLoad } from './$types';

// Per-category roles landing page: the category's seniorities ranked by open-job
// demand. Gated + SSR-live like the sibling pages. The global roles ranking drives
// the gate + cross-links; the page's own rows come from the category-scoped read so
// a low-volume seniority isn't lost past the global top-N cutoff.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = params.category;
  const api = serverApi(fetch);

  const globalRoles = await loadInsightsGate(fetch);
  if (!isCovered(globalRoles, category)) error(404, 'No role insights for this category yet');

  const roles = await api.insightsRoles({ category, sort: 'open', limit: 20 });
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  // Which of this category's seniorities have a leaf page. Derived from the SAME
  // gate the leaf route and the sitemap read, so a level is linked only where the
  // link resolves — a row linking to a 404 is worse than a row that does not link.
  const leaves = new Set(
    roles.filter((r) => roleQualifies(globalRoles, category, r.seniority, r.open_count))
      .map((r) => r.seniority),
  );

  return {
    category,
    label: categoryLabel(category),
    covered: coveredCategories(globalRoles),
    roles,
    leaves,
    intro: rolesIntro(category, roles),
  };
};
