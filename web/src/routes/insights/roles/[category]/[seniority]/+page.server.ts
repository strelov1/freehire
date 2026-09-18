import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { loadInsightsGate } from '$lib/server/insights';
import { coveredCategories, isCoveredRole, roleSkillsIntro, seniorityLabel } from '$lib/insights';
import { categoryLabel } from '$lib/labels';
import type { PageServerLoad } from './$types';

// One role's leaf page: what the postings that state this (category, seniority) ask
// for, and — for a signed-in visitor — how much of it they already hold.
//
// The category-only distribution is loaded beside the role's and is NOT decoration.
// Measured 2026-09-18, only 39.0% of open technical postings state a seniority at all
// against 98.7% for category, so the role slice is a minority of the market and not a
// random one — it is the postings whose title happens to name a level. The category
// figures cover 98.7% and cost nothing new, so one number backstops the other.
export const load: PageServerLoad = async ({ params, fetch, request, setHeaders }) => {
  const { category, seniority } = params;

  const globalRoles = await loadInsightsGate(fetch);
  if (!isCoveredRole(globalRoles, category, seniority)) {
    error(404, 'No insights for this role yet');
  }

  // The cookie is forwarded so the coverage overlay can be resolved during SSR — with
  // an absolute API base, event.fetch does not carry it on its own.
  const cookie = request.headers.get('cookie');
  const api = serverApi(fetch, cookie);
  const [role, categorySkills] = await Promise.all([
    api.insightsRole(category, seniority),
    serverApi(fetch).insightsSkills({ category, limit: 12 }),
  ]);
  if (!role) error(404, 'No insights for this role yet');

  // A page carrying one visitor's coverage must never be held by a shared cache. The
  // sibling insights pages all set s-maxage, so the anonymous path keeps that and the
  // signed-in path opts out explicitly rather than inheriting it.
  setHeaders({
    'cache-control': role.coverage
      ? 'private, no-store'
      : 'public, max-age=0, s-maxage=3600',
  });

  return {
    category,
    seniority,
    label: categoryLabel(category),
    roleName: `${seniorityLabel(seniority)} ${categoryLabel(category)}`,
    covered: coveredCategories(globalRoles),
    role,
    categorySkills,
    intro: roleSkillsIntro(role),
  };
};
