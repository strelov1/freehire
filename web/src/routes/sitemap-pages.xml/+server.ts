import { listPosts } from '$lib/blogPosts';
import { STATIC_PATHS, blogPaths, collectionPaths, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { PathEntry } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// Sub-sitemap for the site's static pages plus the curated collection landing
// pages and the blog (index + published posts), referenced by the sitemap index.
//
// Only the blog carries a lastmod: a post has a real publish date, while a static
// page changes with a deploy and a collection with the catalogue behind it —
// neither has a date we can state honestly, and a made-up one teaches crawlers to
// ignore the field everywhere.
export const GET: RequestHandler = ({ url }) => {
  const undated: PathEntry[] = [...STATIC_PATHS, ...collectionPaths()].map((path) => ({ path }));
  const entries = [...undated, ...blogPaths(listPosts())].map(({ path, lastmod }) => ({
    loc: `${url.origin}${path}`,
    lastmod,
  }));
  return xmlResponse(urlsetXml(entries));
};
