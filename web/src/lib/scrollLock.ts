// Body-scroll lock shared by the header's mobile overlays (search dropdown and
// menu). Both can request a lock; a small reference count means the body only
// unlocks once every requester has released, so closing one overlay while the
// other is still open doesn't restore background scroll prematurely.
//
// Callers pair lock()/unlock() and MUST release on cleanup (e.g. a Svelte
// `$effect` return), so an overlay unmounting while open never leaves the page
// stuck. Guarded for SSR: no-ops when `document` is absent.

let count = 0;
let savedScrollX = 0;
let savedScrollY = 0;

/**
 * Prevent the page body from scrolling. Balanced by `unlockScroll`.
 *
 * `overflow: hidden` alone is not enough: iOS Safari still lets a touch drag
 * scroll the body underneath it, which is exactly the gap that let background
 * content show up behind a full-screen mobile overlay (see NotificationBell).
 * Pinning the body with `position: fixed` removes it from the document flow
 * entirely, so there is nothing left for a touch drag to scroll — the `top`/
 * `left` offsets compensate so the page doesn't visibly jump, and
 * `unlockScroll` restores the exact scroll position on both axes.
 */
export function lockScroll(): void {
  if (typeof document === 'undefined') return;
  count += 1;
  if (count === 1) {
    savedScrollX = window.scrollX;
    savedScrollY = window.scrollY;
    document.body.style.position = 'fixed';
    document.body.style.top = `-${savedScrollY}px`;
    document.body.style.left = `-${savedScrollX}px`;
    document.body.style.right = '0';
    document.body.style.overflow = 'hidden';
  }
}

/** Release one scroll lock; restores scrolling and position when the last one is released. */
export function unlockScroll(): void {
  if (typeof document === 'undefined') return;
  if (count === 0) return;
  count -= 1;
  if (count === 0) {
    document.body.style.position = '';
    document.body.style.top = '';
    document.body.style.left = '';
    document.body.style.right = '';
    document.body.style.overflow = '';
    window.scrollTo(savedScrollX, savedScrollY);
  }
}
