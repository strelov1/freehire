import { serverApi } from '$lib/server/api';
import { JOB_SITEMAP_CHUNK, SITEMAP_CHUNK, sitemapIndexXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The sitemap index: the static pages, one job sub-sitemap per keyset chunk, and
// one company sub-sitemap per keyset chunk. Company chunk cursors come from the
// backend's boundary endpoint (the slug ending each chunk), so building the index
// is a couple of small queries — never a walk of the catalogue. Cached; a
// sub-sitemap is fetched only when a crawler follows its URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const api = serverApi(fetch);
  // Two independent boundary reads; run them together rather than in series.
  const [companyCursors, jobCursors] = await Promise.all([
    api.sitemapCompanyBoundaries(SITEMAP_CHUNK),
    api.sitemapJobBoundaries(JOB_SITEMAP_CHUNK),
  ]);

  const locs = [`${origin}/sitemap-pages.xml`, `${origin}/sitemap-insights.xml`];
  // Jobs page the same way companies do: the first chunk starts above every id
  // (empty cursor), and each boundary id starts the next.
  for (const after of ['', ...jobCursors]) {
    locs.push(`${origin}/sitemap-jobs.xml?after=${encodeURIComponent(String(after))}`);
  }
  // The first company chunk starts before every slug (empty string); each boundary
  // cursor starts the next chunk.
  for (const after of ['', ...companyCursors]) {
    locs.push(`${origin}/sitemap-companies.xml?after=${encodeURIComponent(after)}`);
  }

  return xmlResponse(sitemapIndexXml(locs));
};
