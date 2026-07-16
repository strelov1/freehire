import { serverApi } from '$lib/server/api';
import { SITEMAP_CHUNK, sitemapIndexXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The sitemap index: the static pages, the freshest-jobs sub-sitemap (one file),
// and one company sub-sitemap per keyset chunk. Company chunk cursors come from the
// backend's boundary endpoint (the slug ending each chunk), so building the index
// is a couple of small queries — never a walk of the catalogue. Cached; a
// sub-sitemap is fetched only when a crawler follows its URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const companyCursors = await serverApi(fetch).sitemapCompanyBoundaries(SITEMAP_CHUNK);

  const locs = [
    `${origin}/sitemap-pages.xml`,
    `${origin}/sitemap-jobs.xml`,
    `${origin}/sitemap-insights.xml`,
  ];
  // The first company chunk starts before every slug (empty string); each boundary
  // cursor starts the next chunk.
  for (const after of ['', ...companyCursors]) {
    locs.push(`${origin}/sitemap-companies.xml?after=${encodeURIComponent(after)}`);
  }

  return xmlResponse(sitemapIndexXml(locs));
};
