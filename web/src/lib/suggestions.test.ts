import { describe, it, expect } from 'vitest';
import { starterSuggestions } from './suggestions';
import type { FacetCounts } from './types';

// What an empty search box offers. The point is to answer "I don't know what I can
// type here", so the list has to show the SHAPE of the catalogue rather than its
// biggest buckets — measured on production those are Management (266,883), Sales
// (179,993) and Support (127,110), which read as a different website to someone who
// came for engineering work.

const counts = (category: Record<string, number>): FacetCounts =>
  ({ total: 0, facets: { category } }) as unknown as FacetCounts;

describe('starterSuggestions', () => {
  it('leads with engineering, not with the biggest bucket', () => {
    const got = starterSuggestions(
      counts({ management: 266883, sales: 179993, backend: 8000, software_engineering: 89465 }),
    );
    expect(got[0]?.slug).toBe('software_engineering');
  });

  it('spans the groups instead of spending every row on engineering', () => {
    // Engineering alone carries 13 categories, so a flat "first N in curated order"
    // would fill the whole list with it and never reach a designer or a PM.
    const got = starterSuggestions(
      counts({
        backend: 8000, frontend: 7000, fullstack: 6000, devops: 5000, sre: 4000,
        mobile: 3000, hardware: 2000, embedded: 1000, blockchain: 900,
        architecture: 800, network_engineering: 700, industrial_engineering: 600,
        design: 40005, product: 30000,
      }),
    );
    const slugs = got.map((s) => s.slug);
    expect(slugs).toContain('design');
    expect(slugs).toContain('product');
  });

  it('takes the busiest category of each group as its representative', () => {
    const got = starterSuggestions(counts({ backend: 8000, software_engineering: 89465 }));
    expect(got[0]?.slug).toBe('software_engineering');
  });

  it('offers a category no measurement covers not at all', () => {
    const got = starterSuggestions(counts({ backend: 8000 }));
    expect(got.map((s) => s.slug)).toEqual(['backend']);
  });

  it('carries the open-posting count so a row says how big it is', () => {
    const got = starterSuggestions(counts({ backend: 8000 }));
    expect(got[0]?.count).toBe(8000);
  });

  it('names each row the way the filter modal names it', () => {
    const got = starterSuggestions(counts({ ml_ai: 1000 }));
    expect(got[0]?.label).toBe('ML / AI');
  });

  it('applies the category facet, not the role facet', () => {
    const got = starterSuggestions(counts({ backend: 8000 }));
    expect(got[0]?.kind).toBe('category');
  });

  // `other` is a real category and a useless suggestion: it names nothing, so it
  // cannot answer the question the empty box is asking.
  it('never offers the catch-all category', () => {
    const got = starterSuggestions(counts({ other: 500000, backend: 8000 }));
    expect(got.map((s) => s.slug)).not.toContain('other');
  });

  it('puts the consumer industries last', () => {
    const got = starterSuggestions(counts({ healthcare: 77787, backend: 8000 }));
    expect(got.map((s) => s.slug)).toEqual(['backend', 'healthcare']);
  });

  // The curated group order is a priority, not just a sequence: Engineering, Data &
  // AI, Quality & Security and Design & Creative come first because that is who this
  // catalogue is for. Taking one per group flattens that priority — measured against
  // production it spent half the list on Management, Sales, HR, Operations and
  // Healthcare. Two per group spends the ten rows on the groups that lead.
  it('gives each leading group two rows before reaching a later one', () => {
    const got = starterSuggestions(
      counts({
        software_engineering: 89598, devops: 53313,
        data_analytics: 77404, data_engineering: 42090,
        security: 73684, qa: 28985,
        design: 40023, creative: 9000,
        management: 267971, project_management: 68851,
        sales: 180253, hr: 15369, healthcare: 78498,
      }),
    );
    const slugs = got.map((s) => s.slug);
    expect(slugs).toEqual([
      'software_engineering', 'devops',
      'data_analytics', 'data_engineering',
      'security', 'qa',
      'design', 'creative',
      'management', 'project_management',
    ]);
    expect(slugs).not.toContain('sales');
    expect(slugs).not.toContain('healthcare');
  });

  it('moves on when a leading group cannot fill its two rows', () => {
    const got = starterSuggestions(counts({ backend: 8000, design: 40023, creative: 9000 }));
    expect(got.map((s) => s.slug)).toEqual(['backend', 'design', 'creative']);
  });

  it('offers nothing at all when nothing has been measured yet', () => {
    expect(starterSuggestions(null)).toEqual([]);
  });
});
