import { describe, expect, it } from 'vitest';
import { must } from './utils';
import {
  CONVERGENCE,
  CRITERIA,
  WITNESS_GATE,
  ghostBadge,
  ghostChecklist,
  ghostGauge,
  ghostLevel,
  ghostUnobserved,
  supersedesReality,
} from './ghost';
import type { Ghost } from './generated/contracts';

const CODES = ['evergreen_posting', 'ats_absent', 'silent_applications', 'user_reports'];

const possible: Ghost = {
  level: 'possible',
  criteria: ['evergreen_posting', 'ats_absent'],
  criteria_total: 4,
};

describe('ghostBadge', () => {
  it('shows nothing without a signal', () => {
    expect(ghostBadge(null)).toBeNull();
    expect(ghostBadge(undefined)).toBeNull();
  });

  // The system observes facts about a posting, never an employer's intent, so the
  // strongest wording available is that a posting may be inactive.
  it('hedges the wording at every level', () => {
    const labels = [
      must(ghostBadge(possible)).label,
      must(ghostBadge({ ...possible, level: 'likely' })).label,
    ];
    for (const label of labels) {
      expect(label.toLowerCase()).toMatch(/possibly|likely/);
      expect(label.toLowerCase()).not.toMatch(/ghost|fake|scam|not hiring/);
    }
  });

  it('distinguishes the two levels', () => {
    expect(must(ghostBadge(possible)).label).not.toBe(
      must(ghostBadge({ ...possible, level: 'likely' })).label,
    );
  });

  it('carries the scale as fired-over-total', () => {
    expect(must(ghostBadge(possible)).scale).toBe('2/4');
  });

  it('tones the stronger level more loudly', () => {
    expect(must(ghostBadge(possible)).tone).toBe('muted');
    expect(must(ghostBadge({ ...possible, level: 'likely' })).tone).toBe('warn');
  });

  // An unknown level must not render as a badge with an empty label: a chip that
  // says nothing beside a job is worse than no chip.
  it('shows nothing for a level it does not know', () => {
    expect(ghostBadge({ ...possible, level: 'invented' })).toBeNull();
  });
});

describe('ghostChecklist', () => {
  it('lists every criterion, not only the ones that fired', () => {
    expect(ghostChecklist(possible)).toHaveLength(4);
  });

  it('marks which criteria fired', () => {
    const rows = ghostChecklist(possible);
    expect(rows.filter((r) => r.fired).map((r) => r.code)).toEqual([
      'evergreen_posting',
      'ats_absent',
    ]);
  });

  // The tick beside the row already says "yes". Repeating it on its own line was a
  // second line of type carrying nothing.
  it('adds no detail to a criterion whose firing is the whole fact', () => {
    const evergreen = must(ghostChecklist(possible).find((r) => r.code === 'evergreen_posting'));
    expect(evergreen.fired).toBe(true);
    expect(evergreen.detail).toBe('');
  });

  // Unfired criteria collapse into one summary line, which needs a name that reads
  // in a list rather than the full sentence a row of its own can afford.
  it('carries a short name for every criterion', () => {
    for (const row of ghostChecklist(possible)) {
      expect(row.short.length).toBeGreaterThan(0);
      expect(row.short.length).toBeLessThan(row.label.length);
    }
  });

  // A fired criterion's fact is appended to its own line rather than given a second
  // line of its own, so it has to read as a continuation and not as a new sentence.
  it('phrases a fired criterion fact to sit inline after its label', () => {
    const rows = ghostChecklist({
      ...possible,
      ats_checked_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
    });
    const ats = must(rows.find((r) => r.code === 'ats_absent'));
    expect(ats.detail.startsWith('checked ')).toBe(true);
  });

  it('reports how many people contributed once the gate is met', () => {
    const rows = ghostChecklist({
      ...possible,
      level: 'likely',
      criteria: [...possible.criteria, 'user_reports'],
      contributors: 4,
    });
    const reports = must(rows.find((r) => r.code === 'user_reports'));
    expect(reports.fired).toBe(true);
    expect(reports.detail).toContain('4');
  });

  // Below the gate the server omits the count entirely, so the UI must not invent
  // one — a count of one identifies that applicant to the employer.
  it('never invents a contributor count the server withheld', () => {
    for (const row of ghostChecklist(possible)) {
      expect(row.detail).not.toMatch(/\b1 (person|people)\b/);
    }
  });

  it('dates the cross-check when the criterion stands on it', () => {
    const rows = ghostChecklist({
      ...possible,
      ats_checked_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
    });
    const ats = must(rows.find((r) => r.code === 'ats_absent'));
    expect(ats.detail.toLowerCase()).toMatch(/checked/);
  });
});

