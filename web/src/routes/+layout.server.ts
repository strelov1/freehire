import { ApiError } from '$lib/api';
import { hasSessionCookie } from '$lib/authCookie';
import { isLocale, LOCALE_COOKIE, type Locale } from '$lib/locale';
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
const SIGNED_OUT = { user: null, locale: 'en' as const };

export const load: LayoutServerLoad = async ({ fetch, request, cookies }) => {
  const cookie = request.headers.get('cookie');
  if (!hasSessionCookie(cookie)) return SIGNED_OUT;
  try {
    const user = await serverApi(fetch, cookie).me();
    const locale: Locale = isLocale(user.language) ? user.language : 'en';
    // Self-healing: re-synced on every request that resolves a signed-in user, so
    // a stale, absent, or cross-device value never survives more than one full
    // load. hooks.server.ts reads this cookie on the next request without a DB
    // round trip.
    cookies.set(LOCALE_COOKIE, locale, {
      path: '/',
      httpOnly: false,
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365,
    });
    return { user, locale };
  } catch (e) {
    if (e instanceof ApiError) return SIGNED_OUT;
    // A non-API failure (network/parse) shouldn't 500 the whole site over chrome.
    return SIGNED_OUT;
  }
};
