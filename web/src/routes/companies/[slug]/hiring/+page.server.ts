import { error, redirect } from '@sveltejs/kit';
import { ApiError, MovedError } from '#lib/api.js';
import { dominantCurrency } from '#lib/companyHiring.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { FacetCounts } from '#lib/types.js';
import type { PageServerLoad } from './$types';

/** The facets the "Who they hire" block reads. Named, because counting is paid per facet. */
const BREAKDOWN_FACETS = ['category', 'seniority', 'countries', 'work_mode', 'skills'];

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
  const salary = yearlySalary(client, scope);

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
    salary: await salary,
  };
};

/** The company's yearly pay in the currency most of its YEARLY postings use — counts
 *  narrowed to one currency and period, because a range across them is meaningless (one
 *  employer measured 60 to 7,000,000). The currency is chosen among yearly postings only:
 *  chosen across all periods, ten hourly INR postings would pick INR over eight yearly
 *  USD ones and leave nothing to show. Null when nothing is stated yearly or a read
 *  fails. */
async function yearlySalary(
  client: ReturnType<typeof serverApi>,
  scope: URLSearchParams,
): Promise<{ currency: string; total: number; stats: FacetCounts['stats'] } | null> {
  const yearly = new URLSearchParams(scope);
  yearly.set('salary_period', 'year');
  try {
    const currencies = await client.facetCounts(yearly, { facets: ['salary_currency'] });
    const currency = dominantCurrency(currencies.facets.salary_currency);
    if (!currency) return null;
    yearly.set('salary_currency', currency);
    const counts = await client.facetCounts(yearly, { facets: ['salary_min', 'salary_max'] });
    return { currency, total: counts.total, stats: counts.stats };
  } catch {
    return null;
  }
}
