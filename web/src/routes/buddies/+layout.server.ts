import { redirect } from '@sveltejs/kit';
import { signinUrl } from '#lib/signin.js';
import type { LayoutServerLoad } from './$types';

// The buddies space needs a session on every page, like /my (whose gate this mirrors): a
// signed-out visitor gets a 302 to sign-in before any buddies chrome renders, and comes
// straight back to the page they asked for.
export const load: LayoutServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
};
