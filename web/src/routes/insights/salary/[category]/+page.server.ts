import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { coveredCategories, isCovered, categoryLabel, sortBandsBySeniority, salaryIntro } from '$lib/insights';
import type { PageServerLoad } from './$types';

// Per-category salary landing page. The gate (global roles ranking) decides whether
// this category is published; an uncovered category (or an unknown slug) 404s rather
// than rendering a thin page. Served SSR-live with a CDN cache window so crawler
// bursts don't hit the API per request.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = params.category;
  const api = serverApi(fetch);

  const globalRoles = await api.insightsRoles({ limit: 200 });
  if (!isCovered(globalRoles, category)) error(404, 'No salary insights for this category yet');

  const bands = sortBandsBySeniority(await api.insightsSalaryByCategory(category));
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    category,
    label: categoryLabel(category),
    covered: coveredCategories(globalRoles),
    bands,
    intro: salaryIntro(category, bands),
  };
};
