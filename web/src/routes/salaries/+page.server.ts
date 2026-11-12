import { serverApi } from '#lib/server/api.js';
import { readSalariesQuery } from '#lib/salaries.js';
import { scopeBands } from '#lib/insights.js';
import type { PageServerLoad } from './$types';

// /salaries: one role × grade × country scope — its bands now, its distribution and its
// months — and the lines it is compared with. Every read degrades to "no figure" on
// its own (a bad value in the address is a 400 from the API, read as no data), so one
// failed read never blanks the page.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const query = readSalariesQuery(url.searchParams);
  const { scope } = query;
  const api = serverApi(fetch);
  const lines = [{ category: scope.category, country: scope.country }, ...query.compare];
  const scopeParams = new URLSearchParams();
  if (scope.category) scopeParams.set('category', scope.category);
  if (scope.seniority) scopeParams.set('seniority', scope.seniority);
  if (scope.country) scopeParams.set('countries', scope.country);

  const [bands, histories, openJobs, countries] = await Promise.all([
    api
      .insightsSalary(scope)
      .then((b) => scopeBands(b, scope, Infinity))
      .catch(() => null),
    Promise.all(
      lines.map((l) =>
        api
          .insightsSalaryHistory({ category: l.category, seniority: scope.seniority, country: l.country })
          .catch(() => null),
      ),
    ),
    api
      .facetCounts(scopeParams)
      .then((c) => c.total)
      .catch(() => null),
    // The country picker: every country with open postings, busiest first.
    api
      .facetCountsDaily(new URLSearchParams(), { facets: ['countries'] })
      .then((c) =>
        Object.entries(c.facets.countries ?? {})
          .sort((a, b) => b[1] - a[1])
          .map(([code]) => code),
      )
      .catch(() => [] as string[]),
  ]);

  return { query, bands, histories, openJobs, countries, currentMonth: new Date().toISOString().slice(0, 7) };
};
