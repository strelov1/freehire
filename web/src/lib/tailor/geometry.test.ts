import { describe, it, expect } from 'vitest';
import {
  clampWidth,
  paginateBlocks,
  stepMargin,
  MARGIN_MIN,
  MARGIN_MAX,
  previewTypography,
  stepFontSize,
  FONT_SIZE_MIN,
  FONT_SIZE_MAX,
  PREVIEW_FONT_SIZE_PX,
  PREVIEW_LINE_HEIGHT,
  axisValue,
  stepAxis,
} from './geometry';

describe('previewTypography', () => {
  // An unset style must leave the preview exactly as it has always rendered, so an existing
  // CV paginates identically. This is the browser-side half of the zero-means-inherit rule.
  it('falls back to the preview defaults when nothing is set', () => {
    const t = previewTypography({}, '');
    expect(t.fontSizePx).toBe(PREVIEW_FONT_SIZE_PX);
    expect(t.lineHeight).toBe(PREVIEW_LINE_HEIGHT);
    expect(t.fontFamily).toBe('');
  });

  it('converts a set point size to CSS pixels at 96dpi', () => {
    expect(previewTypography({ font_size: 12 }, '').fontSizePx).toBe(16);
    expect(previewTypography({ font_size: 9 }, '').fontSizePx).toBe(12);
  });

  // The calibration is what keeps preview and PDF agreeing: Typst's leading is the gap between
  // lines, CSS line-height is the whole line box. If this drifts, the preview paginates in one
  // metric and the PDF in another, and nothing reports the disagreement.
  it('maps the templates own leading onto the line height the preview already used', () => {
    expect(previewTypography({ line_height: 0.5 }, '').lineHeight).toBe(PREVIEW_LINE_HEIGHT);
  });

  it('maps a looser leading to a looser line height', () => {
    expect(previewTypography({ line_height: 0.7 }, '').lineHeight).toBeCloseTo(1.575, 5);
  });

  it('passes the font stack through', () => {
    expect(previewTypography({ font_family: 'carlito' }, 'Carlito, Calibri, sans-serif').fontFamily).toBe(
      'Carlito, Calibri, sans-serif',
    );
  });

  it('ignores a stack when no family is set', () => {
    expect(previewTypography({}, 'Carlito, Calibri, sans-serif').fontFamily).toBe('');
  });
});

describe('margin axes', () => {
  const uniform = { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 };

  it('reads a uniform axis as its shared value', () => {
    expect(axisValue(uniform, 'sides')).toBe(0.5);
    expect(axisValue(uniform, 'ends')).toBe(0.5);
  });

  // The linked view must say so rather than pick one side and quietly overwrite the other on
  // the next step.
  it('reads a non-uniform axis as null', () => {
    expect(axisValue({ ...uniform, left: 0.4 }, 'sides')).toBeNull();
    expect(axisValue({ ...uniform, top: 1.0 }, 'ends')).toBeNull();
  });

  it('steps both sides of an axis and leaves the other alone', () => {
    expect(stepAxis(uniform, 'sides', 0.05)).toEqual({ top: 0.5, right: 0.55, bottom: 0.5, left: 0.55 });
    expect(stepAxis(uniform, 'ends', -0.05)).toEqual({ top: 0.45, right: 0.5, bottom: 0.45, left: 0.5 });
  });

  // Stepping a non-uniform axis shifts both sides rather than levelling them: the user opened
  // the per-side disclosure to make them differ, so a nudge in the linked view must not undo it.
  it('preserves an asymmetry while stepping', () => {
    expect(stepAxis({ ...uniform, left: 0.4, right: 0.8 }, 'sides', 0.05)).toEqual({
      top: 0.5,
      right: 0.85,
      bottom: 0.5,
      left: 0.45,
    });
  });

  it('clamps each side independently at the bounds', () => {
    const at = { top: MARGIN_MAX, right: MARGIN_MAX, bottom: MARGIN_MIN, left: MARGIN_MIN };
    expect(stepAxis(at, 'sides', 0.05)).toEqual({ ...at, right: MARGIN_MAX, left: MARGIN_MIN + 0.05 });
  });
});

describe('stepFontSize', () => {
  it('steps by half a point', () => {
    expect(stepFontSize(10, 0.5)).toBe(10.5);
  });

  // Zero means "the template decides", and the stepper has to leave that state deliberately
  // rather than arithmetically: 0 + 0.5 would be 0.5pt, clamped to the minimum, which reads as
  // the size jumping to its smallest the first time the user nudges it upward.
  it('leaves the unset state at the template default, not at zero plus a step', () => {
    expect(stepFontSize(0, 0.5)).toBe(10);
    expect(stepFontSize(0, -0.5)).toBe(9);
  });

  it('clamps at both ends', () => {
    expect(stepFontSize(FONT_SIZE_MAX, 0.5)).toBe(FONT_SIZE_MAX);
    expect(stepFontSize(FONT_SIZE_MIN, -0.5)).toBe(FONT_SIZE_MIN);
  });

  it('does not accumulate float drift', () => {
    let v = 8.5;
    for (let i = 0; i < 7; i++) v = stepFontSize(v, 0.5);
    expect(v).toBe(12);
  });
});

describe('clampWidth', () => {
  it('returns the value when inside the range', () => {
    expect(clampWidth(500, 360, 900)).toBe(500);
  });
  it('clamps to the minimum below the range', () => {
    expect(clampWidth(100, 360, 900)).toBe(360);
  });
  it('clamps to the maximum above the range', () => {
    expect(clampWidth(1200, 360, 900)).toBe(900);
  });
  it('returns integer pixels (rounds)', () => {
    expect(clampWidth(500.7, 360, 900)).toBe(501);
  });
});

describe('paginateBlocks', () => {
  it('keeps blocks that fit on one page together', () => {
    expect(paginateBlocks([300, 300, 300], 1000)).toEqual([[0, 1, 2]]);
  });

  it('treats an exactly-full page as fitting (boundary is inclusive)', () => {
    expect(paginateBlocks([500, 500], 1000)).toEqual([[0, 1]]);
  });

  it('starts a new page when the next block would overflow', () => {
    expect(paginateBlocks([400, 400, 400], 1000)).toEqual([
      [0, 1],
      [2],
    ]);
  });

  it('gives a block taller than a whole page its own page', () => {
    expect(paginateBlocks([1500, 300], 1000)).toEqual([[0], [1]]);
  });

  it('returns a single empty page for no blocks', () => {
    expect(paginateBlocks([], 1000)).toEqual([[]]);
  });
});

describe('stepMargin', () => {
  it('steps up without float drift', () => {
    expect(stepMargin(0.5, 0.05)).toBe(0.55);
  });

  it('clamps at the minimum', () => {
    expect(stepMargin(MARGIN_MIN, -0.05)).toBe(MARGIN_MIN);
  });

  it('clamps at the maximum', () => {
    expect(stepMargin(MARGIN_MAX, 0.05)).toBe(MARGIN_MAX);
  });

  it('steps down within range', () => {
    expect(stepMargin(0.75, -0.05)).toBe(0.7);
  });
});
