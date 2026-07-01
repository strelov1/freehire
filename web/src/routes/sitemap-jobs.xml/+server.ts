import { serverApi } from '$lib/server/api';
import { SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One keyset chunk of open-job URLs, addressed by ?after=<job id> (0 for the
// first chunk). The sitemap index lists these; each fetches exactly one chunk.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const after = Number(url.searchParams.get('after') ?? '0') || 0;
  const jobs = await serverApi(fetch).sitemapJobs(after, SITEMAP_CHUNK);
  const entries = jobs.map((j) => ({ loc: `${url.origin}/jobs/${j.slug}`, lastmod: j.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
