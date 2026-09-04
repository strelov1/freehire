// How much horizontal space the docked assistant is taking, so the account shell can
// step aside for it.
//
// The panel is opened from inside the Experience tab and the offset has to be applied by
// `my/+layout.svelte`, on the other side of a route boundary a page cannot pass props
// through. A module-level $state singleton, safe under SSR because it is a client-only
// UI concern that defaults to nothing and is only ever mutated from browser
// interactions.

/**
 * The docked panel's width. Fixed rather than measured: the shell's offset and the media
 * query below are derived from it, and a value that changed at runtime would make the
 * threshold that keeps the bank wide unverifiable.
 */
export const DOCK_WIDTH = 360;

/**
 * Where docking starts paying for itself, and the number is arithmetic rather than taste.
 *
 * Closed, the bank gets the shell's 1152 cap, less its 32px of padding, less the 224px
 * section nav and the 32px gap: 864px. Docked, the shell has `viewport − 360` to work
 * with, and the nav collapses to its 56px rail (see `my/+layout.svelte`), so the bank gets
 * `viewport − 360 − 32 − 56 − 32`. That reaches 864 at a 1344px viewport.
 *
 * 85rem leaves a little slack above it, and keeps the dock available on the 1440 and 1536
 * laptops most people are on. Below it there is no arrangement where both columns are
 * comfortable, so the panel covers the bank instead.
 */
export const DOCKED_QUERY = '(min-width: 85rem)';

let offset = $state(0);

/** The width the shell must yield on its left, or 0 when nothing is docked. */
export function dockOffset(): number {
  return offset;
}

/**
 * Claim or release the shell's left margin. Called by the panel, which owns the media
 * query and knows whether it is currently a dock or a covering overlay — an overlay takes
 * no space, so it must not move the page behind it.
 */
export function setDockOffset(px: number) {
  offset = px;
}
