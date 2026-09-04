import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import { loadBoard } from '$lib/server/tracking';
import type { PageServerLoad } from './$types';

// /my/tracking is personal: a signed-out visitor has nothing to show, so guard it
// server-side rather than render an empty "sign in" state. The user is resolved
// once in the root layout load; reuse it via parent(). `returnTo` carries the
// destination so sign-in returns here, not wherever /signin's own default is.
// `cancelTo: '/'` matters here specifically: without it, /signin's close button
// would default to THIS same guarded page, which redirects right back to
// /signin — a loop. `mode: 'login'` because a visitor bounced off a guarded page
// already has an account; the register form is the wrong default for them.
export const load: PageServerLoad = async ({ parent, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
  // Fetch the board here so it renders with the page, not after a client fetch on
  // mount (see loadBoard for the waterfall it replaces + the failure fallback).
  return { board: await loadBoard(fetch, request.headers.get('cookie')) };
};
