// The page-scroll lock every overlay shares: Dialog, and the app's own drawers, sheets
// and hand-rolled modals. One module-level reference count for all of them, because two
// locks keeping their own counts each restore the body styles they read — and the one
// that reads second reads the other's "locked" values, leaving the page stuck.
//
// Callers pair lockScroll()/unlockScroll() and MUST release on cleanup (a Svelte
// `$effect` return), so an overlay unmounting while open never leaves the page frozen.
// Guarded for SSR: no-ops when `document` is absent.

const PINNED = ['position', 'top', 'left', 'right', 'overflow'] as const;

let count = 0;
let savedScrollX = 0;
let savedScrollY = 0;
let savedStyles: Record<(typeof PINNED)[number], string> | null = null;

/**
 * Stop the page behind an overlay from scrolling. Balanced by `unlockScroll`.
 *
 * `overflow: hidden` alone is not enough: iOS Safari still lets a touch drag scroll the
 * body underneath it. Pinning the body with `position: fixed` takes it out of the flow,
 * so there is nothing left for a drag to scroll; the `top`/`left` offsets keep the page
 * from visibly jumping, and `unlockScroll` puts back the exact scroll position and
 * whatever inline styles the body carried before.
 */
export function lockScroll(): void {
  if (typeof document === 'undefined') return;
  count += 1;
  if (count > 1) return;
  const body = document.body.style;
  savedStyles = { position: body.position, top: body.top, left: body.left, right: body.right, overflow: body.overflow };
  savedScrollX = window.scrollX;
  savedScrollY = window.scrollY;
  body.position = 'fixed';
  body.top = `-${savedScrollY}px`;
  body.left = `-${savedScrollX}px`;
  body.right = '0';
  body.overflow = 'hidden';
}

/** Release one lock; the last release restores scrolling, the body's styles and the position. */
export function unlockScroll(): void {
  if (typeof document === 'undefined' || count === 0) return;
  count -= 1;
  if (count > 0 || !savedStyles) return;
  Object.assign(document.body.style, savedStyles);
  savedStyles = null;
  window.scrollTo(savedScrollX, savedScrollY);
}
