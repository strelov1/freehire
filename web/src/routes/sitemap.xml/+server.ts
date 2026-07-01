import { serverApi } from '$lib/server/api';
import { SITEMAP_CHUNK, sitemapIndexXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The sitemap index. It lists one sub-sitemap for the static pages plus one per
// keyset chunk of jobs and companies, so the whole open catalogue is covered
// within the 50k-URL protocol limit. The chunk cursors come from the backend's
// boundary endpoints (the id/slug ending each chunk), so building the index is a
// couple of small queries — never a walk of the catalogue. Cached; a chunk is
// fetched only when a crawler follows its URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const api = serverApi(fetch);

  const [jobCursors, companyCursors] = await Promise.all([
    api.sitemapJobBoundaries(SITEMAP_CHUNK),
    api.sitemapCompanyBoundaries(SITEMAP_CHUNK),
  ]);

  const locs = [`${origin}/sitemap-pages.xml`];
  // The first chunk starts before every row (job id 0 / empty slug); each
  // boundary cursor starts the next chunk.
  for (const after of [0, ...jobCursors]) {
    locs.push(`${origin}/sitemap-jobs.xml?after=${after}`);
  }
  for (const after of ['', ...companyCursors]) {
    locs.push(`${origin}/sitemap-companies.xml?after=${encodeURIComponent(after)}`);
  }

  return xmlResponse(sitemapIndexXml(locs));
};
