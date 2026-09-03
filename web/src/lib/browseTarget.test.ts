import { describe, it, expect } from 'vitest';
import { browseQuery, planForSuggestion } from './browseTarget';

// Off the list pages the header search has no list to filter, so a pick becomes a
// NAVIGATION to the feed carrying that filter. This is the only thing the launcher
// ever did differently, and turning it into a URL is the whole of it.
describe('browseQuery', () => {
  it('carries free text', () => {
    expect(browseQuery({ facets: [], q: 'product owner' })).toBe('q=product+owner');
  });

  it('carries a facet', () => {
    expect(browseQuery({ facets: [['role', 'backend']] })).toBe('role=backend');
  });

  // A composed suggestion names several things at once, and the feed must open with
  // all of them — the same rule the in-place filter follows.
  it('carries every part of a composed suggestion', () => {
    const got = new URLSearchParams(
      browseQuery({ facets: [['role', 'senior_software_engineer'], ['company_slug', 'google']] }),
    );
    expect(got.get('role')).toBe('senior_software_engineer');
    expect(got.get('company_slug')).toBe('google');
  });

  it('carries text and facets together', () => {
    const got = new URLSearchParams(browseQuery({ facets: [['skills', 'java']], q: 'remote' }));
    expect(got.get('skills')).toBe('java');
    expect(got.get('q')).toBe('remote');
  });

  // Nothing to search is not a search: the caller uses this to decide whether to
  // navigate at all, so an empty plan must be distinguishable.
  it('is empty when nothing was named', () => {
    expect(browseQuery({ facets: [] })).toBe('');
    expect(browseQuery({ facets: [], q: '   ' })).toBe('');
  });
});

describe('planForSuggestion', () => {
  it('applies a starter category to the category facet', () => {
    expect(planForSuggestion({ kind: 'category', slug: 'backend', label: 'Backend' }).facets).toEqual([
      ['category', 'backend'],
    ]);
  });

  it('applies a role to the role facet', () => {
    expect(planForSuggestion({ kind: 'role', slug: 'devops', label: 'DevOps Engineer' }).facets).toEqual([
      ['role', 'devops'],
    ]);
  });
});
