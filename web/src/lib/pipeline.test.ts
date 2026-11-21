import { describe, expect, it } from 'vitest';

import { STAGE_VALUES } from './generated/contracts';
import { PIPELINE_BANDS, interviewRate, offerRate, replyRate } from './pipeline';
import type { PipelineStats } from './types';

const stats = (stages: Record<string, number>, reached = { interview: 0, offer: 0 }): PipelineStats => ({
  applications: Object.values(stages).reduce((a, b) => a + b, 0),
  stages: Object.fromEntries(STAGE_VALUES.map((s) => [s, stages[s] ?? 0])) as PipelineStats['stages'],
  reached_interview: reached.interview,
  reached_offer: reached.offer,
});

// The bands ARE the board's columns — same ids, same labels, same order. A funnel that
// grouped applications differently from the board would be a third vocabulary, which is
// what this change exists to remove.
describe('PIPELINE_BANDS', () => {
  it('follows the generated groups in order', () => {
    expect(PIPELINE_BANDS.map((b) => b.id)).toEqual(['preparing', 'applied', 'interview', 'offer', 'closed']);
  });

  it('places every stage in exactly one band', () => {
    const placed = PIPELINE_BANDS.flatMap((b) => b.stages);
    expect([...placed].sort()).toEqual([...STAGE_VALUES].sort());
  });

  it('gives every band a colour', () => {
    for (const band of PIPELINE_BANDS) {
      expect(band.color, band.id).toMatch(/^#[0-9a-f]{6}$/i);
    }
  });
});

// Both rates read the server's history counts, not the current stage: an application
// rejected after interviewing is still an interview.
describe('rates', () => {
  it('divides the reached counts by applications', () => {
    const s = stats({ applied: 77, interview: 20, offer: 2, accepted: 1 }, { interview: 23, offer: 3 });
    expect(interviewRate(s)).toBeCloseTo(0.23);
    expect(offerRate(s)).toBeCloseTo(0.03);
  });

  it('counts an interview that ended in a rejection', () => {
    const s = stats({ applied: 6, rejected: 4 }, { interview: 4, offer: 1 });
    expect(interviewRate(s)).toBeCloseTo(0.4);
    expect(offerRate(s)).toBeCloseTo(0.1);
  });

  it('yields zero without dividing by zero', () => {
    const s = stats({});
    expect(interviewRate(s)).toBe(0);
    expect(offerRate(s)).toBe(0);
  });
});

// The backend serves raw counts, never a pre-divided percentage — same precedent as
// interviewRate/offerRate above — so the comparison card computes both sides itself.
describe('replyRate', () => {
  it('divides answered by applications', () => {
    expect(replyRate({ applications: 287, answered: 97 })).toBeCloseTo(0.338);
  });

  it('yields zero without dividing by zero', () => {
    expect(replyRate({ applications: 0, answered: 0 })).toBe(0);
  });
});
