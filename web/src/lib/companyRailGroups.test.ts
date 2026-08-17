import { describe, expect, it } from 'vitest';

import { COMPANY_FACETS } from './facets';
import { COMPANY_RAIL_GROUPS } from './filterSections';

describe('company filter rail groups', () => {
  // The rail is the ONLY way into a company facet: a param the groups forget is
  // still parsed from the URL and still sent to the API, but nothing in the modal
  // can select it. That is how the curated Industry facet and Company stage sat
  // unreachable — declared, wired, and invisible. Assert the cover so the next
  // facet cannot repeat it.
  it('renders every company facet in exactly one pane', () => {
    const grouped = COMPANY_RAIL_GROUPS.flatMap((g) => g.params);

    expect(new Set(grouped).size).toBe(grouped.length);
    expect(new Set(grouped)).toEqual(new Set(COMPANY_FACETS.map((f) => f.param)));
  });

  it('names each pane with a distinct key', () => {
    const keys = COMPANY_RAIL_GROUPS.map((g) => g.key);

    expect(new Set(keys).size).toBe(keys.length);
  });
});
