import type { Http } from '../api-core';
import type {
  InsightCompany,
  InsightRole,
  InsightRoleCountries,
  InsightRoleCountryCell,
  InsightRolePulse,
  InsightSalaryBand,
  InsightSalaryMonth,
  InsightSkill,
  SitemapEntry,
  SkillPulse,
} from '../api-types';
import { insightsQuery } from '../api-core';

export function insightsApi({ request, requestData }: Http) {
  async function searchCities(q: string, country?: string): Promise<{ value: string; country: string }[]> {
    const params = new URLSearchParams({ q });
    if (country) params.set('country', country);
    return requestData<{ value: string; country: string }[]>(`/api/v1/geo/cities?${params}`);
  }

  // --- Insights (aggregate market data behind the /insights SEO pages) --------

  /** Ranked roles (category × seniority). Pass `category` to restrict to one
   *  category's seniorities (the per-category roles page); omit for all. */
  async function insightsRoles(
    opts: { category?: string; country?: string; sort?: 'open' | 'growth'; limit?: number } = {},
  ): Promise<InsightRole[]> {
    return requestData<InsightRole[]>(`/api/v1/insights/roles?${insightsQuery(opts)}`);
  }

  /** One role's skill distribution, plus the signed-in caller's coverage of it when
   *  the request carries a session. Returns null when the role has no rollup row —
   *  the route renders that as a 404 rather than as an empty page. */
  async function insightsRole(category: string, seniority: string): Promise<InsightRole | null> {
    const rows = await requestData<InsightRole[]>(`/api/v1/insights/roles?${insightsQuery({ category, seniority })}`);
    return rows[0] ?? null;
  }

  /** The role × country grid. The endpoint picks the columns (its default ten). */
  async function insightsRoleCountries(): Promise<InsightRoleCountries> {
    const res = await request<{ data: InsightRoleCountryCell[]; meta: { countries: string[] } }>(
      '/api/v1/insights/role-countries',
    );
    return { cells: res.data, countries: res.meta.countries };
  }

  /** One category's market pulse and its all-seniority open count. */
  async function insightsRolePulse(category: string): Promise<InsightRolePulse> {
    return requestData<InsightRolePulse>(`/api/v1/insights/role-pulse?${insightsQuery({ category })}`);
  }

  /** Ranked skills, optionally scoped by category or country (not both). */
  async function insightsSkills(
    opts: { category?: string; country?: string; sort?: 'open' | 'growth'; limit?: number } = {},
  ): Promise<InsightSkill[]> {
    return requestData<InsightSkill[]>(`/api/v1/insights/skills?${insightsQuery(opts)}`);
  }

  /** Every seniority's salary bands for one category, in a single call (the
   *  per-category salary page). Rows carry their own `seniority` ('' = the
   *  category-wide band). */
  /** One role × grade × country scope's monthly salary history, every (currency,
   *  period) it has; an empty part means every value. */
  async function insightsSalaryHistory(scope: {
    category: string;
    seniority: string;
    country: string;
  }): Promise<InsightSalaryMonth[]> {
    const q = new URLSearchParams(Object.entries(scope).filter(([, v]) => v));
    return requestData<InsightSalaryMonth[]>(`/api/v1/insights/salary/history?${q}`);
  }

  async function insightsSalaryByCategory(category: string): Promise<InsightSalaryBand[]> {
    return requestData<InsightSalaryBand[]>(`/api/v1/insights/salary?category=${encodeURIComponent(category)}`);
  }

  /** The salary bands for one category in one country, as the /roles landings read
   *  them. Scoping by country drops the per-seniority breakdown the category-only
   *  call returns — the endpoint answers with one row per (currency, period) for the
   *  scope as a whole — so the rows carry no seniority and are not merged. */
  async function insightsSalaryByCategoryInCountry(category: string, country: string): Promise<InsightSalaryBand[]> {
    const q = new URLSearchParams({ category, country });
    return requestData<InsightSalaryBand[]>(`/api/v1/insights/salary?${q}`);
  }

  /** The salary bands for any role × grade × country scope (/analytics); an empty
   *  part means every value. A category-only scope also returns its per-seniority
   *  bands, each row tagged with its own `seniority`. */
  async function insightsSalary(scope: {
    category: string;
    seniority: string;
    country: string;
  }): Promise<InsightSalaryBand[]> {
    const q = new URLSearchParams(Object.entries(scope).filter(([, v]) => v));
    return requestData<InsightSalaryBand[]>(`/api/v1/insights/salary?${q}`);
  }

  /** Company hiring-signal leaderboard: companies ranked by 30-day open-job growth
   *  (`growth` ramping, `-growth` freezing) or `open` size. `minOpen` floors the
   *  current open-count to blunt ingest-artifact spikes. */
  async function insightsCompanies(
    opts: { sort?: 'growth' | '-growth' | 'open'; minOpen?: number; limit?: number } = {},
  ): Promise<InsightCompany[]> {
    const q = new URLSearchParams();
    if (opts.sort) q.set('sort', opts.sort);
    if (opts.minOpen != null) q.set('min_open', String(opts.minOpen));
    if (opts.limit != null) q.set('limit', String(opts.limit));
    return requestData<InsightCompany[]>(`/api/v1/insights/companies?${q.toString()}`);
  }

  /** The signed-in caller's own profile skills, joined against their retained
   *  weekly demand history (/my/market-pulse). Cookie-only, unlike the public
   *  insights* reads above. */
  async function marketPulse(): Promise<SkillPulse[]> {
    return requestData<SkillPulse[]>('/api/v1/me/market-pulse');
  }

  // --- Sitemap --------------------------------------------------------------
  //
  // Feeds behind the sitemap index (server routes only). Both halves are offset-paged
  // over a search index, and each boundary endpoint returns the offset that OPENS every
  // chunk — including the first, 0 — so the index lists those cursors as they come.

  /** One page of job sitemap entries starting at `offset` in the search index. */
  async function sitemapJobs(offset: number, limit: number): Promise<SitemapEntry[]> {
    return requestData<SitemapEntry[]>(`/api/v1/jobs/sitemap?offset=${offset}&limit=${limit}`);
  }

  /** One page of the FRESHEST-first job sub-sitemap, newest first.
   *
   *  A separate endpoint from sitemapJobs rather than a flag on it: the two read the
   *  same index through different engine routes — one pages documents (any offset,
   *  cheap, no ordering available), the other searches with a sort (ordered, bounded
   *  to a shallow window) — so they cannot share a cost model or a bound. */
  async function sitemapJobsFresh(offset: number, limit: number): Promise<SitemapEntry[]> {
    return requestData<SitemapEntry[]>(`/api/v1/jobs/sitemap/fresh?offset=${offset}&limit=${limit}`);
  }

  /** The offset opening each `chunk`-sized page of jobs — every sub-sitemap's cursor,
   *  including the first (0). */
  async function sitemapJobBoundaries(chunk: number): Promise<number[]> {
    return requestData<number[]>(`/api/v1/jobs/sitemap/boundaries?chunk=${chunk}`);
  }

  /** One page of company sitemap entries starting at `offset` in the search index. */
  async function sitemapCompanies(offset: number, limit: number): Promise<SitemapEntry[]> {
    return requestData<SitemapEntry[]>(`/api/v1/companies/sitemap?offset=${offset}&limit=${limit}`);
  }

  /** The offset opening each `chunk`-sized page of companies, including the first (0). */
  async function sitemapCompanyBoundaries(chunk: number): Promise<number[]> {
    return requestData<number[]>(`/api/v1/companies/sitemap/boundaries?chunk=${chunk}`);
  }

  return {
    searchCities,
    insightsRoles,
    insightsRole,
    insightsRoleCountries,
    insightsRolePulse,
    insightsSkills,
    insightsSalaryHistory,
    insightsSalaryByCategory,
    insightsSalaryByCategoryInCountry,
    insightsSalary,
    insightsCompanies,
    marketPulse,
    sitemapJobs,
    sitemapJobsFresh,
    sitemapJobBoundaries,
    sitemapCompanies,
    sitemapCompanyBoundaries,
  };
}