describe('ghostUnobserved', () => {
  it('names every criterion that did not fire', () => {
    const line = ghostUnobserved(possible);
    for (const row of ghostChecklist(possible).filter((r) => !r.fired)) {
      expect(line).toContain(row.short);
    }
  });

  // The payload says a criterion did not fire. It never says whether that criterion
  // was checked and found clear or had nothing to check — `evergreen_posting` is
  // computed for every job, so "no data" about it is simply false. The summary must
  // claim only the absence of an observation, which is true either way.
  it('claims an absence of observation, never an absence of data', () => {
    const line = ghostUnobserved(possible);
    expect(line.toLowerCase()).not.toMatch(/no data|nothing known|unknown/);
    expect(line.toLowerCase()).toContain('not observed');
  });

  it('says nothing when every criterion fired', () => {
    expect(ghostUnobserved({ ...possible, criteria: CODES })).toBe('');
  });
});

describe('ghostGauge', () => {
  it('draws one segment per criterion the classifier weighs', () => {
    expect(must(ghostGauge(possible)).segments).toBe(4);
  });

  it('fills exactly the criteria that fired', () => {
    expect(must(ghostGauge(possible)).filled).toBe(2);
  });

  // The gauge is a second reading of the same evidence, and it has to be readable
  // at four states where the wording has only two. Keying the tone to `level` would
  // render one-of-four and two-of-four identically.
  it('separates a count the wording cannot', () => {
    const one = must(ghostGauge({ ...possible, criteria: ['evergreen_posting'] }));
    const two = must(ghostGauge(possible));
    expect(one.tone).not.toBe(two.tone);
  });

  it('escalates the tone as more criteria fire', () => {
    const tones = [1, 2, 3, 4].map(
      (n) => must(ghostGauge({ ...possible, criteria: CODES.slice(0, n) })).tone,
    );
    expect(new Set(tones).size).toBe(4);
    expect(tones.at(-1)).toBe('severe');
  });

  // Thresholds keyed to an absolute count would mis-tone the day the classifier
  // gains a fifth criterion: three of five is not three of four.
  it('tones on the share that fired, not the raw count', () => {
    const threeOfFour = must(ghostGauge({ ...possible, criteria: CODES.slice(0, 3) }));
    const threeOfSix = must(
      ghostGauge({ ...possible, criteria: CODES.slice(0, 3), criteria_total: 6 }),
    );
    expect(threeOfSix.tone).not.toBe(threeOfFour.tone);
  });

  it('shows nothing wherever the chip shows nothing', () => {
    expect(ghostGauge(null)).toBeNull();
    expect(ghostGauge(undefined)).toBeNull();
    expect(ghostGauge({ ...possible, level: 'invented' })).toBeNull();
  });

  // A gauge with nothing to draw is not a gauge. The row falls back to its wording
  // rather than rendering an empty frame beside "Possibly inactive".
  it('shows nothing when there is nothing to fill', () => {
    expect(ghostGauge({ ...possible, criteria: [] })).toBeNull();
    expect(ghostGauge({ ...possible, criteria_total: 0 })).toBeNull();
  });

  // The scale's denominator is served, so a payload claiming more fired criteria
  // than the total must not paint segments that do not exist.
  it('never fills past the segments it has', () => {
    const gauge = must(ghostGauge({ ...possible, criteria: CODES, criteria_total: 2 }));
    expect(gauge.filled).toBeLessThanOrEqual(gauge.segments);
  });
});

