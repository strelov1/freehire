import { describe, it, expect } from 'vitest';
import { clampWidth } from './geometry';

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
