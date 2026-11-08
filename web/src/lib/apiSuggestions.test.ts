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
      api(
        { kind: 'category', slug: 'senior_software_engineer', text: 'Senior Software Engineer' },
        { kind: 'company', slug: 'google', text: 'Google' },
      ),
    ]);
    expect(got[0]?.label).toBe('Senior Software Engineer Google');
  });

  it('carries the posting count so a row says how big it is', () => {
    const got = fromApi([api({ kind: 'category', slug: 'backend', text: 'Backend Engineer' })]);
    expect(got[0]?.count).toBe(100);
  });

  // The glyph and the key are chosen from the LAST part: that is the completion, the
  // part the row actually adds, and the earlier parts are context the visitor already
  // typed.
  it('takes its kind from the part being completed', () => {
    const got = fromApi([
      api(
        { kind: 'category', slug: 'backend', text: 'Backend Engineer' },
        { kind: 'company', slug: 'google', text: 'Google' },
      ),
    ]);
    expect(got[0]?.kind).toBe('company');
  });

  it('gives rows distinct keys even when they complete to the same word', () => {
    const got = fromApi([
      api({ kind: 'category', slug: 'backend', text: 'Backend Engineer' }),
      api({ kind: 'skill', slug: 'backend', text: 'Backend' }),
    ]);
    expect(got[0]?.slug).not.toBe(got[1]?.slug);
  });
});

// Picking a row applies EVERY part it names. Applying one of two silently discards
// what the visitor typed, which is exactly the composed search this feature exists to
// make possible.
describe('applyParams', () => {
  it('applies a specialization and a company together', () => {
    const got = applyParams([
      { kind: 'category', slug: 'software_engineering', text: 'Software Engineering' },
      { kind: 'company', slug: 'google', text: 'Google' },
    ]);
    expect(got.facets).toEqual([
      ['category', 'software_engineering'],
      ['company_slug', 'google'],
    ]);
    expect(got.q).toBeUndefined();
  });

  // A title names no facet, so it becomes a query — quoted and restricted to the
  // title field, so the count the suggestion showed (an exact-title-match count)
  // approximates what the click actually returns, instead of an unscoped multi-field
  // match against title, company, description, and location.
  it('applies a title as a quoted, title-scoped query', () => {
    const got = applyParams([{ kind: 'title', text: 'Product Owner' }]);
    expect(got.q).toBe('"Product Owner"');
    expect(got.qFields).toEqual(['title']);
    expect(got.facets).toEqual([]);
  });

  // Meilisearch's quoting syntax uses `"` as its own delimiter, so an embedded one
  // would split the query into more than one quoted segment instead of one.
  it('strips an embedded quote from a title before wrapping it', () => {
    const got = applyParams([{ kind: 'title', text: 'He said "wow" Engineer' }]);
    expect(got.q).toBe('"He said wow Engineer"');
  });

  it('quotes and scopes the title part of a composed suggestion, alongside the facet', () => {
    const got = applyParams([
      { kind: 'title', text: 'Founding Engineer' },
      { kind: 'company', slug: 'acme', text: 'Acme' },
    ]);
    expect(got.q).toBe('"Founding Engineer"');
    expect(got.qFields).toEqual(['title']);
    expect(got.facets).toEqual([['company_slug', 'acme']]);
  });

  // A suggestion with no title part never touches search text at all, so it must not
  // carry a stray field restriction that would silently narrow an unrelated `q` a
  // caller might already have typed.
  it('carries no field restriction when there is no title part', () => {
    const got = applyParams([{ kind: 'company', slug: 'acme', text: 'Acme' }]);
    expect(got.qFields).toBeUndefined();
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
    const got = applyParams([{ kind: 'category', text: 'Backend Engineer' }]);
    expect(got.facets).toEqual([]);
  });
});
