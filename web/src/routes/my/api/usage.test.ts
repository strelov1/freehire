import { describe, expect, it } from 'vitest';
import { dailyUsage } from './usage';

describe('dailyUsage', () => {
  it('fills missing days with zero and ends on the UTC day of now', () => {
    const rows = [
      { day: '2026-11-11', requests: 5, errors: 1 },
      { day: '2026-11-13', requests: 7, errors: 0 },
    ];
    const out = dailyUsage(rows, new Date('2026-11-13T23:30:00Z'), 3);
    expect(out).toEqual([
      { date: '2026-11-11', new: 5 },
      { date: '2026-11-12', new: 0 },
      { date: '2026-11-13', new: 7 },
    ]);
  });
});
