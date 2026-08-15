import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { LayoutServerLoad } from './$types';

// The session cookie's name. The API owns it (internal/auth/cookie.go,
// `CookieName`) and it is httpOnly, so the SPA never reads its value — but the
// layout does need to know whether one is present at all; see below.
const SESSION_COOKIE = 'hire_token';

// Resolve the current user once per full load, forwarding the request's session
// cookie, so the signed-in/out chrome renders correctly server-side (no /me
// flash). Exposed as `page.data.user` everywhere; re-runs on `invalidateAll()`
// after login/logout. A missing/expired session is simply signed out — never an
// error page.
//
// A request carrying no session cookie skips the call. Without this, every
// anonymous page load spent a round trip asking the API who the visitor is, only
// to be told 401 and render signed-out chrome anyway — measured at 7.7ms of a
// ~74ms homepage response on prod, where anonymous traffic is the large majority
// (crawlers alone were 83% of requests). Testing for this cookie specifically,
// rather than for any cookie, is deliberate: analytics sets its own cookies on
// first-time visitors, so "has cookies" is true for nearly everyone and would
// skip nothing.
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
