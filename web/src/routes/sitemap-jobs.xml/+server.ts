import { serverApi } from '$lib/server/api';
import { JOB_SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One keyset chunk of open-job URLs (newest first), addressed by ?after=<id> (empty
// for the first chunk). The sitemap index lists these; each fetches exactly one
// chunk. This used to be a single file holding only the freshest slice of the
// catalogue — see JOB_SITEMAP_CHUNK for what changed.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const after = url.searchParams.get('after') ?? '';
  const jobs = await serverApi(fetch).sitemapJobs(after, JOB_SITEMAP_CHUNK);
  const entries = jobs.map((j) => ({ loc: `${url.origin}/jobs/${j.slug}`, lastmod: j.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
