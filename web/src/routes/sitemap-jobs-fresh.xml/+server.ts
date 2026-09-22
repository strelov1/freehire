import { serverApi } from '$lib/server/api';
import { JOB_SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One page of the freshest-first job sub-sitemap, addressed by ?offset=<n>. The
// entries arrive newest first and are rendered in the order they arrive — the sort
// happens in the search engine, which is the only place it CAN happen (the paged
// sibling route reads an endpoint that cannot sort at all), so re-ordering anything
// here would throw away the one property this file exists for.
//
// An offset past the fresh window yields an empty file rather than an error, the same
// promise sitemap-jobs.xml makes to a crawler holding a stale index.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const offset = Math.max(Number(url.searchParams.get('offset')) || 0, 0);
  const jobs = await serverApi(fetch).sitemapJobsFresh(offset, JOB_SITEMAP_CHUNK);
  const entries = jobs.map((j) => ({ loc: `${url.origin}/jobs/${j.slug}`, lastmod: j.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
