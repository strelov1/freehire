import { describe, expect, it } from 'vitest';
import { dateXs } from './growthArea';

describe('dateXs', () => {
  it('spaces points by elapsed days, so a missing week is wider than one day', () => {
    const [a, b, c] = dateXs(['2026-11-14', '2026-11-15', '2026-11-22'], 100, 0);
    expect(a).toBe(0);
    expect(b).toBeCloseTo(12.5);
    expect(c).toBe(100);
  });

  it('centres a single point', () => {
    expect(dateXs(['2026-11-14'], 100, 10)).toEqual([50]);
  });
});
