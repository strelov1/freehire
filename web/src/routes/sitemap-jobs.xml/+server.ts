import { serverApi } from '$lib/server/api';
import { urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The freshest open-job URLs (newest first), one file. The jobs table is too large
// to enumerate per request without a heap-bound scan that evicts the buffer cache,
// so the sitemap ships the freshest slice; the backend caps the count.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const jobs = await serverApi(fetch).sitemapJobs();
  const entries = jobs.map((j) => ({ loc: `${url.origin}/jobs/${j.slug}`, lastmod: j.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
