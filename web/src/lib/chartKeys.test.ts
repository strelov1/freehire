import { describe, expect, it } from 'vitest';
import { nearestIndexByX, nextChartIndex } from './chartKeys';

describe('nextChartIndex', () => {
  it('starts at the ends and clamps', () => {
    expect(nextChartIndex('ArrowRight', null, 3)).toBe(0);
    expect(nextChartIndex('ArrowLeft', null, 3)).toBe(2);
    expect(nextChartIndex('ArrowRight', 2, 3)).toBe(2);
    expect(nextChartIndex('ArrowLeft', 0, 3)).toBe(0);
    expect(nextChartIndex('Home', 2, 3)).toBe(0);
    expect(nextChartIndex('End', 0, 3)).toBe(2);
    expect(nextChartIndex('Enter', 1, 3)).toBeNull();
    expect(nextChartIndex('ArrowRight', null, 0)).toBeNull();
  });
});

describe('nearestIndexByX', () => {
  const pts = [{ x: 10 }, { x: 50 }, { x: 200 }];

  it('picks the closest point, not the slot the x falls in', () => {
    expect(nearestIndexByX(pts, 0)).toBe(0);
    expect(nearestIndexByX(pts, 29)).toBe(0);
    expect(nearestIndexByX(pts, 31)).toBe(1);
    expect(nearestIndexByX(pts, 140)).toBe(2);
  });

  it('is null for no points', () => {
    expect(nearestIndexByX([], 5)).toBeNull();
  });
});

