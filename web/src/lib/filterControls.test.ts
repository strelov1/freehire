import { describe, expect, it } from 'vitest';
import {
  EXPERIENCE_PRESETS,
  experienceLabel,
  FRESHNESS_PRESETS,
  freshnessLabel,
  freshnessOptions,
} from './filterControls';

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

// A select can only display a value it has an option for. The label above tells the
// truth about an off-preset bound; without a matching option the CONTROL still renders
// blank over that live bound, which is the same lie from a different direction.
describe('freshnessOptions', () => {
  it('is the presets, unchanged, for an unset or preset bound', () => {
    expect(freshnessOptions(null)).toBe(FRESHNESS_PRESETS);
    expect(freshnessOptions(7)).toBe(FRESHNESS_PRESETS);
  });

  it('offers an off-preset bound so the control can show it', () => {
    const opts = freshnessOptions(5);
    const match = opts.find((o) => o.days === 5);

    expect(match).toBeDefined();
    expect(match?.label).toBe(freshnessLabel(5));
    expect(opts).toHaveLength(FRESHNESS_PRESETS.length + 1);
  });

  it('keeps the list in day order with Any still last', () => {
    const days = freshnessOptions(5).map((o) => o.days);

    expect(days).toEqual([1, 3, 5, 7, 14, 30, 90, null]);
  });

  it('places a bound past every preset before Any, not after it', () => {
    expect(freshnessOptions(365).map((o) => o.days)).toEqual([1, 3, 7, 14, 30, 90, 365, null]);
  });
});
