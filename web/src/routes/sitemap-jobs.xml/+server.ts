import { serverApi } from '$lib/server/api';
import { JOB_SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One page of job URLs, addressed by ?offset=<n> into the search index. The sitemap
// index lists these; each fetches exactly one page. An offset past the end yields an
// empty file rather than an error, so a crawler holding a stale index is never sent
// to a broken URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const offset = Math.max(Number(url.searchParams.get('offset')) || 0, 0);
  const jobs = await serverApi(fetch).sitemapJobs(offset, JOB_SITEMAP_CHUNK);
  const entries = jobs.map((j) => ({ loc: `${url.origin}/jobs/${j.slug}`, lastmod: j.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
