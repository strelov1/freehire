import { ApiError } from '$lib/api';
import { hasSessionCookie } from '$lib/authCookie';
import { serverApi } from '$lib/server/api';
import type { LayoutServerLoad } from './$types';

// Resolve the current user once per full load, forwarding the request's session
// cookie, so the signed-in/out chrome renders correctly server-side (no /me
// flash). Exposed as `page.data.user` everywhere; re-runs on `invalidateAll()`
// after login/logout. A missing/expired session is simply signed out — never an
// error page.
//
// A request with no session cookie skips the call: it would 401 and render
// signed-out chrome anyway, and anonymous traffic is most of ours (see
// $lib/authCookie for why the test is for THIS cookie, not for any cookie).
export const load: LayoutServerLoad = async ({ fetch, request }) => {
  const cookie = request.headers.get('cookie');
  if (!hasSessionCookie(cookie)) return { user: null };
  try {
    const user = await serverApi(fetch, cookie).me();
    return { user };
  } catch (e) {
    if (e instanceof ApiError) return { user: null };
    // A non-API failure (network/parse) shouldn't 500 the whole site over chrome.
    return { user: null };
  }
};
