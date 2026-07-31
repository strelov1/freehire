import { describe, expect, it } from 'vitest';
import { cvOpenedLabel } from './followup';
import type { MyJob } from './types';

const job = (cv_opened_at: string | null): MyJob => ({ cv_opened_at }) as unknown as MyJob;

describe('cvOpenedLabel', () => {
  it('says nothing when no CV of this application was ever opened', () => {
    expect(cvOpenedLabel(job(null))).toBeNull();
  });

  it('reads in whole days, matching the silence badge beside it', () => {
    const now = new Date('2026-07-31T12:00:00Z');
    expect(cvOpenedLabel(job('2026-07-29T12:00:00Z'), now)).toBe('CV opened 2d ago');
    expect(cvOpenedLabel(job('2026-07-30T12:00:00Z'), now)).toBe('CV opened yesterday');
    expect(cvOpenedLabel(job('2026-07-31T09:00:00Z'), now)).toBe('CV opened today');
  });

  // A browser clock ahead of the server would otherwise print a negative age, the same guard
  // chasedLabel carries.
  it('does not print a negative age', () => {
    const now = new Date('2026-07-31T12:00:00Z');
    expect(cvOpenedLabel(job('2026-08-02T12:00:00Z'), now)).toBe('CV opened today');
  });

  it('says nothing for a timestamp that will not parse', () => {
    expect(cvOpenedLabel(job('not a date'))).toBeNull();
  });
});
