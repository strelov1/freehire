import { describe, it, expect } from 'vitest';
import { fromApi, applyParams } from './apiSuggestions';
import type { ApiSuggestion } from './types';

const api = (...parts: ApiSuggestion['parts']): ApiSuggestion => ({
  text: parts.map((p) => p.text).join(' '),
  parts,
  jobs: 100,
});

describe('fromApi', () => {
  it('shows the whole phrase, not just the completion', () => {
    const got = fromApi([
      api({ kind: 'role', slug: 'senior_software_engineer', text: 'Senior Software Engineer' }, { kind: 'company', slug: 'google', text: 'Google' }),
    ]);
    expect(got[0]?.label).toBe('Senior Software Engineer Google');
  });

  it('carries the posting count so a row says how big it is', () => {
    const got = fromApi([api({ kind: 'role', slug: 'backend', text: 'Backend Engineer' })]);
    expect(got[0]?.count).toBe(100);
  });

  // The glyph and the key are chosen from the LAST part: that is the completion, the
  // part the row actually adds, and the earlier parts are context the visitor already
  // typed.
  it('takes its kind from the part being completed', () => {
    const got = fromApi([
      api({ kind: 'role', slug: 'backend', text: 'Backend Engineer' }, { kind: 'company', slug: 'google', text: 'Google' }),
    ]);
    expect(got[0]?.kind).toBe('company');
  });

  it('gives rows distinct keys even when they complete to the same word', () => {
    const got = fromApi([
      api({ kind: 'role', slug: 'backend', text: 'Backend Engineer' }),
      api({ kind: 'skill', slug: 'backend', text: 'Backend' }),
    ]);
    expect(got[0]?.slug).not.toBe(got[1]?.slug);
  });
});

// Picking a row applies EVERY part it names. Applying one of two silently discards
// what the visitor typed, which is exactly the composed search this feature exists to
// make possible.
describe('applyParams', () => {
  it('applies a role and a company together', () => {
    const got = applyParams([
      { kind: 'role', slug: 'senior_software_engineer', text: 'Senior Software Engineer' },
      { kind: 'company', slug: 'google', text: 'Google' },
    ]);
    expect(got.facets).toEqual([
      ['role', 'senior_software_engineer'],
      ['company_slug', 'google'],
    ]);
    expect(got.q).toBeUndefined();
  });

  it('applies a title as free text, since no facet names it', () => {
    const got = applyParams([{ kind: 'title', text: 'Product Owner' }]);
    expect(got.q).toBe('Product Owner');
    expect(got.facets).toEqual([]);
  });

  it('maps each kind to its own facet', () => {
    const got = applyParams([
      { kind: 'skill', slug: 'java', text: 'Java' },
      { kind: 'category', slug: 'backend', text: 'Backend' },
    ]);
    expect(got.facets).toEqual([
      ['skills', 'java'],
      ['category', 'backend'],
    ]);
  });

  // A part with no slug and a kind that needs one is malformed; dropping it is better
  // than writing `role=undefined` into the URL.
  it('ignores a facet part with no value', () => {
    const got = applyParams([{ kind: 'role', text: 'Backend Engineer' }]);
    expect(got.facets).toEqual([]);
  });
});
