// Keeps the header's overlays mutually exclusive: the notification bell's dropdown,
// the hamburger menu's drawer, and the search box's suggestion list can each open
// independently, but only one should be on screen at a time.
//
// A click-bubbling "outside click" check per component can't coordinate this: the
// bell is rendered inside the menu's own root wrapper (see HeaderMenu.svelte), so a
// click on the bell is never "outside" the menu's root regardless of propagation,
// and HeaderMenu's own toggle stops propagation entirely for an unrelated reason
// (its button's icon swaps on open, which would otherwise re-close it immediately).
// This module sidesteps click bubbling and DOM containment altogether: whichever
// overlay opens explicitly closes whatever was open before it.

let activeClose: (() => void) | null = null;

/** Call when one of the header's overlays opens: closes whichever other one was
 *  left open. Pair with `closedOverlay` (e.g. an `$effect` cleanup keyed on the
 *  overlay's own open state) so a later open doesn't invoke a stale closer. */
export function openedOverlay(close: () => void): void {
  if (activeClose && activeClose !== close) activeClose();
  activeClose = close;
}

/** Call when this overlay closes by its own means (outside click, Escape,
 *  navigation), so it isn't mistaken for still being the active one. */
export function closedOverlay(close: () => void): void {
  if (activeClose === close) activeClose = null;
}
