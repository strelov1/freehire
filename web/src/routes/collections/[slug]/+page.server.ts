import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { findCollection } from '$lib/collections';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// Server-render a collection's first page of search results. The job list reuses
// the same filterable, counted search view as /jobs, pinned to this collection:
// `collections` is fixed (not a selectable facet) and merged into the search the
// same way the company page pins `company_slug`. An unknown slug is a 404.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const collection = findCollection(params.slug);
  if (!collection) error(404, 'Collection not found');

  const facets = new URLSearchParams(url.searchParams);
  facets.set('collections', params.slug);
  const initial = await serverApi(fetch).searchJobs(facets, LIMIT, 0);
  return { collection, initial };
};
