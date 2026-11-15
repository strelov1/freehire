import { keyboardFit, NO_FIT, type KeyboardFit } from '#lib/keyboardFit.js';

/** Where this panel may sit with the on-screen keyboard up: a lift for the one that is
 *  pinned to the bottom of the screen, a ceiling for the one that hangs off the box,
 *  and why one number cannot serve both — see #lib/keyboardFit.
 *
 *  Measured while the panel is `open`, from its `anchor` box; `bottomAnchored` says the
 *  panel is pinned to the bottom of the screen rather than hanging off the box. Called
 *  during component initialisation, since it registers an effect. */
export function trackKeyboardFit(
  open: () => boolean,
  anchor: () => HTMLElement | null,
  bottomAnchored: () => boolean,
): { readonly fit: KeyboardFit } {
  const state = $state({ fit: NO_FIT as KeyboardFit });
  $effect(() => {
    // Nothing to place while the panel is shut, and no listener to keep either.
    if (!open()) return;
    const viewport = window.visualViewport;
    if (!viewport) return;
    const box = anchor();
    if (!box) return;
    const measure = () => {
      state.fit = keyboardFit({
        windowHeight: window.innerHeight,
        viewportHeight: viewport.height,
        viewportOffsetTop: viewport.offsetTop,
        // The ceiling is measured from the bottom of the box the panel hangs off.
        anchorBottom: box.getBoundingClientRect().bottom,
        bottomAnchored: bottomAnchored(),
      });
    };
    measure();
    // `scroll` as well as `resize`: iOS scrolls the visual viewport inside the layout one
    // to keep the focused field visible, which moves the keyboard's top edge without
    // changing its height. The window's own scroll counts too — a ceiling is measured
    // from where the box IS, and scrolling the page moves it under a keyboard that has
    // not budged.
    viewport.addEventListener('resize', measure);
    viewport.addEventListener('scroll', measure);
    window.addEventListener('scroll', measure, { passive: true });
    return () => {
      viewport.removeEventListener('resize', measure);
      viewport.removeEventListener('scroll', measure);
      window.removeEventListener('scroll', measure);
      state.fit = NO_FIT;
    };
  });
  return state;
}
