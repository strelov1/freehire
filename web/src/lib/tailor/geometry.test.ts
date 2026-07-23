import { describe, it, expect } from 'vitest';
import { clampWidth, paginateBlocks, stepMargin, MARGIN_MIN, MARGIN_MAX } from './geometry';

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
