import { STATIC_PATHS, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// Sub-sitemap for the site's static pages, referenced by the sitemap index.
export const GET: RequestHandler = ({ url }) => {
  const entries = STATIC_PATHS.map((path) => ({ loc: `${url.origin}${path}` }));
  return xmlResponse(urlsetXml(entries));
};
