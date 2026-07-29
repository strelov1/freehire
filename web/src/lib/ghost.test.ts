import { describe, expect, it } from 'vitest';
import { ghostBadge, ghostChecklist, supersedesReality } from './ghost';
import type { Ghost } from './generated/contracts';

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
      ghostBadge(possible)!.label,
      ghostBadge({ ...possible, level: 'likely' })!.label,
    ];
    for (const label of labels) {
      expect(label.toLowerCase()).toMatch(/possibly|likely/);
      expect(label.toLowerCase()).not.toMatch(/ghost|fake|scam|not hiring/);
    }
  });

  it('distinguishes the two levels', () => {
    expect(ghostBadge(possible)!.label).not.toBe(ghostBadge({ ...possible, level: 'likely' })!.label);
  });

  it('carries the scale as fired-over-total', () => {
    expect(ghostBadge(possible)!.scale).toBe('2/4');
  });

  it('tones the stronger level more loudly', () => {
    expect(ghostBadge(possible)!.tone).toBe('muted');
    expect(ghostBadge({ ...possible, level: 'likely' })!.tone).toBe('warn');
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

  // "No data" is the point of showing an unfired row: it tells the reader WHY the
  // level is not higher, instead of leaving them to guess how serious this is.
  it('says a criterion has no data rather than hiding it', () => {
    const silent = ghostChecklist(possible).find((r) => r.code === 'silent_applications')!;
    expect(silent.fired).toBe(false);
    expect(silent.detail.toLowerCase()).toContain('no data');
  });

  it('reports how many people contributed once the gate is met', () => {
    const rows = ghostChecklist({
      ...possible,
      level: 'likely',
      criteria: [...possible.criteria, 'user_reports'],
      contributors: 4,
    });
    const reports = rows.find((r) => r.code === 'user_reports')!;
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
    const ats = rows.find((r) => r.code === 'ats_absent')!;
    expect(ats.detail.toLowerCase()).toMatch(/checked/);
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
