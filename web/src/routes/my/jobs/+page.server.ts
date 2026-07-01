import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// /my/jobs is personal: a signed-out visitor has nothing to show, so guard it
// server-side rather than render an empty "sign in" state. The user is resolved
// once in the root layout load; reuse it via parent(). Auth is a layout-level
// dialog (no /login route), so we bounce home with ?auth=required and the TopBar
// pops the sign-in dialog (same pattern as the ?auth_error OAuth callback). The
// ?redirect carries the destination so sign-in returns here, not the home page.
export const load: PageServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    const target = url.pathname + url.search;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
};
