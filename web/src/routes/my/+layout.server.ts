import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import type { LayoutServerLoad } from './$types';

// The auth gate for the whole account section. Every /my/** page needs a session, so
// the check lives here once rather than as an in-page "Sign in to access your account"
// placeholder: an anonymous visitor gets a 302 to the sign-in form before any account
// chrome renders, and comes straight back to the page they asked for.
//
// It also covers a sign-out that happens while sitting on one of these pages —
// logout() calls invalidateAll(), which re-runs this load and redirects.
export const load: LayoutServerLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
};
