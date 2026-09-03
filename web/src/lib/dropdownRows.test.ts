import { describe, it, expect } from 'vitest';
import { dropdownRows, namedCompanies } from './dropdownRows';
import type { Suggestion } from './suggestions';
import type { Job, CompanyListItem } from './types';

// The dropdown holds three sections — what to search, what exists, who is hiring —
// and the keyboard has to run through all of them as ONE list. Every off-by-one in
// arrow navigation lives here, so the flattening is a function rather than three
// index offsets computed in the template.

const suggestion = (slug: string): Suggestion => ({ kind: 'role', slug, label: slug });
const job = (slug: string): Job => ({ public_slug: slug, title: slug }) as unknown as Job;
const company = (slug: string): CompanyListItem => ({ slug, name: slug }) as unknown as CompanyListItem;

describe('dropdownRows', () => {
  it('runs suggestions, then postings, then companies', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a')],
      jobs: [job('j')],
      companies: [company('c')],
      text: 'x',
    });
    expect(rows.map((r) => r.kind)).toEqual(['suggestion', 'job', 'company', 'text']);
  });

  // The free-text row is last because it is the fallback, and absent on an empty box
  // because there is no text to offer searching for.
  it('offers the free-text row last, and only with text to search', () => {
    expect(dropdownRows({ suggestions: [], jobs: [], companies: [], text: 'x' }).at(-1)?.kind).toBe(
      'text',
    );
    expect(dropdownRows({ suggestions: [suggestion('a')], jobs: [], companies: [], text: '  ' })
      .map((r) => r.kind)).toEqual(['suggestion']);
  });

  it('skips a section that has nothing rather than leaving a gap', () => {
    const rows = dropdownRows({
      suggestions: [],
      jobs: [job('j')],
      companies: [],
      text: 'x',
    });
    expect(rows.map((r) => r.kind)).toEqual(['job', 'text']);
  });

  it('numbers rows continuously so one arrow press crosses a section boundary', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a'), suggestion('b')],
      jobs: [job('j')],
      companies: [company('c')],
      text: 'x',
    });
    expect(rows.map((_, i) => i)).toEqual([0, 1, 2, 3, 4]);
    expect(rows[2]?.kind).toBe('job');
  });

  it('marks the first row of each section so a heading renders once', () => {
    const rows = dropdownRows({
      suggestions: [suggestion('a'), suggestion('b')],
      jobs: [job('j')],
      companies: [],
      text: 'x',
    });
    expect(rows.map((r) => r.first)).toEqual([true, false, true, true]);
  });

  it('gives every row a key that cannot collide across sections', () => {
    // `backend` is both a role slug and a company slug; a key of the slug alone would
    // duplicate, and a duplicate key in an {#each} is what took the site down before.
    const rows = dropdownRows({
      suggestions: [suggestion('backend')],
      jobs: [job('backend')],
      companies: [company('backend')],
      text: 'x',
    });
    const keys = rows.map((r) => r.key);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it('is empty when there is nothing to show at all', () => {
    expect(dropdownRows({ suggestions: [], jobs: [], companies: [], text: '' })).toEqual([]);
  });
});

// The companies endpoint matches fuzzily, which is right for a companies PAGE and
// wrong for a dropdown section three rows tall. Observed in the browser: typing
// `product own` offered Energise.pro, CLOVERDALE FOODS CO and GreenStar Food Co+op —
// none of them anything to do with the query, and enough to make the whole dropdown
// look broken.
describe('namedCompanies', () => {
  const named = (...names: string[]) =>
    names.map((name) => ({ name, slug: name })) as unknown as CompanyListItem[];

  it('drops a company the query does not name', () => {
    const got = namedCompanies(named('Energise.pro', 'CLOVERDALE FOODS CO'), 'product own');
    expect(got).toEqual([]);
  });

  it('keeps a company the query names', () => {
    expect(namedCompanies(named('Google'), 'google').map((c) => c.name)).toEqual(['Google']);
  });

  it('keeps a company the query has only started naming', () => {
    expect(namedCompanies(named('Google'), 'goog').map((c) => c.name)).toEqual(['Google']);
  });

  it('ignores case, since a company writes its own name however it likes', () => {
    expect(namedCompanies(named('CLOVERDALE FOODS CO'), 'cloverdale').map((c) => c.name)).toEqual([
      'CLOVERDALE FOODS CO',
    ]);
  });

  it('matches inside the name, not only at its start', () => {
    expect(namedCompanies(named('The Reject Shop'), 'reject').map((c) => c.name)).toEqual([
      'The Reject Shop',
    ]);
  });

  it('offers nothing for an empty query rather than everything', () => {
    expect(namedCompanies(named('Google'), '   ')).toEqual([]);
  });

  it('caps how many survive', () => {
    const got = namedCompanies(named('Googol A', 'Googol B', 'Googol C', 'Googol D'), 'googol', 3);
    expect(got).toHaveLength(3);
  });
});
