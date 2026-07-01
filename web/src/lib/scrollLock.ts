// Body-scroll lock shared by the header's mobile overlays (search dropdown and
// menu). Both can request a lock; a small reference count means the body only
// unlocks once every requester has released, so closing one overlay while the
// other is still open doesn't restore background scroll prematurely.
//
// Callers pair lock()/unlock() and MUST release on cleanup (e.g. a Svelte
// `$effect` return), so an overlay unmounting while open never leaves the page
// stuck. Guarded for SSR: no-ops when `document` is absent.

let count = 0;

/** Prevent the page body from scrolling. Balanced by `unlockScroll`. */
export function lockScroll(): void {
  if (typeof document === 'undefined') return;
  count += 1;
  if (count === 1) document.body.style.overflow = 'hidden';
}

/** Release one scroll lock; restores scrolling when the last one is released. */
export function unlockScroll(): void {
  if (typeof document === 'undefined') return;
  if (count === 0) return;
  count -= 1;
  if (count === 0) document.body.style.overflow = '';
}
