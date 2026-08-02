import { describe, expect, it } from 'vitest';

import { STAGE_VALUES } from './generated/contracts';
import { PIPELINE_BANDS, interviewRate, offerRate } from './pipeline';
import type { PipelineStats } from './types';

const stats = (stages: Record<string, number>): PipelineStats => ({
  applications: Object.values(stages).reduce((a, b) => a + b, 0),
  stages: Object.fromEntries(STAGE_VALUES.map((s) => [s, stages[s] ?? 0])) as PipelineStats['stages'],
});

// The bands ARE the board's columns — same ids, same labels, same order. A funnel that
// grouped applications differently from the board would be a third vocabulary, which is
// what this change exists to remove.
describe('PIPELINE_BANDS', () => {
  it('follows the generated groups in order', () => {
    expect(PIPELINE_BANDS.map((b) => b.id)).toEqual(['applied', 'interview', 'offer', 'closed']);
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

// Both rates are a current-status snapshot and so a lower bound: an application rejected
// after interviewing is only ever counted as rejected.
describe('rates', () => {
  it('counts everyone at or past interview', () => {
    const s = stats({ applied: 77, interview: 20, offer: 2, accepted: 1 });
    expect(interviewRate(s)).toBeCloseTo(0.23);
    expect(offerRate(s)).toBeCloseTo(0.03);
  });

  it('reads the counts per stage, not per band', () => {
    // screening and responded sit in the `applied` band but are not interviews.
    const s = stats({ applied: 50, screening: 30, responded: 20 });
    expect(interviewRate(s)).toBe(0);
  });

  it('yields zero without dividing by zero', () => {
    const s = stats({});
    expect(interviewRate(s)).toBe(0);
    expect(offerRate(s)).toBe(0);
  });
});
