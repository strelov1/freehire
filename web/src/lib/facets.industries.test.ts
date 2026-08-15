import { describe, expect, it } from 'vitest';

import { COMPANY_FACETS } from './facets';
import { INDUSTRY_VALUES } from './generated/contracts';

describe('company industry facets', () => {
  it('offers the curated industry vocabulary', () => {
    const facet = COMPANY_FACETS.find((f) => f.param === 'industries');

    expect(facet).toBeDefined();
    expect(facet?.label).toBe('Industry');
    // Options come from the generated contract, so the filter cannot drift from
    // the Go dictionary the column is written through.
    expect(facet?.options?.length).toBe(INDUSTRY_VALUES.length);
  });

  it('labels the YC-only subindustry facet as such', () => {
    // subindustry is written by the YC importer alone, so calling it plain
    // "Industry" promised catalogue-wide coverage it never had — and it held the
    // label the curated vocabulary now earns.
    const facet = COMPANY_FACETS.find((f) => f.param === 'subindustries');

    expect(facet?.label).toBe('YC industry');
  });

  it('keeps the coarse domain facet distinct from the fine one', () => {
    const labels = COMPANY_FACETS.filter((f) =>
      ['industries', 'domains', 'subindustries'].includes(f.param),
    ).map((f) => f.label);

    expect(new Set(labels).size).toBe(labels.length);
  });
});
