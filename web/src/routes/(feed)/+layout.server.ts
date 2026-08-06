import { serverApi } from '$lib/server/api';
import type { LayoutServerLoad } from './$types';

const LIMIT = 20;

// Owns the job list SSR fetch for every route this layout wraps (`/` and
// `/jobs/[slug]`), so the list column is server-rendered whichever one the visitor
// lands on directly. Deliberately NOT the first-visit redirect (see `(feed)/+page.server.ts`)
// — that must fire only for the bare homepage, never for a shared `/jobs/[slug]` link.
export const load: LayoutServerLoad = async ({ url, fetch }) => {
  const initial = await serverApi(fetch).searchJobs(url.searchParams, LIMIT, 0);
  return { initial };
};
