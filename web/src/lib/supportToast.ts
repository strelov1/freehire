// Open-source support toast: the show condition and the dismissal, kept free of Svelte
// runes and SvelteKit imports so it unit-tests in the plain-node vitest environment. The
// markup lives in components/SupportToast.svelte.
//
// `./productHunt` is imported by relative path on purpose: the project's vitest setup
// does not resolve `$lib`, and an aliased import fails at module load rather than inside
// a test body.
//
// Unlike the Product Hunt strip this surface never renders on the server. The strip needs
// SSR plus a pre-paint class because it sits in the document flow; a fixed toast moves
// nothing, so it can appear on mount — which is why `app.html` stays untouched and the
// SHA-256 CSP hash over its inline script stays intact.

import { launchPhase, PH_DISMISSED_KEY } from './productHunt';

/** localStorage key holding the visitor's answer to the star request. */
export const SUPPORT_DISMISSED_KEY = 'hire.support-toast-dismissed';

/** What the show condition needs to know. Passed in rather than read inside, so the rule
 *  itself is pure and the storage reads stay at the edge. */
export type SupportToastState = {
  /** Now, in epoch milliseconds. */
  now: number;
  /** Whether the visitor has closed the Product Hunt strip. */
  phBannerDismissed: boolean;
  /** Whether the visitor has already answered this toast. */
  selfDismissed: boolean;
};

/**
 * Whether the site may ask for a star right now.
 *
 * Two rules meet here. An answered toast never returns. And the ask queues behind the
 * Product Hunt strip, which stops asking in two different ways: the visitor closes it, or
 * the launch day passes and it retires itself. Only the first leaves a key behind — after
 * the launch day the strip does not render and can never be closed, so gating on the key
 * alone would hide this toast from everyone who arrives later.
 */
export function shouldShow({ now, phBannerDismissed, selfDismissed }: SupportToastState): boolean {
  if (selfDismissed) return false;
  return phBannerDismissed || launchPhase(now) === 'over';
}

/** Whether the visitor has answered the ask. */
export function readDismissed(): boolean {
  return readFlag(SUPPORT_DISMISSED_KEY);
}

/** Whether the visitor has closed the Product Hunt strip. Reads that surface's own key
 *  rather than restating the literal, so a rename there breaks the build here instead of
 *  leaving a toast that silently waits until after the launch day. */
export function readPhBannerDismissed(): boolean {
  return readFlag(PH_DISMISSED_KEY);
}

/** Record that the visitor answered — by closing the toast, or by following the link.
 *  Silently a no-op when storage is unavailable; the toast then returns on the next page
 *  load. */
export function writeDismissed(): void {
  try {
    localStorage.setItem(SUPPORT_DISMISSED_KEY, '1');
  } catch {
    /* storage unavailable; the dismissal lasts for this page only */
  }
}

/** Unavailable storage (Safari private mode, a blocked origin) reads as "not set" —
 *  showing a toast to someone who closed it is a smaller harm than a throw in the
 *  layout. */
function readFlag(key: string): boolean {
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
}
