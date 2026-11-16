import { api } from './api';
import type { FacetOption } from './facets';

// Option source for the Company facet's 'remote' control: the count-ordered
// companies endpoint (real names + open-job counts), not the Meili facet
// distribution (which Meili caps at 300 values and returns alphabetically — so
// popular employers never surface). An empty query returns the most active
// companies (the endpoint's first page).
export async function companySearch(query: string): Promise<FacetOption[]> {
  const { items } = await api.listCompanies(query, 20, 0);
  return items.map((c) => ({ value: c.slug, label: c.name, count: c.job_count }));
}
