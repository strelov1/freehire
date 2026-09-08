import { describe, expect, it } from 'vitest';

import { TALENT_FACETS } from './facets';
import { TALENT_RAIL_GROUPS } from './filterSections';
import { TALENT_FACET_PARAMS } from './generated/contracts';

describe('talent catalogue rail groups', () => {
  // The rail is the ONLY way into a catalogue facet: a param the groups forget is still
  // parsed from the URL and still sent to the API, but nothing in the modal can select
  // it. That is precisely the failure this whole feature exists to have fixed — a filter
  // reachable only by editing the URL — so it must not come back one facet at a time.
  it('renders every catalogue facet in exactly one pane', () => {
    const grouped = TALENT_RAIL_GROUPS.flatMap((g) => g.params);

    expect(new Set(grouped).size).toBe(grouped.length);
    expect(new Set(grouped)).toEqual(new Set(TALENT_FACETS.map((f) => f.param)));
  });

  // The rail and the facet definitions agreeing with each OTHER only proves they were
  // written from the same list. What the visitor actually meets is the API, and it is in
  // another language: a facet defined here that the API does not read is reported in
  // meta.ignored_params and WIDENS the answer, showing more candidates than the chips
  // claim. TALENT_FACET_PARAMS is generated from Go's own FacetParams, so this is the tie
  // that the two lists above cannot be.
  it('defines exactly the facets the API reads', () => {
    expect(new Set(TALENT_FACETS.map((f) => f.param))).toEqual(new Set(TALENT_FACET_PARAMS));
  });

  it('names each pane with a distinct key', () => {
    const keys = TALENT_RAIL_GROUPS.map((g) => g.key);

    expect(new Set(keys).size).toBe(keys.length);
  });
});
