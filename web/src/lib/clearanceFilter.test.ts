import { describe, it, expect } from 'vitest';
import { emptyFilters, filtersToParams, filtersFromParams, activeFilterCount } from './facetModel';

// The control is phrased as an exclusion — "hide jobs requiring security clearance" —
// so the checked state serializes to requires_clearance=false. The inversion is
// deliberate: the API param names the facet's value, while the control names what the
// user wants done about it.
describe('hideClearance filter', () => {
  it('serializes the checked state as requires_clearance=false', () => {
    const f = emptyFilters();
    f.hideClearance = true;
    expect(filtersToParams(f).get('requires_clearance')).toBe('false');
  });

  it('writes no param when unchecked, so the default listing is unfiltered', () => {
    expect(filtersToParams(emptyFilters()).has('requires_clearance')).toBe(false);
  });

  it('round-trips through the URL', () => {
    const f = emptyFilters();
    f.hideClearance = true;
    expect(filtersFromParams(filtersToParams(f)).hideClearance).toBe(true);
  });

  it('reads an absent param as unchecked', () => {
    expect(filtersFromParams(new URLSearchParams()).hideClearance).toBe(false);
  });

  // requires_clearance=true is a valid API request — a cleared candidate searching for
  // the work they are uniquely eligible for — but it is not what this checkbox means,
  // so it must not tick it.
  it('does not read requires_clearance=true as hiding them', () => {
    expect(filtersFromParams(new URLSearchParams('requires_clearance=true')).hideClearance).toBe(
      false,
    );
  });

  it('counts towards the mobile filter badge', () => {
    const f = emptyFilters();
    f.hideClearance = true;
    expect(activeFilterCount(f)).toBe(1);
  });
});
