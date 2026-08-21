import { describe, expect, it } from 'vitest';
import { EXPERIENCE_PRESETS, experienceLabel, FRESHNESS_PRESETS, freshnessLabel } from './filterControls';

describe('EXPERIENCE_PRESETS', () => {
  it('runs least-to-most experience with Any as the rightmost stop', () => {
    const years = EXPERIENCE_PRESETS.map((p) => p.years);
    expect(years.at(-1)).toBeNull();

    const bounded = years.slice(0, -1) as number[];
    expect(bounded).toEqual([...bounded].sort((a, b) => a - b));
    expect(bounded.every((y) => Number.isInteger(y) && y >= 0)).toBe(true);
  });

  // The leftmost stop is the entry-level selector. It must be a real 0, not null:
  // `experience_years_max=0` is what selects the postings stating no prior
  // experience is required, while null means "no bound at all".
  it('starts at a zero bound, not at no bound', () => {
    expect(EXPERIENCE_PRESETS[0]?.years).toBe(0);
  });

  it('labels every stop', () => {
    expect(EXPERIENCE_PRESETS.every((p) => p.label.length > 0)).toBe(true);
  });
});

describe('experienceLabel', () => {
  it('names each preset stop', () => {
    for (const p of EXPERIENCE_PRESETS) {
      expect(experienceLabel(p.years)).toBe(p.label);
    }
  });

  it('reads an unset bound as Any', () => {
    expect(experienceLabel(null)).toBe('Any');
  });

  // A hand-edited URL can carry a year count that is no preset stop. The label must
  // still describe the bound in force rather than claiming the filter is off.
  it('describes an off-preset bound instead of calling it Any', () => {
    expect(experienceLabel(6)).not.toBe('Any');
    expect(experienceLabel(6)).toContain('6');
  });
});

describe('freshnessLabel', () => {
  it('names each preset stop', () => {
    for (const p of FRESHNESS_PRESETS) {
      expect(freshnessLabel(p.days)).toBe(p.label);
    }
  });

  it('reads an unset bound as Any', () => {
    expect(freshnessLabel(null)).toBe('Any');
  });

  // Same hazard experienceLabel already guards against: a shared link, a hand-edited
  // URL or an AI-built filter can carry a day count that is no preset stop. Reading it
  // as "Any" tells the user the freshness filter is off while it is quietly hiding
  // every posting older than that bound.
  it('describes an off-preset bound instead of calling it Any', () => {
    expect(freshnessLabel(5)).not.toBe('Any');
    expect(freshnessLabel(5)).toContain('5');
  });
});
