import { describe, expect, it } from 'vitest';
import { keyboardFit, NO_FIT, type PanelGeometry } from './keyboardFit';

/** An iPhone-shaped phone with the keyboard up: the window stays 844 tall, the visual
 *  viewport is the 508 above the keys, and the box the panel hangs off ends 300px down. */
const phone: PanelGeometry = {
  windowHeight: 844,
  viewportHeight: 508,
  viewportOffsetTop: 0,
  anchorBottom: 300,
  bottomAnchored: true,
};

/** The same phone with the panel hanging off the box — the hero on the homepage, and
 *  either size at `sm` and up. */
const hanging: PanelGeometry = { ...phone, bottomAnchored: false };

describe('keyboardFit', () => {
  it('lifts a bottom-anchored panel by the height the keyboard covers', () => {
    expect(keyboardFit(phone)).toEqual({ lift: 336, ceiling: 0 });
  });

  it('never lifts a hanging panel — it has no bottom edge to lift', () => {
    // The regression: a `bottom` on a panel positioned by `top` inside a wrapper as tall
    // as the box computes a negative height, and the browser clamps it to zero.
    expect(keyboardFit(hanging).lift).toBe(0);
  });

  it('caps a hanging panel at the room between the box and the keyboard', () => {
    // 508 visible − 300 to the bottom of the box − the 16px gap.
    expect(keyboardFit(hanging).ceiling).toBe(192);
  });

  it('subtracts the visual viewport offset iOS scrolls the field into view by', () => {
    expect(keyboardFit({ ...phone, viewportOffsetTop: 60 }).lift).toBe(276);
    expect(keyboardFit({ ...hanging, viewportOffsetTop: 60 }).ceiling).toBe(252);
  });

  it('keeps a floor under a hanging panel when the box sits on the keys', () => {
    expect(keyboardFit({ ...hanging, anchorBottom: 505 }).ceiling).toBe(120);
  });

  it('never raises the cap the stylesheet already put on a hanging panel', () => {
    // 70vh of 844 is 590.8; a box 100px down leaves 728, so an inline ceiling would make
    // the panel TALLER than the class allows. Nothing to tighten, nothing to say.
    expect(keyboardFit({ ...hanging, viewportHeight: 844, anchorBottom: 100 })).toBe(NO_FIT);
  });

  it('reads a difference too small to be a keyboard as no keyboard', () => {
    // Fractional visual-viewport heights and collapsing browser chrome, not keys.
    expect(keyboardFit({ ...phone, viewportHeight: 843.5 })).toBe(NO_FIT);
    expect(keyboardFit({ ...hanging, viewportHeight: 843.5 })).toBe(NO_FIT);
  });

  it('still lifts off a hardware keyboard accessory bar', () => {
    // iOS leaves ~55px for it, which is a real thing to clear.
    expect(keyboardFit({ ...phone, viewportHeight: 789 }).lift).toBe(55);
  });

  it('does nothing with no keyboard up', () => {
    expect(keyboardFit({ ...phone, viewportHeight: 844 })).toBe(NO_FIT);
    expect(keyboardFit({ ...hanging, viewportHeight: 844 })).toBe(NO_FIT);
  });
});
