import { describe, expect, it } from 'vitest';

import { COMPANY_FACETS, FACETS } from './facets';
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

  it('offers no separate Domain control either', () => {
    // The Domain facet asked the same question of the job-derived column, so the two
    // read as rival answers rather than one. The Industry facet now matches through
    // that column as well (see internal/industrytag's domain mapping), which leaves
    // Domain nothing of its own to offer.
    expect(COMPANY_FACETS.find((f) => f.param === 'domains')).toBeUndefined();
  });

  it('leaves the job catalogue its own domain facet', () => {
    // Same param name, different catalogue: on /jobs it filters a job's own
    // enrichment, which no curated company vocabulary covers. Retiring it here must
    // not take that one with it.
    expect(FACETS.find((f) => f.param === 'domains')).toBeDefined();
  });
});
