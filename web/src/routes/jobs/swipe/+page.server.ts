import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import type { PageServerLoad } from './$types';

// Swipe mode is personal: both actions (save and dismiss) are per-user, and the
// deck endpoint is authenticated. A signed-out visitor has no deck to show, so
// guard server-side and send them to /signin — `returnTo` carries the intended
// destination (path + query), so a shared link like /jobs/swipe?seniority=senior
// survives sign-in and reopens the filtered deck.
export const load: PageServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
};
