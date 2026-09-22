import { landingCategories } from '$lib/roleLandings';
import { serverApi } from '$lib/server/api';
import {
  JOB_SITEMAP_CHUNK,
  SITEMAP_CHUNK,
  sitemapIndexLocs,
  sitemapIndexXml,
  xmlResponse,
} from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The sitemap index: the static pages, the freshest-first job sub-sitemaps, one paged
// job sub-sitemap per index page, and one company sub-sitemap per index page. Both
// cursor lists are offsets the backend derives from a document count the search engine
// reports for free, so building the index costs no catalogue read at all. Cached; a
// sub-sitemap is fetched only when a crawler follows its URL.
//
// What goes in the list, and in what order, lives in sitemapIndexLocs — the order is a
// requirement, and a requirement asserted against a rendered XML string is barely
// asserted at all.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const api = serverApi(fetch);
  // Two independent boundary reads; run them together rather than in series.
  const [companyOffsets, jobOffsets] = await Promise.all([
    api.sitemapCompanyBoundaries(SITEMAP_CHUNK),
    api.sitemapJobBoundaries(JOB_SITEMAP_CHUNK),
  ]);

  return xmlResponse(
    sitemapIndexXml(
      sitemapIndexLocs({
        origin: url.origin,
        roleCategorySlugs: landingCategories().map(({ slug }) => slug),
        jobOffsets,
        companyOffsets,
      }),
    ),
  );
};
