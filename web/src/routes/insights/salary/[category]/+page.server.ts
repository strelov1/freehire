import { error } from '@sveltejs/kit';
import { serverApi } from '#lib/server/api.js';
import { loadInsightsGate } from '#lib/server/insights.js';
import { coveredCategories, isCovered, sortBandsBySeniority, salaryIntro } from '#lib/insights.js';
import { categoryLabel } from '#lib/labels.js';
import type { PageServerLoad } from './$types';

// Per-category salary landing page. The gate (global roles ranking) decides whether
// this category is published; an uncovered category (or an unknown slug) 404s rather
// than rendering a thin page. Served SSR-live with a CDN cache window so crawler
// bursts don't hit the API per request.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = params.category;
  const api = serverApi(fetch);

  const globalRoles = await loadInsightsGate(fetch);
  if (!isCovered(globalRoles, category)) error(404, 'No salary insights for this category yet');

  // The history is a second read beside the bands. A failed one drops the chart rather
  // than the page, and caches for a minute so a transient error is not served for the
  // full hour.
  const [rawBands, history] = await Promise.all([
    api.insightsSalaryByCategory(category),
    api.insightsSalaryHistory(category).catch(() => null),
  ]);
  const bands = sortBandsBySeniority(rawBands);
  setHeaders({ 'cache-control': `public, max-age=0, s-maxage=${history ? 3600 : 60}` });

  return {
    category,
    label: categoryLabel(category),
    covered: coveredCategories(globalRoles),
    bands,
    intro: salaryIntro(category, bands),
    history: history ?? [],
    // The month still in progress, decided once on the server so the chart's hollow
    // point cannot move between the rendered HTML and hydration.
    currentMonth: new Date().toISOString().slice(0, 7),
  };
};
