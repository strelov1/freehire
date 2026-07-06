import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// /my/recommendations is personal (a per-user CV-ranked feed), so a signed-out
// visitor has nothing to show — guard it server-side rather than render an empty
// state, matching /my/jobs. The user is resolved once in the root layout load;
// reuse it via parent(). Auth is a layout-level dialog (no /login route), so we
// bounce home with ?auth=required and the TopBar pops the sign-in dialog; ?redirect
// carries the destination so sign-in returns here.
export const load: PageServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    const target = url.pathname + url.search;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
};
