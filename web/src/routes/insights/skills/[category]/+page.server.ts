import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { coveredCategories, isCovered, categoryLabel, skillsIntro } from '$lib/insights';
import type { PageServerLoad } from './$types';

// Per-category skill-demand landing page. Gated like the salary page: an uncovered
// category 404s. SSR-live + CDN cache window.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = params.category;
  const api = serverApi(fetch);

  const globalRoles = await api.insightsRoles({ limit: 200 });
  if (!isCovered(globalRoles, category)) error(404, 'No skill insights for this category yet');

  const skills = await api.insightsSkills({ category, sort: 'open', limit: 40 });
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    category,
    label: categoryLabel(category),
    covered: coveredCategories(globalRoles),
    skills,
    intro: skillsIntro(category, skills),
  };
};
