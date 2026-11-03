import { describe, expect, it } from 'vitest';
import { emptyFilters, filtersFromParams, filtersToParams } from './facetModel';
import { FACETS } from './facets';
import { ENGAGEMENT_LABELS, ENGAGEMENT_VALUES } from './generated/contracts';

// The engagement facet is the "Contract terms" chip set — the IR35 status a UK posting
// states. It rides the generic facet plumbing, so these pin what is specific to it:
// every value the dictionary writes is offered under its generated label, and both
// include and exclude round-trip through the URL.
describe('engagement facet', () => {
  const def = FACETS.find((f) => f.param === 'engagement');

  it('offers every generated value, excludable', () => {
    expect(def?.control).toBe('pills');
    expect(def?.excludable).toBe(true);
    expect((def?.options ?? []).map((o) => o.value)).toEqual([...ENGAGEMENT_VALUES]);
    for (const o of def?.options ?? []) expect(o.label).toBe(ENGAGEMENT_LABELS[o.value as keyof typeof ENGAGEMENT_LABELS]);
  });

  it('round-trips include and exclude through the URL', () => {
    const f = emptyFilters();
    f.facets.engagement = { include: ['ir35_outside'], exclude: ['ir35_inside'], matchAll: false };
    const p = filtersToParams(f);
    expect(p.getAll('engagement')).toEqual(['ir35_outside']);
    expect(p.getAll('engagement_exclude')).toEqual(['ir35_inside']);
    expect(filtersFromParams(p).facets.engagement).toEqual(f.facets.engagement);
  });
});
