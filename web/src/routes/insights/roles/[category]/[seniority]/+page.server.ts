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
// Both 404s mean the same thing — the gate said no, or the rollup has no row — and say
// so in the same words, which is only guaranteed while there is one copy of them.
const NOT_COVERED = 'No insights for this role yet';

export const load: PageServerLoad = async ({ params, fetch, request, setHeaders }) => {
  const { category, seniority } = params;

  const globalRoles = await loadInsightsGate(fetch);
  if (!isCoveredRole(globalRoles, category, seniority)) error(404, NOT_COVERED);

  // The cookie is forwarded so the coverage overlay can be resolved during SSR — with
  // an absolute API base, event.fetch does not carry it on its own. The category-wide
  // read goes through the same client: it is a public endpoint that ignores the cookie,
  // and a second client built only to withhold one would be a distinction with no effect.
  const api = serverApi(fetch, request.headers.get('cookie'));
  const [role, categorySkills] = await Promise.all([
    api.insightsRole(category, seniority),
    api.insightsSkills({ category, limit: 12 }),
  ]);
  if (!role) error(404, NOT_COVERED);

  // A page carrying one visitor's coverage must never be held by a shared cache. The
  // sibling insights pages all set s-maxage, so the anonymous path keeps that and the
  // signed-in path opts out explicitly rather than inheriting it.
  setHeaders({
    'cache-control': role.coverage ? 'private, no-store' : 'public, max-age=0, s-maxage=3600',
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
