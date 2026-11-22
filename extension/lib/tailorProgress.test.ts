import { describe, it, expect } from 'vitest';
import { TAILOR_IDLE_MS, tailorFinished, tailorProgress } from './tailorProgress';

describe('tailorProgress', () => {
  it('is empty before the run lays down its plan', () => {
    expect(tailorProgress(undefined)).toEqual({ done: 0, total: 0 });
    expect(tailorFinished({ done: 0, total: 0 }, TAILOR_IDLE_MS * 10)).toBe(false);
  });

  it('counts every reached requirement, closed or not', () => {
    const report = [
      { requirement: 'Java', status: 'closed_bank' },
      { requirement: 'Kafka', status: 'open' },
      { requirement: 'Spring', status: 'not_reached' },
    ];
    expect(tailorProgress(report)).toEqual({ done: 2, total: 3 });
  });

  it('is finished when every requirement is reached', () => {
    expect(tailorFinished({ done: 3, total: 3 }, 0)).toBe(true);
  });

  it('is finished when the CV has been quiet long enough', () => {
    expect(tailorFinished({ done: 2, total: 3 }, TAILOR_IDLE_MS - 1)).toBe(false);
    expect(tailorFinished({ done: 2, total: 3 }, TAILOR_IDLE_MS)).toBe(true);
  });
});
