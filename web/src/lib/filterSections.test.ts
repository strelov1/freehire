import { describe, expect, it } from 'vitest';
import { RAIL } from './filterSections';

describe('the Experience pane', () => {
  it('is a ROLE-section rail entry of its own', () => {
    const entry = RAIL.find((e) => e.key === 'experience');
    expect(entry).toBeDefined();
    expect(entry?.kind).toBe('experience');
    expect(entry?.section).toBe('ROLE');
  });

  // Seniority is a control of the Experience pane, not a facet entry of its own —
  // FilterModal renders it there. A standalone entry would put the same pills in
  // two places in the rail.
  it('leaves seniority without a standalone rail entry', () => {
    expect(RAIL.find((e) => e.key === 'seniority')).toBeUndefined();
    expect(RAIL.find((e) => e.facetParam === 'seniority')).toBeUndefined();
  });
});

describe('the ai_archetype facet', () => {
  it('has no standalone rail entry — FilterModal folds it into the Role pane instead', () => {
    expect(RAIL.find((e) => e.key === 'ai_archetype')).toBeUndefined();
    expect(RAIL.find((e) => e.facetParam === 'ai_archetype')).toBeUndefined();
  });
});
