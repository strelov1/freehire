// Where a dropdown panel may sit while the on-screen keyboard is up.
//
// Neither mobile browser shrinks the PAGE for the keyboard on its own — it is drawn over
// the bottom of a viewport that stays full height — so a panel reaching the bottom edge
// runs under the keys with its last rows unreachable. `visualViewport` is the part of the
// page actually left visible, so the difference between it and the window is the keyboard.
//
// The answer is not one number, because a panel is placed one of two ways and only one of
// them has a bottom edge to lift. The search box's phone panel is pinned from under the
// header to `bottom: 0`, so it moves UP. Every other way that box draws it — the hero on
// the homepage, and both sizes at `sm` and up — hangs it off the box with a `top` and no
// bottom, inside a wrapper as tall as the box; giving THAT a bottom makes the browser
// compute its height as the distance between the two, a negative number clamped to zero,
// which is a search box answering with a hairline. It gets a ceiling instead.

/** The gap between the box and the panel (`mt-2`), plus the same again so the last row
 *  does not sit flush against the keys. */
const PANEL_GAP = 16;

/** The shortest panel worth drawing — roughly two rows and the scroll that follows them.
 *  A box sitting right on the keyboard's top edge would otherwise be given a ceiling of
 *  nothing, which is the failure this module exists to prevent rather than a tighter fit. */
const MIN_CEILING = 120;

/** The stylesheet's own cap on the panel (`max-h-[70vh]`), as a fraction of the window.
 *  Written twice on purpose — it is not applied here, only compared against, so that a
 *  ceiling can never LOOSEN what the class already said. See `keyboardFit`. */
const PANEL_CAP = 0.7;

/** Below this, the difference is not a keyboard. `visualViewport.height` is fractional and
 *  browser chrome collapsing moves it by a pixel or two; well under the ~55px iOS leaves
 *  for a hardware keyboard's accessory bar, which IS worth lifting a panel off. */
const KEYBOARD_FLOOR = 24;

export type PanelGeometry = {
  /** `window.innerHeight` — the layout viewport, which the keyboard does not shrink. */
  windowHeight: number;
  /** `visualViewport.height` — what is actually visible. */
  viewportHeight: number;
  /** `visualViewport.offsetTop` — iOS scrolls the visual viewport inside the layout one
   *  to keep the focused field visible, which moves the keyboard's top edge without
   *  changing its height. */
  viewportOffsetTop: number;
  /** The bottom edge of the box the panel hangs off, in layout coordinates. */
  anchorBottom: number;
  /** Whether the panel is pinned to the bottom of the screen, rather than hanging off the
   *  box. The one thing that decides which of the two answers below applies — and not the
   *  same question as "is this a phone", which is what the old measurement asked. */
  bottomAnchored: boolean;
};

export type KeyboardFit = {
  /** How far to lift a bottom-anchored panel off the keyboard, in px. 0 leaves it alone. */
  lift: number;
  /** How tall a panel hanging off the box may be, in px. 0 leaves it to the stylesheet. */
  ceiling: number;
};

/** Leave the panel where the stylesheet put it. Exported so the caller's idle state is
 *  this same answer rather than a second literal that could drift from it. */
export const NO_FIT: Readonly<KeyboardFit> = { lift: 0, ceiling: 0 };

export function keyboardFit(geometry: PanelGeometry): KeyboardFit {
  const covered =
    geometry.windowHeight - geometry.viewportHeight - geometry.viewportOffsetTop;
  // `app.html` also asks Chrome to shrink the page itself
  // (`interactive-widget=resizes-content`) and the two must not double up: where the
  // browser honours that, the window shrinks with it and this measures nothing.
  if (covered < KEYBOARD_FLOOR) return NO_FIT;
  if (geometry.bottomAnchored) return { lift: covered, ceiling: 0 };

  const room =
    geometry.viewportOffsetTop + geometry.viewportHeight - geometry.anchorBottom - PANEL_GAP;
  // An inline `max-height` beats the class, so a ceiling taller than the stylesheet's own
  // cap would quietly RAISE it — the panel would grow because a keyboard appeared, which
  // is the opposite of the point. Nothing to tighten means nothing to say.
  if (room >= geometry.windowHeight * PANEL_CAP) return NO_FIT;
  return { lift: 0, ceiling: Math.max(MIN_CEILING, room) };
}
