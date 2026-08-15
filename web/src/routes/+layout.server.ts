import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { LayoutServerLoad } from './$types';

// The API owns this cookie (internal/auth/cookie.go, `CookieName`). It is
// httpOnly, so the SPA never reads its value — only whether it is there at all.
const SESSION_COOKIE = 'hire_token';

// Resolve the current user once per full load, forwarding the request's session
// cookie, so the signed-in/out chrome renders correctly server-side (no /me
// flash). Exposed as `page.data.user` everywhere; re-runs on `invalidateAll()`
// after login/logout. A missing/expired session is simply signed out — never an
// error page.
//
// A request with no session cookie skips the call: it would 401 and render
// signed-out chrome anyway, and anonymous traffic is most of ours. Testing for
// THIS cookie rather than for any cookie is the point — analytics sets its own
// on first-time visitors, so "has cookies" is true for nearly everyone.
export const load: LayoutServerLoad = async ({ fetch, request }) => {
  const cookie = request.headers.get('cookie');
  if (!cookie?.includes(`${SESSION_COOKIE}=`)) return { user: null };
  try {
    const user = await serverApi(fetch, cookie).me();
    return { user };
  } catch (e) {
    if (e instanceof ApiError) return { user: null };
    // A non-API failure (network/parse) shouldn't 500 the whole site over chrome.
    return { user: null };
  }
};
