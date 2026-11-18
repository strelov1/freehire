import type { Http } from '../api-core';
import type {
  ActivityGranularity,
  CompanyHiring,
  CompanyListItem,
  CompanyRelationKind,
  Job,
  PendingCompanyRelation,
} from '../types';
import type { CompanyDetail, Page, Slice } from '../api-types';
import { jsonBody, query, toSlice } from '../api-core';

export function companiesApi({ call, request, requestData }: Http) {
  /** List companies, optionally filtered by a name query `q` (a case-insensitive
   *  substring match; an empty `q` lists everything). `meta.total` reflects the
   *  filtered count, so the Paginator pages over the matches. */
  // `facets` carries the sidebar's filter params (regions/collections/…, and `q`
  // when it lives in the filter model); the count-ordered typeahead just passes a
  // bare `q`. Both funnel through here so the endpoint stays one call site.
  async function listCompanies(
    q: string,
    limit: number,
    offset: number,
    facets?: URLSearchParams,
    signal?: AbortSignal,
  ): Promise<Slice<CompanyListItem>> {
    const params = new URLSearchParams(facets);
    if (q) params.set('q', q);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return toSlice(await request<Page<CompanyListItem>>(`/api/v1/companies?${params}`, { signal }), offset);
  }

  /** Propose a correction about the company at `slug`: that it is the same employer as
   *  `target_slug` (and retires into it), or a subsidiary of it. A moderator decides; a
   *  duplicate or a relation the registry cannot take is a 409 whose message says why. */
  async function proposeCompanyRelation(
    slug: string,
    input: { kind: CompanyRelationKind; target_slug: string; note: string },
  ): Promise<void> {
    await call(`/api/v1/companies/${encodeURIComponent(slug)}/relations`, jsonBody('POST', input));
  }

  /** The moderator queue of proposed company relations, oldest first. Moderator-only. */
  async function listPendingCompanyRelations(): Promise<PendingCompanyRelation[]> {
    return requestData<PendingCompanyRelation[]>('/api/v1/company-relations');
  }

  /** Apply a proposed relation. A `same` approval moves the retiring company's jobs and
   *  reports how many; approving it again resumes a move that stopped part-way. */
  async function approveCompanyRelation(id: number) {
    return requestData<{ kind: CompanyRelationKind; moved_jobs: number }>(`/api/v1/company-relations/${id}/approve`, {
      method: 'POST',
    });
  }

  /** Close a proposed relation without applying it. Moderator-only. */
  async function rejectCompanyRelation(id: number): Promise<void> {
    await call(`/api/v1/company-relations/${id}/reject`, { method: 'POST' });
  }

  async function getCompany(slug: string, limit: number, offset: number): Promise<CompanyDetail> {
    // `redirect: 'manual'` so a slug a merge retired surfaces as MovedError rather than
    // silently rendering the canonical company under the old url.
    return requestData<CompanyDetail>(`/api/v1/companies/${slug}${query(limit, offset)}`, { redirect: 'manual' });
  }

  /** The company page's Closed tab: a page of the company's closed postings. No total —
   *  the endpoint reports only whether another page follows. */
  async function companyClosedJobs(
    slug: string,
    limit: number,
    offset: number,
  ): Promise<{ jobs: Job[]; hasMore: boolean }> {
    const res = await request<{ data: Job[]; meta: { has_more: boolean } }>(
      `/api/v1/companies/${encodeURIComponent(slug)}/closed-jobs${query(limit, offset)}`,
    );
    return { jobs: res.data, hasMore: res.meta.has_more };
  }

  /** The company page's Hiring tab payload. The window follows the same rules as
   *  `jobsActivity`. A retired slug is not special-cased here: the page resolves the
   *  company through `getCompany` first, which is where the move is surfaced. */
  async function companyHiring(slug: string, granularity: ActivityGranularity): Promise<CompanyHiring> {
    return requestData<CompanyHiring>(
      `/api/v1/companies/${encodeURIComponent(slug)}/hiring?${new URLSearchParams({ granularity })}`,
    );
  }

  return {
    listCompanies,
    proposeCompanyRelation,
    listPendingCompanyRelations,
    approveCompanyRelation,
    rejectCompanyRelation,
    getCompany,
    companyClosedJobs,
    companyHiring,
  };
}