describe('supersedesReality', () => {
  // evergreen_posting IS the reality verdict. Rendering both badges shows one fact
  // twice, the second time louder.
  it('replaces the reality badge when a signal is present', () => {
    expect(supersedesReality(possible)).toBe(true);
  });

  it('leaves the reality badge alone when there is no signal', () => {
    expect(supersedesReality(null)).toBe(false);
    expect(supersedesReality(undefined)).toBe(false);
  });

  it('leaves the reality badge alone for a level it does not know', () => {
    expect(supersedesReality({ ...possible, level: 'invented' })).toBe(false);
  });
});

// The level rule, mirrored from internal/ghost so the /features/ghost-jobs landing can
// demonstrate the ladder instead of asserting it in a sentence no test can reach.
describe('ghostLevel', () => {
  const STRUCTURAL = ['evergreen_posting', 'ats_absent'];
  const OUTCOME = ['silent_applications', 'user_reports'];

  it('shows nothing when neither gate passes', () => {
    expect(ghostLevel([], 0)).toBe('none');
    expect(ghostLevel(['evergreen_posting'], 0)).toBe('none');
    // One outcome criterion from one person passes neither gate either.
    expect(ghostLevel(['user_reports'], 1)).toBe('none');
  });

  it('reaches possible on convergence alone', () => {
    expect(ghostLevel(STRUCTURAL, 0)).toBe('possible');
  });

  // Two strangers independently reporting silence is a stronger fact than any two
  // artifacts of posting shape, so the witness gate stands on its own.
  it('reaches possible on the witness gate alone', () => {
    expect(ghostLevel(['user_reports'], WITNESS_GATE)).toBe('possible');
  });

  it('reaches likely only when both gates pass', () => {
    expect(ghostLevel(['ats_absent', 'user_reports'], WITNESS_GATE)).toBe('likely');
  });

  // The gate counts people, not criteria: one person with both a silent application and
  // a filed report converges two criteria but remains a single witness.
  it('counts people rather than criteria', () => {
    expect(ghostLevel(OUTCOME, 1)).toBe('possible');
    expect(ghostLevel(OUTCOME, WITNESS_GATE)).toBe('likely');
  });

  // The guarantee the landing is built to demonstrate. It lives here rather than in the
  // caller because the landing's sandbox lets a reader move the criteria and the
  // contributor count independently: a rule that trusted its caller would happily render
  // `likely` from posting shape alone, which is the one claim the feature must never make.
  it('never reaches likely on structural evidence, whatever the contributor count', () => {
    const subsets = [[], ['evergreen_posting'], ['ats_absent'], STRUCTURAL];
    for (const criteria of subsets) {
      for (const contributors of [0, 1, 2, 7]) {
        expect(ghostLevel(criteria, contributors)).not.toBe('likely');
      }
    }
  });

  it('mirrors the gates it depends on', () => {
    expect(CONVERGENCE).toBe(2);
    expect(WITNESS_GATE).toBe(2);
  });

  // The tiers above are written out rather than derived, so a flipped tier in CRITERIA
  // fails a test instead of being followed by both the rule and its test. Pinning them
  // here is what stops the opposite failure: a fifth criterion joining the vocabulary
  // would otherwise leave the structural property above quietly untested on it.
  it('covers the whole vocabulary', () => {
    expect(CRITERIA.filter((c) => c.tier === 'structural').map((c) => c.code)).toEqual(STRUCTURAL);
    expect(CRITERIA.filter((c) => c.tier === 'outcome').map((c) => c.code)).toEqual(OUTCOME);
  });
});
