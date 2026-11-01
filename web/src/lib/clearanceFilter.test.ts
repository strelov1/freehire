import { describe, it, expect } from 'vitest';
import { emptyFilters, filtersToParams, filtersFromParams, activeFilterCount } from './facetModel';
import { FACETS } from './facets';
import { CLEARANCE_LEVEL_VALUES } from './generated/contracts';

// Security clearance is the `clearance_levels` chip facet: the levels a posting states,
// plus the reserved `none` ("no explicit reference to clearance"). It replaced a
// three-state Hide/Only toggle serialized as `requires_clearance=false|true`, and
// saved searches, alerts and shared links still carry that param — so the old link
// must keep meaning what it meant, which the backend makes exact: `clearance_levels=none`
// is answered with `requires_clearance=false` (search.ClearanceLevelUnspecified).
describe('clearance levels filter', () => {
  it('offers every level the dictionary writes, after the none chip', () => {
    const def = FACETS.find((f) => f.param === 'clearance_levels');
    const values = (def?.options ?? []).map((o) => o.value);
    expect(values).toEqual(['none', ...CLEARANCE_LEVEL_VALUES]);
    // Unique values: the chips are a keyed each block.
    expect(new Set(values).size).toBe(values.length);
  });

  it('writes no clearance param by default', () => {
    const p = filtersToParams(emptyFilters());
    expect(p.has('clearance_levels')).toBe(false);
    expect(p.has('requires_clearance')).toBe(false);
  });

  it('round-trips included and excluded levels through the URL', () => {
    const f = emptyFilters();
    f.facets.clearance_levels = { include: ['secret', 'ts_sci'], exclude: ['none'], matchAll: false };
    const p = filtersToParams(f);
    expect(p.get('clearance_levels')).toBe('secret,ts_sci');
    expect(p.get('clearance_levels_exclude')).toBe('none');
    expect(filtersFromParams(p).facets.clearance_levels).toEqual(f.facets.clearance_levels);
  });

  it('reads the legacy requires_clearance=false as the none chip', () => {
    const f = filtersFromParams(new URLSearchParams('requires_clearance=false'));
    expect(f.facets.clearance_levels?.include).toEqual(['none']);
    expect(f.facets.clearance_levels?.exclude).toEqual([]);
    // ...and writes it back in the new form only.
    const p = filtersToParams(f);
    expect(p.get('clearance_levels')).toBe('none');
    expect(p.has('requires_clearance')).toBe(false);
  });

  it('reads the legacy requires_clearance=true as excluding the none chip', () => {
    const f = filtersFromParams(new URLSearchParams('requires_clearance=true'));
    expect(f.facets.clearance_levels?.include).toEqual([]);
    expect(f.facets.clearance_levels?.exclude).toEqual(['none']);
  });

  it('keeps what clearance_levels says when a link carries both forms', () => {
    const f = filtersFromParams(new URLSearchParams('requires_clearance=false&clearance_levels_exclude=none'));
    expect(f.facets.clearance_levels?.include).toEqual([]);
    expect(f.facets.clearance_levels?.exclude).toEqual(['none']);
  });

  it('adds the legacy none chip beside levels the link already selects', () => {
    const f = filtersFromParams(new URLSearchParams('requires_clearance=false&clearance_levels=public_trust'));
    expect(f.facets.clearance_levels?.include).toEqual(['public_trust', 'none']);
  });

  // A hand-edited or truncated link must not invent a selection.
  it('ignores a malformed legacy value', () => {
    const f = filtersFromParams(new URLSearchParams('requires_clearance=maybe'));
    expect(f.facets.clearance_levels?.include).toEqual([]);
    expect(f.facets.clearance_levels?.exclude).toEqual([]);
  });

  it('counts each selected level towards the filter badge', () => {
    const f = filtersFromParams(new URLSearchParams('clearance_levels=secret,top_secret'));
    expect(activeFilterCount(f)).toBe(2);
    expect(activeFilterCount(filtersFromParams(new URLSearchParams('requires_clearance=false')))).toBe(1);
  });
});
