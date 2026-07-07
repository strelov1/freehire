import { STATIC_PATHS, collectionPaths, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// Sub-sitemap for the site's static pages plus the curated collection landing
// pages, referenced by the sitemap index.
export const GET: RequestHandler = ({ url }) => {
  const paths = [...STATIC_PATHS, ...collectionPaths()];
  const entries = paths.map((path) => ({ loc: `${url.origin}${path}` }));
  return xmlResponse(urlsetXml(entries));
};
