import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { loadInsightsGate } from '$lib/server/insights';
import {
  coveredCategories,
  roleAddressExists,
  roleQualifies,
  roleSkillsIntro,
  seniorityLabel,
} from '$lib/insights';
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

  // The ADDRESS is checked before the API is asked anything. An unknown category or an
  // invented level is a 400 from the endpoint, which a load turns into a 500 — so a
  // mistyped URL would answer "we broke" instead of "no such page". The demand check has
  // to wait for the role's own open-count, but this half never did.
  if (!roleAddressExists(category, seniority)) error(404, NOT_COVERED);

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

  // The demand floor decides whether this page is worth INDEXING, not whether a visitor
  // may read it. It is asked of THIS role's own open-count rather than of its rank in the
  // gate's list, which the endpoint caps at 200 while production carries ~349 roles over
  // the floor.
  //
  // A thin role is SERVED, not refused: the job page links straight here from any posting
  // carrying both facets and cannot know the role's size without a request of its own, and
  // a link into a 404 is the failure this gate exists to avoid. The page answers honestly
  // — its table already has an empty state saying too few postings list skills — and
  // carries noindex, so the other failure, a thin page in the index, is closed too. The
  // sitemap goes on listing only what clears the floor.
  const thin = !roleQualifies(globalRoles, category, seniority, role.open_count);

  // A page carrying one visitor's coverage must never be held by a shared cache. The
  // sibling insights pages all set s-maxage, so the anonymous path keeps that and the
  // signed-in path opts out explicitly rather than inheriting it.
  setHeaders({
    'cache-control': role.coverage ? 'private, no-store' : 'public, max-age=0, s-maxage=3600',
  });

  return {
    thin,
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
