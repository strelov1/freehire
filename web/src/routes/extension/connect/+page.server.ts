import { error, redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import type { PageServerLoad } from './$types';

// The sign-in stop on the browser-extension connect flow.
//
// The extension opens the API's /auth/extension/connect in Chrome's auth window, whose
// cookie jar is not the browsing profile's — so the usual first run arrives with no
// session at all. The API sends that visitor here rather than refusing it: this page
// signs them in (the same /signin every guarded page sends a signed-out visitor to)
// and hands the window straight back, now carrying a session cookie.
//
// The handoff is a real navigation from the browser (see +page.svelte), not a redirect
// from this load: the target is the Go API, not a route this app knows, and only a full
// navigation is guaranteed to leave the client router.
export const load: PageServerLoad = async ({ parent, url }) => {
  const redirectUri = url.searchParams.get('redirect_uri');
  const state = url.searchParams.get('state') ?? '';
  // Reached without the extension's parameters — nothing to connect.
  if (!redirectUri) {
    error(400, 'This page is opened by the freehire browser extension.');
  }

  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }

  // `via=web` tells the API this request already came through here: if it still cannot
  // see a session it must say so rather than send the window back for another round.
  const params = new URLSearchParams({ redirect_uri: redirectUri, state, via: 'web' });
  return { apiUrl: `/api/v1/auth/extension/connect?${params}` };
};
