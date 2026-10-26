import { describe, expect, it } from 'vitest';
import { nextChartIndex } from './chartKeys';

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
