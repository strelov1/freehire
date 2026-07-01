import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// Swipe mode is personal: both actions (save and dismiss) are per-user, and the
// deck endpoint is authenticated. A signed-out visitor has no deck to show, so
// guard server-side and bounce home with ?auth=required — the TopBar pops the
// sign-in dialog (same pattern as /my/jobs). We also carry the intended
// destination (path + query) as ?redirect, so a shared link like
// /jobs/swipe?seniority=senior survives sign-in and reopens the filtered deck.
export const load: PageServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    const target = url.pathname + url.search;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
};
