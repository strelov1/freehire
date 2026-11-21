import { error, redirect } from '@sveltejs/kit';
import { PUBLIC_MATCHES } from '$app/env/public';
import { matchesEnabled } from '#lib/features.js';
import { signinUrl } from '#lib/signin.js';
import type { PageServerLoad } from './$types';

// The feed is personal and dark until it ships: off, the page does not exist; signed out,
// sign in and come back here.
export const load: PageServerLoad = async ({ parent, url }) => {
  if (!matchesEnabled({ PUBLIC_MATCHES })) error(404, 'Not found');
  const { user } = await parent();
  if (!user) redirect(302, signinUrl({ returnTo: url.pathname, cancelTo: '/', mode: 'login' }));
};
