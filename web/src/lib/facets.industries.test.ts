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

  it('offers no second industry vocabulary beside it', () => {
    // The YC subindustry leaf named the same thing in the directory's own words and
    // for the ~1% of companies it covers. cmd/import-yc now folds that leaf into the
    // curated column, so offering it as its own facet would be two filters for one
    // idea — the column stays, the filter does not.
    expect(COMPANY_FACETS.find((f) => f.param === 'subindustries')).toBeUndefined();
  });

  it('keeps the coarse domain facet distinct from the fine one', () => {
    const labels = COMPANY_FACETS.filter((f) => ['industries', 'domains'].includes(f.param)).map(
      (f) => f.label,
    );

    expect(new Set(labels).size).toBe(labels.length);
  });
});
