import { describe, expect, it } from 'vitest';
import { summarizeDemand } from './roleDemand';

describe('summarizeDemand', () => {
  it('compares the latest four weeks with the four before them', () => {
    expect(summarizeDemand([99, 99, 99, 99, 5, 5, 5, 5, 4, 6, 5, 10])).toEqual({
      latest: 25,
      previous: 20,
      change: 0.25,
    });
  });

  it('keeps a zero baseline distinct from a drop to zero', () => {
    expect(summarizeDemand([0, 0, 0, 0, 0, 0, 0, 3]).change).toBeNull();
    expect(summarizeDemand([0, 0, 0, 0, 0, 0, 0, 0]).change).toBeNull();
    expect(summarizeDemand([3, 0, 0, 0, 0, 0, 0, 0]).change).toBe(-1);
  });
});
