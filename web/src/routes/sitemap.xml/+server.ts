import { SKILL_DESCRIBED } from '$lib/generated/contracts';
import { landingCategories } from '$lib/roleLandings';
import { isGlossaryPublished } from '$lib/skillGlossary';
import { serverApi } from '$lib/server/api';
import { JOB_SITEMAP_CHUNK, SITEMAP_CHUNK, sitemapIndexXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// The sitemap index: the static pages, one job sub-sitemap per index page, and one
// company sub-sitemap per index page. Both cursor lists are offsets the backend
// derives from a document count the search engine reports for free, so building the
// index costs no catalogue read at all. Cached; a sub-sitemap is fetched only when a
// crawler follows its URL.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const api = serverApi(fetch);
  // Two independent boundary reads; run them together rather than in series.
  const [companyOffsets, jobOffsets] = await Promise.all([
    api.sitemapCompanyBoundaries(SITEMAP_CHUNK),
    api.sitemapJobBoundaries(JOB_SITEMAP_CHUNK),
  ]);

  const locs = [`${origin}/sitemap-pages.xml`, `${origin}/sitemap-insights.xml`];
  // The skills glossary is one file rather than a shard per letter: it is under a
  // thousand URLs and enumerating it reads nothing, so there is nothing to shard away
  // from — unlike the role landings below, where each shard pays its own facet call.
  //
  // Offered only once there is a glossary to offer. The definitions land in reviewed
  // waves, and a sitemap announcing a glossary of a handful of words describes
  // something this is not yet. It appears on its own when coverage does.
  if (isGlossaryPublished(SKILL_DESCRIBED.length)) {
    locs.push(`${origin}/sitemap-skills.xml`);
  }
  // One role sub-sitemap per category. The category list is a compile-time constant,
  // so naming all of them costs no read here — each shard pays its own single facet
  // call when a crawler actually follows it.
  for (const { slug } of landingCategories()) {
    locs.push(`${origin}/sitemap-roles.xml?category=${slug}`);
  }
  // Both cursor lists already include the opening 0, so they are listed as they come.
  for (const offset of jobOffsets) {
    locs.push(`${origin}/sitemap-jobs.xml?offset=${offset}`);
  }
  for (const offset of companyOffsets) {
    locs.push(`${origin}/sitemap-companies.xml?offset=${offset}`);
  }

  return xmlResponse(sitemapIndexXml(locs));
};
