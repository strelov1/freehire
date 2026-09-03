// Open-source support toast: the route rules and the dismissal, kept free of Svelte
// runes and SvelteKit imports so it unit-tests in the plain-node vitest environment. The
// markup lives in components/SupportToast.svelte.
//
// Unlike the CLI strip this surface never renders on the server. The strip needs SSR plus
// a pre-paint class because it sits in the document flow; a fixed toast moves nothing, so
// it can appear on mount — which is why `app.html` and the SHA-256 CSP hash over its
// inline script do not concern it.
//
// It used to queue behind the Product Hunt launch strip, which was a strip with an end
// date: waiting for it cost the toast a few weeks. The CLI strip that replaced it never
// expires, so the same rule would have retired this ask for good. The two surfaces are
// now independent — the strip is a band under the header, the toast a card in the bottom
// corner, and they do not overlap. What the toast still yields to is the consent banner,
// which shares its corner, and a page's own sticky call to action; both rules are below.

/** localStorage key holding the visitor's answer to the star request. */
export const SUPPORT_DISMISSED_KEY = 'hire.support-toast-dismissed';

/** Pages that make this exact case at length already, where a toast repeating it is
 *  noise. */
export function suppressesToast(pathname: string): boolean {
  return pathname === '/open';
}

/**
 * Whether the page owns a sticky call to action anchored to the bottom of a narrow
 * viewport — currently the job page's mobile Apply bar, which is `lg:hidden` and sits on
 * the same layer in the same corner.
 *
 * A promo must never cover a page's own primary action, so on these routes the toast is
 * shown from `lg` up only, where the bar is not rendered. The rule deliberately matches
 * any single segment under `/jobs/`: keeping a list of static siblings in step with the
 * router would be more fragile than occasionally hiding a promo on a phone.
 */
export function ownsMobileStickyCta(pathname: string): boolean {
  return /^\/jobs\/[^/]+$/.test(pathname);
}

/** Whether the visitor has answered the ask. */
export function readDismissed(): boolean {
  return readFlag(SUPPORT_DISMISSED_KEY);
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
