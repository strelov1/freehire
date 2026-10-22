import { error, redirect } from '@sveltejs/kit';
import { ApiError, MovedError } from '#lib/api.js';
import { PAGE_SIZE, pageOffset, pageWithinWindow, parsePage } from '#lib/pagination.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { PageServerLoad } from './$types';

// The Closed tab: the company's closed postings, read from Postgres (closed rows are never
// in the search index). The company read decides the page's fate exactly as on the other
// tabs; a failed LIST resolves to null so the header still renders.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const client = serverApi(fetch);
  const pageNumber = parsePage(url.searchParams);
  if (!pageWithinWindow(pageNumber)) error(404, 'Page not found');

  const closed = client.companyClosedJobs(params.slug, PAGE_SIZE, pageOffset(pageNumber)).catch(() => null);

  let entity;
  try {
    entity = await client.getCompany(params.slug, 1, 0);
  } catch (e) {
    if (e instanceof MovedError) redirect(301, `/companies/${e.canonicalSlug}/closed${url.search}`);
    if (e instanceof ApiError && e.status === 404) error(404, 'Company not found');
    rethrowUpstream(e);
  }

  const list = await closed;
  // No total exists to ask `pageExists`, so an empty page past the first is the same
  // soft-404 it guards against: refuse it.
  if (list && pageNumber > 1 && list.jobs.length === 0) error(404, 'Page not found');

  return { slug: params.slug, company: entity.company, pageNumber, closed: list };
};
