// CLI promo strip: dismissal persistence, kept free of Svelte runes and SvelteKit
// imports so it unit-tests in the plain-node vitest environment. The markup lives in
// components/CliBanner.svelte.
//
// It replaced the Product Hunt launch strip, which carried a clock: its wording moved
// itself from "launches on the 26th" to "live today" and then retired, so nobody had to
// ship a web build on launch day. Nothing here needs that machinery — the strip points
// at a page that does not expire — so the only thing that stops it is the visitor
// closing it.

/** localStorage key holding the visitor's dismissal of the strip.
 *
 *  Deliberately NOT the Product Hunt strip's `hire.ph-banner-dismissed`: reusing that
 *  key would hide this strip, silently and for good, from every visitor who ever closed
 *  the launch announcement. */
export const CLI_BANNER_DISMISSED_KEY = 'hire.cli-banner-dismissed';

/** Whether the visitor has closed the strip. Absent or unreadable storage (Safari
 *  private mode, a blocked origin) reads as "not dismissed" — showing a strip to
 *  someone who closed it is a smaller harm than a thrown error in the layout. */
export function readDismissed(): boolean {
  try {
    return localStorage.getItem(CLI_BANNER_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

/** Record that the visitor closed the strip. Silently a no-op when storage is
 *  unavailable — the strip then simply returns on the next page load. */
export function writeDismissed(): void {
  try {
    localStorage.setItem(CLI_BANNER_DISMISSED_KEY, '1');
  } catch {
    /* storage unavailable; the dismissal lasts for this page only */
  }
}
