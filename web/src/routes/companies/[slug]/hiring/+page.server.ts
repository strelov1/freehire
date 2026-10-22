import { error, redirect } from '@sveltejs/kit';
import { ApiError, MovedError } from '$lib/api';
import { dominantCurrency } from '$lib/companyHiring';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { FacetCounts } from '$lib/types';
import type { PageServerLoad } from './$types';

/** The facets the "Who they hire" block reads, plus the two that choose the salary
 *  currency and period. Named, because counting is paid per facet. */
const BREAKDOWN_FACETS = ['category', 'seniority', 'countries', 'work_mode', 'skills', 'salary_currency', 'salary_period'];

// The Hiring tab: everything is server-rendered, so the figures, the chart and the
// breakdowns are in the initial HTML a crawler reads.
//
// The company read decides the page's fate — 404, retired-slug 301, or an upstream
// failure — exactly as on the Jobs tab. Every OTHER read is best-effort: the hiring
// series and the facet counts each resolve to null on failure, and the page renders
// what it has rather than going down with one of them.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const client = serverApi(fetch);
  const scope = new URLSearchParams({ company_slug: params.slug });

  // Started before the company read so the three overlap.
  const hiring = client.companyHiring(params.slug, 'day').catch(() => null);
  const breakdown = client.facetCounts(scope, { facets: BREAKDOWN_FACETS }).catch(() => null);

  let entity;
  try {
    entity = await client.getCompany(params.slug, 1, 0);
  } catch (e) {
    if (e instanceof MovedError) redirect(301, `/companies/${e.canonicalSlug}/hiring`);
    if (e instanceof ApiError && e.status === 404) error(404, 'Company not found');
    rethrowUpstream(e);
  }

  return {
    slug: params.slug,
    company: entity.company,
    response: entity.response ?? null,
    hiring: await hiring,
    breakdown: await breakdown,
    salary: await yearlySalary(client, scope, await breakdown),
  };
};

/** The company's yearly pay in the currency most of its postings use — a second count
 *  narrowed to that currency and period, because a range across currencies and periods
 *  is meaningless (one employer measured 60 to 7,000,000). Null when nothing is stated
 *  yearly or the read fails. */
async function yearlySalary(
  client: ReturnType<typeof serverApi>,
  scope: URLSearchParams,
  breakdown: FacetCounts | null,
): Promise<{ currency: string; total: number; stats: FacetCounts['stats'] } | null> {
  const currency = dominantCurrency(breakdown?.facets.salary_currency);
  if (!currency || !breakdown?.facets.salary_period?.year) return null;
  const params = new URLSearchParams(scope);
  params.set('salary_currency', currency);
  params.set('salary_period', 'year');
  try {
    const counts = await client.facetCounts(params, { facets: ['salary_min', 'salary_max'] });
    return { currency, total: counts.total, stats: counts.stats };
  } catch {
    return null;
  }
}
