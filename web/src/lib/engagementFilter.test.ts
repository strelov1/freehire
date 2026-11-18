import { describe, expect, it } from 'vitest';
import { emptyFilters, filtersFromParams, filtersToParams } from './facetModel';
import { ENGAGEMENT_GROUPS, FACETS, visibleEngagementGroups } from './facets';
import { ENGAGEMENT_LABELS, ENGAGEMENT_MARKETS, ENGAGEMENT_VALUES } from './generated/contracts';

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
    for (const o of def?.options ?? [])
      expect(o.label).toBe(ENGAGEMENT_LABELS[o.value as keyof typeof ENGAGEMENT_LABELS]);
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

describe('engagement market groups', () => {
  it('cover every value once, each under its generated market', () => {
    const values = ENGAGEMENT_GROUPS.flatMap((g) => g.options.map((o) => o.value));
    expect(values).toEqual([...ENGAGEMENT_VALUES]);
    for (const g of ENGAGEMENT_GROUPS)
      for (const o of g.options) expect(ENGAGEMENT_MARKETS[o.value as keyof typeof ENGAGEMENT_MARKETS]).toBe(g.market);
    expect(ENGAGEMENT_GROUPS[0]?.market).toBe('United Kingdom');
  });

  it('shows only the markets the scope reaches, plus any holding a selection', () => {
    expect(visibleEngagementGroups(undefined, [])).toBe(ENGAGEMENT_GROUPS);
    const uk = visibleEngagementGroups({ ir35_outside: 12, ir35_inside: 30 }, []);
    expect(uk.map((g) => g.market)).toEqual(['United Kingdom']);
    const kept = visibleEngagementGroups({ ir35_outside: 12 }, ['clt']);
    expect(kept.map((g) => g.market)).toEqual(['United Kingdom', 'Brazil']);
    expect(visibleEngagementGroups({}, [])).toEqual([]);
  });
});
