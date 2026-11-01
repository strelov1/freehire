import { describe, expect, it } from 'vitest';
import { emptyFilters, filtersFromParams, filtersToParams } from './facetModel';
import { FACETS } from './facets';
import { PERK_LABELS, PERK_VALUES } from './generated/contracts';
import { RAIL } from './filterSections';

// The perks facet is the description-derived "Benefits & Perks" chip set. It rides the
// generic facet plumbing, so these pin what is specific to it: every perk the dictionary
// writes is offered under its generated label, the param round-trips through the URL,
// and it is include-only, because a posting without a perk is one whose description does
// not mention it, not one that lacks it.
describe('perks facet', () => {
  const def = FACETS.find((f) => f.param === 'perks');

  it('offers every generated perk, include-only, with AND/OR', () => {
    expect(def?.control).toBe('pills');
    expect(def?.excludable).toBe(false);
    expect(def?.hasAndOr).toBe(true);
    expect((def?.options ?? []).map((o) => o.value)).toEqual([...PERK_VALUES]);
    for (const o of def?.options ?? []) expect(o.label).toBe(PERK_LABELS[o.value as keyof typeof PERK_LABELS]);
  });

  it('round-trips through the URL, AND mode included', () => {
    const f = emptyFilters();
    f.facets.perks = { include: ['401k_match', 'unlimited_pto'], exclude: [], matchAll: true };
    const p = filtersToParams(f);
    expect(p.get('perks_mode')).toBe('and');
    expect(filtersFromParams(p).facets.perks).toEqual(f.facets.perks);
  });

  it('names every perk in the search aliases', () => {
    expect(Object.keys(def?.searchAliases ?? {}).toSorted()).toEqual([...PERK_VALUES].toSorted());
  });

  it('has a rail pane under Pay & Benefits', () => {
    const entry = RAIL.find((e) => e.facetParam === 'perks');
    expect(entry?.label).toBe('Benefits & Perks');
    expect(entry?.section).toBe('PAY & BENEFITS');
  });
});
