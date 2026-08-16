import { serverApi } from '$lib/server/api';
import { SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One page of company URLs, addressed by ?offset=<n> into the search index. The
// sitemap index lists these; each fetches exactly one page. An offset past the end
// yields an empty file rather than an error, so a crawler holding a stale index is
// never sent to a broken URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const offset = Math.max(Number(url.searchParams.get('offset')) || 0, 0);
  const companies = await serverApi(fetch).sitemapCompanies(offset, SITEMAP_CHUNK);
  const entries = companies.map((c) => ({
    loc: `${url.origin}/companies/${c.slug}`,
    lastmod: c.updated_at,
  }));
  return xmlResponse(urlsetXml(entries));
};
