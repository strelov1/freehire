import { describe, it, expect } from 'vitest';
import { BOARD_COLUMNS, columnOf } from './board';
import type { MyJob } from './types';

// columnOf reads only `stage` and `applied_at`; a minimal cast fixture suffices.
function job(fields: Partial<MyJob>): MyJob {
  return { stage: null, applied_at: null, saved_at: null, ...fields } as MyJob;
}

describe('BOARD_COLUMNS', () => {
  it('has no Saved column — only the active application states', () => {
    expect(BOARD_COLUMNS.map((c) => c.id)).toEqual(['applied', 'interview', 'offer', 'closed']);
  });
});

describe('columnOf', () => {
  it('maps a precise stage to its column', () => {
    expect(columnOf(job({ stage: 'interview' }))).toBe('interview');
    expect(columnOf(job({ stage: 'offer' }))).toBe('offer');
    expect(columnOf(job({ stage: 'screening' }))).toBe('applied');
    expect(columnOf(job({ stage: 'rejected' }))).toBe('closed');
  });

  it('maps an applied-without-stage row to applied', () => {
    expect(columnOf(job({ applied_at: '2026-07-12T00:00:00Z' }))).toBe('applied');
  });

  it('returns null for a saved-only row (not on the board)', () => {
    expect(columnOf(job({ saved_at: '2026-07-12T00:00:00Z' }))).toBeNull();
    expect(columnOf(job({}))).toBeNull();
  });
});
