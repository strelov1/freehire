import { serverApi } from '$lib/server/api';
import { SITEMAP_CHUNK, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One keyset chunk of company URLs, addressed by ?after=<slug> (empty for the
// first chunk). The sitemap index lists these; each fetches exactly one chunk.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const after = url.searchParams.get('after') ?? '';
  const companies = await serverApi(fetch).sitemapCompanies(after, SITEMAP_CHUNK);
  const entries = companies.map((c) => ({ loc: `${url.origin}/companies/${c.slug}`, lastmod: c.updated_at }));
  return xmlResponse(urlsetXml(entries));
};
