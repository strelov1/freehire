import { describe, it, expect } from 'vitest';
import { emptyFilters, filtersToParams, filtersFromParams, activeFilterCount } from './facetModel';

// The clearance control has three states, not two, because the facet answers two
// different people: someone who cannot hold a clearance wants these postings gone, and
// someone who holds one wants nothing else. A checkbox can only say one of those.
//
// The API param names the facet's VALUE (`requires_clearance=false` means "postings not
// marked"), while the control names the user's INTENT ("hide them"). The two therefore
// read inverted, which is why the mapping is asserted in both directions here.
describe('clearance filter', () => {
  it('defaults to any, writing no param', () => {
    const f = emptyFilters();
    expect(f.clearance).toBe('any');
    expect(filtersToParams(f).has('requires_clearance')).toBe(false);
  });

  it('hide serializes to requires_clearance=false', () => {
    const f = emptyFilters();
    f.clearance = 'hide';
    expect(filtersToParams(f).get('requires_clearance')).toBe('false');
  });

  it('only serializes to requires_clearance=true', () => {
    const f = emptyFilters();
    f.clearance = 'only';
    expect(filtersToParams(f).get('requires_clearance')).toBe('true');
  });

  it.each(['hide', 'only'] as const)('round-trips %s through the URL', (state) => {
    const f = emptyFilters();
    f.clearance = state;
    expect(filtersFromParams(filtersToParams(f)).clearance).toBe(state);
  });

  it('reads an absent param as any', () => {
    expect(filtersFromParams(new URLSearchParams()).clearance).toBe('any');
  });

  // A hand-edited or truncated link must not land the control in a state it cannot
  // render. Anything unrecognised reads as the neutral default.
  it('reads a malformed value as any', () => {
    expect(filtersFromParams(new URLSearchParams('requires_clearance=maybe')).clearance).toBe('any');
  });

  it.each([
    ['hide', 1],
    ['only', 1],
    ['any', 0],
  ] as const)('counts %s towards the filter badge as %i', (state, want) => {
    const f = emptyFilters();
    f.clearance = state;
    expect(activeFilterCount(f)).toBe(want);
  });
});
