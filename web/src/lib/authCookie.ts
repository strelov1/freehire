// The API owns the session cookie (internal/auth/cookie.go, `CookieName`). It is
// httpOnly, so nothing here can read its value — only whether it is present.

/** Name of the session cookie the API sets. */
const SESSION_COOKIE = 'hire_token';

/** Whether a request's raw `Cookie` header carries a session.
 *
 *  Testing for THIS cookie rather than for any cookie is the point: analytics sets
 *  its own on first-time visitors, so "has cookies" is true for nearly everyone.
 *  Parsed per-pair rather than by substring so a cookie whose name merely ends in
 *  the same characters (`not_hire_token=…`) doesn't read as a session. */
export function hasSessionCookie(header: string | null | undefined): boolean {
  if (!header) return false;
  return header
    .split(';')
    .some((pair) => pair.trimStart().startsWith(`${SESSION_COOKIE}=`));
}
