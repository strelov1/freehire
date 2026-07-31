import { describe, it, expect } from 'vitest';
import { atBottom, BOTTOM_TOLERANCE_PX } from './scrolling';

describe('atBottom', () => {
  it('is true when the pane is scrolled to its exact end', () => {
    expect(atBottom({ scrollHeight: 1000, scrollTop: 600, clientHeight: 400 })).toBe(true);
  });

  it('is true within the tolerance, because the last line grows under the reader', () => {
    // A streaming answer appends to its final line: the pane gets taller while the
    // reader has not moved, so an exact test would stop following on its own content.
    expect(
      atBottom({ scrollHeight: 1000, scrollTop: 600 - BOTTOM_TOLERANCE_PX, clientHeight: 400 }),
    ).toBe(true);
  });

  it('is false once the reader scrolls past the tolerance', () => {
    expect(
      atBottom({ scrollHeight: 1000, scrollTop: 600 - BOTTOM_TOLERANCE_PX - 1, clientHeight: 400 }),
    ).toBe(false);
  });

  it('is false when the reader is far up a long transcript', () => {
    expect(atBottom({ scrollHeight: 5000, scrollTop: 0, clientHeight: 400 })).toBe(false);
  });

  it('is true for a pane shorter than its viewport, which cannot be scrolled at all', () => {
    expect(atBottom({ scrollHeight: 300, scrollTop: 0, clientHeight: 400 })).toBe(true);
  });

  it('is true when a bounce scroll overshoots the end', () => {
    // Momentum scrolling on macOS and iOS reports a scrollTop past the real maximum.
    expect(atBottom({ scrollHeight: 1000, scrollTop: 640, clientHeight: 400 })).toBe(true);
  });
});
