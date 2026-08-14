import type { InsightRole } from '$lib/api';
import { serverApi } from './api';

/** The global roles ranking that gates every insights landing page — the hub, the
 *  three per-category pages (salary/roles/skills), and the sitemap all decide what
 *  to show (or 404) from the same `coveredCategories`/`isCovered` read over this
 *  set, so they all fetch it identically: `limit: 200` and no other filter. Shared
 *  so the five call sites can't quietly drift onto different limits. */
export function loadInsightsGate(fetchImpl: typeof fetch): Promise<InsightRole[]> {
  return serverApi(fetchImpl).insightsRoles({ limit: 200 });
}
