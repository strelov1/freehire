// CLI promo strip: dismissal persistence, kept free of Svelte runes and SvelteKit
// imports so it unit-tests in the plain-node vitest environment. The markup lives in
// components/CliBanner.svelte.
//
// It replaced the Product Hunt launch strip, which carried a clock: its wording moved
// itself from "launches on the 26th" to "live today" and then retired, so nobody had to
// ship a web build on launch day. Nothing here needs that machinery — the strip points
// at a page that does not expire — so the only thing that stops it is the visitor
// closing it.

/** Cookie recording that the visitor closed the strip.
 *
 *  A session cookie (no Max-Age) on purpose: closing the strip quiets it for this
 *  browser session only, and it asks again in the next one until the account connects an
 *  agent (`User.agent_connected`, which removes it for good). Not sessionStorage, which
 *  is per TAB — every job opened in a new tab would ask again. It used to be a
 *  localStorage flag kept forever; that key is simply no longer read.
 *
 *  Deliberately NOT the Product Hunt strip's `hire.ph-banner-dismissed`. */
export const CLI_BANNER_DISMISSED_COOKIE = 'hire_cli_banner_dismissed';

/** Whether the visitor has closed the strip this session. Unreadable cookies (a blocked
 *  origin) read as "not dismissed" — showing a strip to someone who closed it is a
 *  smaller harm than a thrown error in the layout. */
export function readDismissed(): boolean {
  try {
    return document.cookie.split('; ').includes(`${CLI_BANNER_DISMISSED_COOKIE}=1`);
  } catch {
    return false;
  }
}

/** Record that the visitor closed the strip, until the browser session ends. Silently a
 *  no-op when cookies are unavailable — the strip then simply returns on the next page
 *  load. */
export function writeDismissed(): void {
  try {
    document.cookie = `${CLI_BANNER_DISMISSED_COOKIE}=1; Path=/; SameSite=Lax`;
  } catch {
    /* cookies unavailable; the dismissal lasts for this page only */
  }
}
