import { serverApi } from '$lib/server/api';
import { coveredCategories } from '$lib/insights';
import { insightsPaths, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// Sub-sitemap for the insights landing pages, referenced by the sitemap index. It
// lists only categories that clear the data-quality gate (via coveredCategories),
// so a thin/gated-out category never appears — matching what the pages actually
// serve (uncovered → 404).
export const GET: RequestHandler = async ({ url, fetch }) => {
  const roles = await serverApi(fetch).insightsRoles({ limit: 200 });
  const categories = coveredCategories(roles).map((c) => c.category);
  const entries = insightsPaths(categories).map((path) => ({ loc: `${url.origin}${path}` }));
  return xmlResponse(urlsetXml(entries));
};
