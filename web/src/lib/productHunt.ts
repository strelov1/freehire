// Product Hunt launch banner: pure phase logic and dismissal persistence, kept free
// of Svelte runes and SvelteKit imports so it unit-tests in the plain-node vitest
// environment. The markup lives in components/ProductHuntBanner.svelte.
//
// The phase is derived from the clock rather than from a flag someone flips, so the
// banner switches itself from "we launch on the 26th" to "we are live today" and then
// disappears — with no deploy on launch day, which is exactly the day not to be
// shipping a web build.

/** When the Product Hunt day opens: 00:00 PDT on 26 August 2026 (07:00 UTC).
 *  Product Hunt runs its day on Pacific time, so UTC midnight is seven hours early. */
export const LAUNCH_OPENS_AT = Date.UTC(2026, 7, 26, 7, 0, 0);

/** A Product Hunt day is 24 hours from the moment it opens. */
const DAY_MS = 24 * 60 * 60 * 1000;

/** localStorage key holding the visitor's dismissal of the banner. */
export const PH_DISMISSED_KEY = 'hire.ph-banner-dismissed';

/** Where the banner points. Carries its own campaign so the launch-day traffic is
 *  separable from the footer badge's. */
export const PH_URL =
  'https://www.producthunt.com/products/freehire?utm_source=freehire&utm_medium=banner&utm_campaign=launch';

/** Which message the banner should carry, or `over` once the launch day has passed
 *  and it should not render at all. */
export type LaunchPhase = 'before' | 'live' | 'over';

/** Classify a moment against the launch window. */
export function launchPhase(now: number): LaunchPhase {
  if (now < LAUNCH_OPENS_AT) return 'before';
  if (now < LAUNCH_OPENS_AT + DAY_MS) return 'live';
  return 'over';
}

/** Whether the visitor has closed the banner. Absent or unreadable storage (Safari
 *  private mode, a blocked origin) reads as "not dismissed" — showing a banner to
 *  someone who closed it is a smaller harm than a thrown error in the layout. */
export function readDismissed(): boolean {
  try {
    return localStorage.getItem(PH_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

/** Record that the visitor closed the banner. Silently a no-op when storage is
 *  unavailable — the banner then simply returns on the next page load. */
export function writeDismissed(): void {
  try {
    localStorage.setItem(PH_DISMISSED_KEY, '1');
  } catch {
    /* storage unavailable; the dismissal lasts for this page only */
  }
}
